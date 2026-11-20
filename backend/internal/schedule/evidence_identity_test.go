package schedule

import (
	"context"
	"testing"
	"time"

	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
)

const (
	syntheticVideo    = "synthVideo1"
	syntheticVideoURL = "https://www.youtube.com/watch?v=" + syntheticVideo
	youtubeEventID    = "youtube-" + syntheticVideo
)

// applyEntries writes one model result for an X post as an accepted
// extraction does.
func (f *fixture) applyEntries(postID, classification string, entries ...any) {
	f.t.Helper()
	published := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	if entries == nil {
		entries = []any{}
	}
	result := obj("classification", classification, "events", entries, "confidence", 0.9, "evidence", "e", "reason", nil, "action", "add")
	normalized, err := xProfile.normalizeResult(result, postID, "https://x.com/synthetic_handle/status/"+postID, "", published, published)
	if err != nil {
		f.t.Fatalf("normalize: %v", err)
	}
	if err = f.run.applyExtraction(context.Background(), "x", postID, 1, normalized); err != nil {
		f.t.Fatalf("apply: %v", err)
	}
}

// listedStream is one entry of a model result: the same activity whatever
// its link, so its source key stays the same.
func listedStream(title, clock string, url any, tag any) any {
	return obj("title", title, "detail", nil, "date", "2026-04-10", "time", clock, "endTime", nil, "endDate", nil,
		"timePrecision", "exact", "status", "scheduled", "eventType", "stream", "tag", tag, "url", url,
		"platform", "youtube", "confidence", 0.9, "evidence", "synthetic evidence")
}

// xEventOf is the X identity of the post's live evidence, or "".
func (f *fixture) evidenceEvent(postID string) string {
	f.t.Helper()
	return f.text(`SELECT event_id FROM event_sources WHERE source='x' AND source_item_id=? AND deleted_at IS NULL`, postID)
}

func (f *fixture) live(eventID string) bool {
	f.t.Helper()
	return f.number(`SELECT COUNT(*) FROM events WHERE id=? AND deleted_at IS NULL`, eventID) == 1
}

func (f *fixture) attention(eventID string) string {
	f.t.Helper()
	return f.text(`SELECT `+sqlite.EventAttentionSQL("e", "2026-04-01")+` FROM events e WHERE e.id=?`, eventID)
}

func TestEvidenceMovingToAnotherIdentityRetiresTheEventItLeft(t *testing.T) {
	f := newFixture(t)
	f.addPost("p1", "", "synthetic post", "2026-04-01T00:00:00.000Z", "")
	f.applyEntries("p1", "schedule", listedStream("Synthetic stream", "20:00", nil, nil))
	xEvent := f.evidenceEvent("p1")
	if xEvent == "" || xEvent == youtubeEventID || !f.live(xEvent) {
		t.Fatalf("x event = %q", xEvent)
	}
	// A duplicate the reconciler merged into it.
	f.exec(`INSERT INTO events (id, source, title, starts_on, provenance, merged_into, merge_source, merge_reason, created_at, updated_at)
		VALUES ('x-duplicate', 'x', 'Synthetic duplicate', '2026-04-10', 'automatic', ?, 'rule', 'rule_a', ?, ?)`, xEvent, testTimestamp, testTimestamp)

	// The post read again links the video: its evidence takes the video's
	// identity, and the X event it left is retired with its rule merges.
	f.applyEntries("p1", "schedule", listedStream("Synthetic stream", "20:00", syntheticVideoURL, nil))
	if got := f.evidenceEvent("p1"); got != youtubeEventID || !f.live(youtubeEventID) {
		t.Fatalf("evidence = %q, youtube event live = %v", got, f.live(youtubeEventID))
	}
	if f.live(xEvent) {
		t.Fatal("the X event the evidence left stays visible without any source")
	}
	if got := f.text(`SELECT COALESCE(merged_into, '-') FROM events WHERE id='x-duplicate'`); got != "-" {
		t.Fatalf("a rule merge into the retired event stays: %s", got)
	}

	// The link dropped again: the X event returns and the video's event,
	// which only this post supported, is retired.
	f.applyEntries("p1", "schedule", listedStream("Synthetic stream", "20:00", nil, nil))
	if got := f.evidenceEvent("p1"); got != xEvent || !f.live(xEvent) {
		t.Fatalf("evidence = %q, x event live = %v", got, f.live(xEvent))
	}
	if f.live(youtubeEventID) {
		t.Fatal("the video's event the evidence left stays visible without any source")
	}
}

func TestEvidenceOnAnOperatorsEventStaysWhenTheEntryGainsALink(t *testing.T) {
	for _, tc := range []struct {
		name, lock string
		live       bool
	}{
		{"tombstone", `UPDATE events SET provenance='manual', manual_locked=1, deleted_at=? WHERE id=?`, false},
		{"edit", `UPDATE events SET provenance='manual', manual_locked=1, title='Operator title', updated_at=? WHERE id=?`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			f.addPost("p1", "", "synthetic post", "2026-04-01T00:00:00.000Z", "")
			f.applyEntries("p1", "schedule", listedStream("Synthetic stream", "20:00", nil, nil))
			xEvent := f.evidenceEvent("p1")
			f.exec(tc.lock, testTimestamp, xEvent)

			f.applyEntries("p1", "schedule", listedStream("Synthetic stream", "20:00", syntheticVideoURL, nil))
			if got := f.evidenceEvent("p1"); got != xEvent {
				t.Fatalf("evidence moved off the operator's event to %q", got)
			}
			if got := f.number(`SELECT COUNT(*) FROM events WHERE id=?`, youtubeEventID); got != 0 {
				t.Fatal("the operator's activity appeared again as the video's event")
			}
			if f.live(xEvent) != tc.live {
				t.Fatalf("operator's event live = %v, want %v", f.live(xEvent), tc.live)
			}
		})
	}
}

