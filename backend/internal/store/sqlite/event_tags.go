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

// Who decided an event's tag. Manual wins over the model, and an automatic
// extraction never replaces a manual one because the whole row is locked.
const (
	TagSourceLLM    = "llm"
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

// ListEventTagCandidates returns the unlabelled YouTube reservations whose
// Japan date falls in the inclusive [from, to] range, oldest first. Entries an
// operator locked, entries that already carry a label, and entries the model
// already judged (tag_source is set even when it found no label) are not
// offered again.
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
		ORDER BY starts_on ASC,id ASC LIMIT ?`, from, to, limit)
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

// RecordEventTag stores the model's label, or records that it found none, on
// an automatic YouTube entry. Recording the attempt even without a label keeps
// a title the model cannot classify from being sent again on every scan. An
// operator's edit, or a label that appeared in the meantime, wins and the call
// reports false.
func (s *Store) RecordEventTag(ctx context.Context, id string, tag *string) (bool, error) {
	tx, err := s.eventTx(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var value any
	if tag != nil && *tag != "" {
		value = *tag
	}
	now := ISOTime(time.Now())
	result, err := tx.ExecContext(ctx, `UPDATE events SET tag=?,tag_source=?,updated_at=?
		WHERE id=? AND deleted_at IS NULL AND manual_locked=0 AND tag IS NULL AND COALESCE(tag_source,'')=''`,
		value, TagSourceLLM, now, id)
	if err != nil {
		return false, fmt.Errorf("%w: event tag", ErrUnavailable)
	}
	if changed, _ := result.RowsAffected(); changed == 0 {
		return false, nil
	}
	if err = bumpRevision(ctx, tx, now); err != nil {
		return false, fmt.Errorf("%w: revision", ErrUnavailable)
	}
	if err = tx.Commit(); err != nil {
		return false, fmt.Errorf("%w: commit", ErrUnavailable)
	}
	return true, nil
}
