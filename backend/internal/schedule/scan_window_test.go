package schedule

import (
	"testing"
)

// addArchivePost stores a post that predates the scan window, the way an
// imported archive does (no raw_json, nothing extracted yet).
func (f *fixture) addArchivePost(id, publishedAt string) {
	f.t.Helper()
	f.exec(`INSERT INTO posts (id, source, account_handle, type, label, text, published_at, url)
		VALUES (?, 'x', 'synthetic_handle', 'daily', '', '2024年3月の振り返り配信', ?, ?)`,
		id, publishedAt, "https://x.com/synthetic_handle/status/"+id)
}

// An imported archive is history: posts older than the configured window stay
// out of the scan, so a backfill cannot spend provider calls.
func TestScanWindowKeepsTheArchiveOutOfTheQueue(t *testing.T) {
	f := messageFixture(t)
	f.addArchivePost("archived-1", "2024-03-01T00:00:00.000Z")
	f.env["SCHEDULE_EXTRACT_MAX_AGE_DAYS"] = "30"
	f.at(backoffStart)

	f.scan()
	if got := f.provider.count(); got != 1 {
		t.Fatalf("provider requests = %d, want 1 (only the recent post)", got)
	}
	if rows := f.number(`SELECT COUNT(*) FROM post_llm_states WHERE post_id='archived-1'`); rows != 0 {
		t.Fatalf("archived post state rows = %d, want none", rows)
	}
}

// An operator's reprocess request bypasses the window: that post is the one
// they asked about.
func TestScanWindowStillHonoursAReprocessRequest(t *testing.T) {
	f := messageFixture(t)
	f.addArchivePost("archived-1", "2024-03-01T00:00:00.000Z")
	f.env["SCHEDULE_EXTRACT_MAX_AGE_DAYS"] = "30"
	f.at(backoffStart)
	f.scan()
	if got := f.provider.count(); got != 1 {
		t.Fatalf("provider requests before reprocess = %d, want 1", got)
	}

	f.requestReprocess("archived-1")
	f.scan()
	if got := f.provider.count(); got != 2 {
		t.Fatalf("provider requests after reprocess = %d, want 2", got)
	}
}

// Without a window every stored post stays a candidate, which is the behaviour
// before the setting existed.
func TestScanWindowOffScansEveryPost(t *testing.T) {
	f := messageFixture(t)
	f.addArchivePost("archived-1", "2024-03-01T00:00:00.000Z")
	f.at(backoffStart)

	f.scan()
	if got := f.provider.count(); got != 2 {
		t.Fatalf("provider requests = %d, want 2", got)
	}
}