// writeYoutubeEvent stores the video's event as the YouTube writer does,
// with its reservation evidence.
func (f *fixture) writeYoutubeEvent() {
	f.t.Helper()
	f.exec(`INSERT INTO events (id, source, source_item_id, source_key, title, detail, starts_on, starts_at, timezone, time_precision,
		status, event_type, url, is_upcoming, provenance, manual_locked, created_at, updated_at, raw_json, platform, platforms, platforms_inferred)
		VALUES (?, 'youtube', ?, 'reservation', 'Official title', 'official detail', '2026-04-10', '2026-04-10T10:30:00.000Z', 'Asia/Tokyo', 'exact',
		'upcoming', 'stream', ?, 1, 'automatic', 0, ?, ?, '{}', 'youtube', '["youtube"]', 0)`,
		youtubeEventID, syntheticVideo, syntheticVideoURL, testTimestamp, testTimestamp)
	f.exec(`INSERT INTO event_sources (event_id, source, source_item_id, source_key, url, raw_json, created_at, updated_at)
		VALUES (?, 'youtube', ?, 'reservation', ?, '{}', ?, ?)`, youtubeEventID, syntheticVideo, syntheticVideoURL, testTimestamp, testTimestamp)
}

const youtubeFields = `SELECT title || '|' || detail || '|' || starts_on || '|' || starts_at || '|' || COALESCE(ends_at, '-') || '|' || time_precision || '|' ||
	status || '|' || event_type || '|' || url || '|' || COALESCE(confidence, '-') || '|' || COALESCE(extraction_id, '-') FROM events WHERE id=?`

func TestAnEntryOfAYoutubeVideoOnlyAttachesToTheYoutubeWritersEvent(t *testing.T) {
	f := newFixture(t)
	f.addPost("p1", "", "synthetic post", "2026-04-01T00:00:00.000Z", "")
	f.writeYoutubeEvent()
	before := f.text(youtubeFields, youtubeEventID)

	f.applyEntries("p1", "schedule", listedStream("Notice title", "21:00", syntheticVideoURL, "game"))
	if got := f.evidenceEvent("p1"); got != youtubeEventID {
		t.Fatalf("evidence = %q", got)
	}
	if got := f.text(youtubeFields, youtubeEventID); got != before {
		t.Fatalf("the notice rewrote the YouTube writer's event:\n got %s\nwant %s", got, before)
	}
	// The stated label follows the label rules, as on any entry.
	if got := f.text(`SELECT COALESCE(tag, '-') || '|' || COALESCE(tag_source, '-') || '|' || platforms FROM events WHERE id=?`, youtubeEventID); got != `game|llm|["youtube"]` {
		t.Fatalf("lent label and platforms = %s", got)
	}

	// Without the YouTube writer's event the notice creates it.
	g := newFixture(t)
	g.addPost("p1", "", "synthetic post", "2026-04-01T00:00:00.000Z", "")
	g.applyEntries("p1", "schedule", listedStream("Notice title", "21:00", syntheticVideoURL, nil))
	if got := g.text(`SELECT title || '|' || starts_at FROM events WHERE id=?`, youtubeEventID); got != "Notice title|2026-04-10T12:00:00.000Z" {
		t.Fatalf("notice-created event = %s", got)
	}
}

func TestAPostReadAgainAsNoScheduleSendsItsEventsToReview(t *testing.T) {
	f := newFixture(t)
	f.addPost("p1", "", "synthetic post", "2026-04-01T00:00:00.000Z", "")
	f.addPost("p2", "", "synthetic post", "2026-04-01T00:00:00.000Z", "")
	f.applyEntries("p1", "schedule", listedStream("Synthetic stream", "20:00", nil, nil), listedStream("Other stream", "22:00", nil, nil))
	event := f.text(`SELECT event_id FROM event_sources WHERE source_item_id='p1' AND raw_json LIKE '%Synthetic stream%'`)
	other := f.text(`SELECT event_id FROM event_sources WHERE source_item_id='p1' AND raw_json LIKE '%Other stream%'`)
	// Another post still announces the other stream.
	f.applyEntries("p2", "schedule", listedStream("Other stream", "22:00", nil, nil))
	if f.evidenceEvent("p2") != other || f.attention(event) != "" {
		t.Fatalf("setup: p2 evidence %q, attention %q", f.evidenceEvent("p2"), f.attention(event))
	}

	f.applyEntries("p1", "not_schedule")
	if !f.live(event) || !f.live(other) {
		t.Fatal("a not_schedule reading retired the post's events")
	}
	if got := f.attention(event); got != sqlite.AttentionUnlisted {
		t.Fatalf("attention = %q, want unlisted", got)
	}
	if got := f.attention(other); got != "" {
		t.Fatalf("an event another post still lists needs review: %q", got)
	}
	counts, err := f.store.AdminCounts(context.Background(), "2026-04-01")
	if err != nil || counts.UnlistedEvents != 1 {
		t.Fatalf("unlisted count = %d, %v", counts.UnlistedEvents, err)
	}

	// Listing the event again settles it; so does the operator's decision.
	f.applyEntries("p1", "schedule", listedStream("Synthetic stream", "20:00", nil, nil), listedStream("Other stream", "22:00", nil, nil))
	if got := f.attention(event); got != "" {
		t.Fatalf("a listed event still needs review: %q", got)
	}
	f.applyEntries("p1", "not_schedule")
	f.exec(`UPDATE events SET provenance='manual', manual_locked=1 WHERE id=?`, event)
	if got := f.attention(event); got != "" {
		t.Fatalf("a kept event still needs review: %q", got)
	}
}
