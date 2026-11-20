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
