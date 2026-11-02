package sqlite

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// EventTags are the calendar labels an event may carry. The stored values are
// stable English codes; every reader translates them for display. A video or
// an X post is not labelled itself: the label describes one calendar entry.
var EventTags = []string{"live", "game", "talk", "song", "video"}

// Who decided an event's tag. An operator's label belongs to the locked row,
// so nothing automatic replaces it. The extraction prompt, or the X notice a
// rule merge folds into the entry, outranks the title inference, which reads
// the YouTube title alone.
const (
	TagSourceLLM    = "llm"
	TagSourceTitle  = "title"
	TagSourceManual = "manual"
)

// contentTagSQL lists the labels that name what an entry is about (the
// VideoTags); formTagSQL lists the labels that name only its form. Among
// automatic labels a content label is never replaced by a form label.
const (
	contentTagSQL = `('game','talk','song')`
	formTagSQL    = `('live','video')`
)

// formTag reports whether tag names only an entry's form.
func formTag(tag string) bool { return tag == "live" || tag == "video" }

// eventEntryIDsSQL lists the events row aliased o and the rows merged into
// it, whose videos are o's videos too.
func eventEntryIDsSQL(o string) string {
	return `(SELECT ` + o + `.id UNION ALL SELECT k.id FROM events k WHERE k.merged_into=` + o + `.id AND k.deleted_at IS NULL)`
}

// eventVideoIDsSQL lists the YouTube videos of the events row aliased o by the
// links videoEventMatch reads in the other direction. The unary + keeps
// SQLite from walking every YouTube row through a source index instead of
// looking each entry up by ID.
func eventVideoIDsSQL(o string) string {
	ids := eventEntryIDsSQL(o)
	return `(SELECT substr(c.id,9) FROM events c WHERE c.id IN ` + ids + ` AND c.id LIKE 'youtube-%'
		UNION ALL SELECT c.source_item_id FROM events c WHERE c.id IN ` + ids + ` AND +c.source='youtube' AND c.source_item_id IS NOT NULL
		UNION ALL SELECT s.source_item_id FROM event_sources s WHERE s.event_id IN ` + ids + ` AND +s.source='youtube' AND s.deleted_at IS NULL)`
}

// eventLabelSQL derives the label readers see for the events row aliased o.
// An operator's label on the entry decides; an operator's row without one
// reads on like any other. Otherwise the first of: an operator's tag on one
// of its videos, the entry's own content
// label, a video's inferred tag, and the entry's form label. Video tags are
// content tags only, so a video never turns content back into a form; with
// several videos the alphabetically first tag is taken. The label is never
// copied into the entry row.
func eventLabelSQL(o string) string {
	return `CASE WHEN ` + o + `.tag_source='` + TagSourceManual + `' THEN ` + o + `.tag ELSE
		(SELECT COALESCE(MIN(v.manual_tag),CASE WHEN ` + o + `.tag IN ` + contentTagSQL + ` THEN ` + o + `.tag END,MIN(v.tag),` + o + `.tag)
		FROM videos v WHERE v.id IN ` + eventVideoIDsSQL(o) + `) END`
}

// eventColumnsAs is eventColumns read from the events row aliased o, with the
// derived label in place of the stored tag.
func eventColumnsAs(o string) string {
	columns := strings.Split(eventColumns, ",")
	for i, column := range columns {
		if column == "tag" {
			columns[i] = eventLabelSQL(o) + ` AS tag`
		} else {
			columns[i] = o + "." + column
		}
	}
	return strings.Join(columns, ",")
}

// ValidEventTag reports whether value is one of EventTags. The empty string
// means "no label" and is accepted separately.
func ValidEventTag(value string) bool {
	for _, tag := range EventTags {
		if tag == value {
			return true
		}
	}
	return false
}

// EventTagCandidate is one automatic YouTube calendar entry whose title the
// model may label.
type EventTagCandidate struct{ ID, Title string }

