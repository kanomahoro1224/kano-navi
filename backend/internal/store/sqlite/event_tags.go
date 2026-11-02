package sqlite

import (
	"context"
	"fmt"
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
