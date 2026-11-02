package sqlite

import (
	"context"
	"fmt"
	"time"
)

// VideoTags are the content tags a video may carry: what it is about, apart
// from its form (kind). The schedule labels live and video name a form and
// never reach a video. A video without a tag is "other" in the archive.
var VideoTags = []string{"game", "talk", "song"}

// ValidVideoTag reports whether value is one of VideoTags.
func ValidVideoTag(value string) bool {
	for _, tag := range VideoTags {
		if tag == value {
			return true
		}
	}
	return false
}

// videoEventMatch is the condition that the events row e is the schedule
// entry of the videos row aliased v: the entry the sync created for it, one
// whose source item it is, or one that lists it among its event sources.
func videoEventMatch(v string) string {
	return `(e.id='youtube-'||` + v + `.id OR e.source='youtube' AND e.source_item_id=` + v + `.id
		OR e.id IN (SELECT s.event_id FROM event_sources s WHERE s.source='youtube' AND s.source_item_id=` + v + `.id AND s.deleted_at IS NULL))`
}

// videoEventTagSQL reads the content label of the video's schedule entry, a
// label on the entry it is merged into first. The entry keeps its own order
// (manual, then prompt or merged notice, then title inference), so this is
// never copied into the video row.
func videoEventTagSQL(v string) string {
	return `(SELECT COALESCE(t.tag,e.tag) FROM events e
	LEFT JOIN events t ON t.id=e.merged_into AND t.deleted_at IS NULL
	WHERE e.deleted_at IS NULL AND ` + videoEventMatch(v) + `
		AND COALESCE(t.tag,e.tag) IN ('game','talk','song')
	ORDER BY e.id LIMIT 1)`
}

// videoTagSQL derives the shown content tag of the videos row aliased v: the
// operator's video tag, else its schedule entry's label, else the tag the
// title inference gave the video. It is NULL for a video without one.
func videoTagSQL(v string) string {
	return `COALESCE(` + v + `.manual_tag,` + videoEventTagSQL(v) + `,` + v + `.tag)`
}

// videoCategorySQL is a videos row's archive category: its tag, or other.
var videoCategorySQL = `COALESCE(` + videoTagSQL("videos") + `,'other')`

// VideoTagCandidate is one video whose title the model may tag.
type VideoTagCandidate struct{ ID, Source, Title string }

// ListVideoTagCandidates returns up to limit videos the schedule cannot tag:
// shown videos (not merged into another) without a manual tag or a judgement
// of their own, whose schedule entries, if any, are neither labelled, judged,
// operator-owned or merged, nor YouTube reservations still on or after
// eventFrom, the first day the entry inference reads. Videos never attempted
// come first, then the least recently attempted, each group newest first, so
// recent uploads are tagged before the backlog and a title whose attempts
// keep failing moves behind the others.
func (s *Store) ListVideoTagCandidates(ctx context.Context, eventFrom string, limit int) ([]VideoTagCandidate, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	if limit < 1 {
		limit = 1
	}
	rows, err := s.db.QueryContext(ctx, `SELECT v.id,v.source,COALESCE(v.manual_title,v.title) FROM videos v
		WHERE v.merged_into IS NULL AND v.manual_tag IS NULL AND v.tag IS NULL AND COALESCE(v.tag_source,'')=''
			AND NOT EXISTS (SELECT 1 FROM events e WHERE e.deleted_at IS NULL AND `+videoEventMatch("v")+`
				AND (e.merged_into IS NOT NULL OR e.tag IS NOT NULL OR COALESCE(e.tag_source,'')<>'' OR e.manual_locked<>0 OR e.provenance<>'automatic'
					OR e.source='youtube' AND e.source_key='reservation' AND e.starts_on>=?))
		ORDER BY v.tag_attempted_at IS NOT NULL, julianday(v.tag_attempted_at) ASC, julianday(COALESCE(v.scheduled_at,v.published_at)) DESC, v.id ASC LIMIT ?`, eventFrom, limit)
	if err != nil {
		return nil, dbErr(err)
	}
	defer rows.Close()
	candidates := []VideoTagCandidate{}
	for rows.Next() {
		var item VideoTagCandidate
		if err = rows.Scan(&item.ID, &item.Source, &item.Title); err != nil {
			return nil, dbErr(err)
		}
		candidates = append(candidates, item)
	}
	if err = rows.Err(); err != nil {
		return nil, dbErr(err)
	}
	return candidates, nil
}

// RecordVideoTag stores the title inference's tag, or records that it found
// none, on a video. A schedule label (live, video) is a form and is recorded
// as no tag. Recording the judgement even without a tag keeps the title from
// being sent again on every scan. An operator's tag, or a judgement recorded
// in the meantime, wins and the call reports false. Only a stored tag changes
// what readers see, so a judgement without one leaves the revision alone.
func (s *Store) RecordVideoTag(ctx context.Context, id string, tag *string, now time.Time) (bool, error) {
	tx, err := s.eventTx(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var value any
	if tag != nil && ValidVideoTag(*tag) {
		value = *tag
	}
	stamp := ISOTime(now)
	result, err := tx.ExecContext(ctx, `UPDATE videos SET tag=?,tag_source=?,tag_attempted_at=?
		WHERE id=? AND manual_tag IS NULL AND tag IS NULL AND COALESCE(tag_source,'')=''`,
		value, TagSourceTitle, stamp, id)
	if err != nil {
		return false, fmt.Errorf("%w: video tag", ErrUnavailable)
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

// RecordVideoTagFailure stamps a title inference that produced no answer. The
// video stays unjudged, so a passing provider outage never marks it decided,
// and the stamp moves it behind the videos not yet attempted.
func (s *Store) RecordVideoTagFailure(ctx context.Context, id string, now time.Time) error {
	if s == nil || !s.writable {
		return ErrReadOnly
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE videos SET tag_attempted_at=?
		WHERE id=? AND manual_tag IS NULL AND tag IS NULL AND COALESCE(tag_source,'')=''`,
		ISOTime(now), id); err != nil {
		return fmt.Errorf("%w: video tag", ErrUnavailable)
	}
	return nil
}