// ListEventTagCandidates returns up to limit unlabelled YouTube reservations
// whose Japan date falls in the inclusive [from, to] range. Entries never
// attempted come first, then the least recently attempted, each group oldest
// date first, so a title whose attempts keep failing moves behind the others
// instead of holding the head of the queue. Entries an operator locked,
// entries that already carry a label, entries the title inference already
// judged (tag_source is set even when it found no label), and entries a
// labelled notice is merged into (the merge lends them its label) are not
// offered.
func (s *Store) ListEventTagCandidates(ctx context.Context, from, to string, limit int) ([]EventTagCandidate, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	if limit < 1 {
		limit = 1
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id,title FROM events
		WHERE source='youtube' AND source_key='reservation' AND deleted_at IS NULL AND merged_into IS NULL
			AND provenance='automatic' AND manual_locked=0
			AND tag IS NULL AND COALESCE(tag_source,'')=''
			AND starts_on BETWEEN ? AND ?
			AND NOT EXISTS (SELECT 1 FROM events n WHERE n.merged_into=events.id AND n.merge_source=? AND n.deleted_at IS NULL AND n.tag IS NOT NULL)
		ORDER BY tag_attempted_at IS NOT NULL, julianday(tag_attempted_at) ASC, starts_on ASC, id ASC LIMIT ?`, from, to, MergeSourceRule, limit)
	if err != nil {
		return nil, dbErr(err)
	}
	defer rows.Close()
	candidates := []EventTagCandidate{}
	for rows.Next() {
		var item EventTagCandidate
		if err = rows.Scan(&item.ID, &item.Title); err != nil {
			return nil, dbErr(err)
		}
		candidates = append(candidates, item)
	}
	if err = rows.Err(); err != nil {
		return nil, dbErr(err)
	}
	return candidates, nil
}

// RecordEventTag stores the title inference's label, or records that it found
// none, on an automatic YouTube entry. Recording the judgement even without a
// label keeps a title the model cannot classify from being sent again on every
// scan. An operator's edit, or a label that appeared in the meantime, wins and
// the call reports false. Only a stored label changes what readers see, so a
// judgement without one leaves the dashboard revision alone.
func (s *Store) RecordEventTag(ctx context.Context, id string, tag *string, now time.Time) (bool, error) {
	tx, err := s.eventTx(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var value any
	if tag != nil && *tag != "" {
		value = *tag
	}
	stamp := ISOTime(now)
	result, err := tx.ExecContext(ctx, `UPDATE events SET tag=?,tag_source=?,tag_attempted_at=?,updated_at=?
		WHERE id=? AND deleted_at IS NULL AND manual_locked=0 AND tag IS NULL AND COALESCE(tag_source,'')=''`,
		value, TagSourceTitle, stamp, stamp, id)
	if err != nil {
		return false, fmt.Errorf("%w: event tag", ErrUnavailable)
	}
	if changed, _ := result.RowsAffected(); changed == 0 {
		return false, nil
	}
	if value != nil {
		if err = bumpRevision(ctx, tx, stamp); err != nil {
			return false, fmt.Errorf("%w: revision", ErrUnavailable)
		}
	}
	if err = tx.Commit(); err != nil {
		return false, fmt.Errorf("%w: commit", ErrUnavailable)
	}
	return true, nil
}

// RecordEventTagFailure stamps a title inference that produced no answer. The
// entry stays unjudged, so a passing provider outage never marks it decided,
// and the stamp moves it behind the entries not yet attempted. Readers see
// nothing new, so the revision stays.
func (s *Store) RecordEventTagFailure(ctx context.Context, id string, now time.Time) error {
	if s == nil || !s.writable {
		return ErrReadOnly
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE events SET tag_attempted_at=?
		WHERE id=? AND deleted_at IS NULL AND manual_locked=0 AND tag IS NULL AND COALESCE(tag_source,'')=''`,
		ISOTime(now), id); err != nil {
		return fmt.Errorf("%w: event tag", ErrUnavailable)
	}
	return nil
}
