package schedule

import "testing"

const revisionQuery = `SELECT COALESCE((SELECT CAST(value AS INTEGER) FROM app_settings WHERE key='dashboard_revision'),0)`

// Extracted events are public, so the write advances the dashboard revision
// itself; a cached rerun that writes nothing leaves it.
func TestExtractionAdvancesTheRevisionOnlyWhenItWrites(t *testing.T) {
	f := messageFixture(t)
	before := f.number(revisionQuery)
	f.scan()
	written := f.number(revisionQuery)
	if f.number(`SELECT COUNT(*) FROM events`) != 1 || written <= before {
		t.Fatalf("revision %d -> %d", before, written)
	}
	f.scan()
	if got := f.number(revisionQuery); got != written {
		t.Fatalf("cached rerun moved the revision %d -> %d", written, got)
	}
}

// A schedule image the model approves appears on the board, so the verdict
// advances the revision; the unchanged image's cached check does not.
func TestApprovedScheduleImageAdvancesTheRevision(t *testing.T) {
	f, _ := boardFixture(t)
	before := f.number(revisionQuery)
	f.scan()
	approved := f.number(revisionQuery)
	if f.text(`SELECT llm_status FROM schedule_asset_reviews WHERE asset_id='schedule-2026-04-27'`) != "schedule" || approved <= before {
		t.Fatalf("revision %d -> %d", before, approved)
	}
	f.scan()
	if got := f.number(revisionQuery); got != approved {
		t.Fatalf("cached check moved the revision %d -> %d", approved, got)
	}
}

// A write that changes no value of the post's events or evidence leaves the
// revision: a post that is no schedule, or the same result applied again.
func TestAppliedResultAdvancesTheRevisionOnlyWhenItChangesTheEvents(t *testing.T) {
	f := newFixture(t)
	f.addPost("p1", "", "synthetic post", "2026-04-01T00:00:00.000Z", "")
	step := func(label, classification string, wantChange bool, entries ...any) {
		t.Helper()
		before := f.number(revisionQuery)
		f.applyEntries("p1", classification, entries...)
		if changed := f.number(revisionQuery) != before; changed != wantChange {
			t.Fatalf("%s: revision changed=%v, want %v", label, changed, wantChange)
		}
	}
	step("no schedule, no events", "not_schedule", false)
	step("new event", "schedule", true, listedStream("Synthetic stream", "20:00", nil, nil))
	step("same result again", "schedule", false, listedStream("Synthetic stream", "20:00", nil, nil))
	step("changed time", "schedule", true, listedStream("Synthetic stream", "21:00", nil, nil))
	step("read again as no schedule", "not_schedule", true)
	step("no schedule again", "not_schedule", false)
}
