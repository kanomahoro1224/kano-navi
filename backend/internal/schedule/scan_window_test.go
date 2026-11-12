package schedule

import (
	"testing"
	"time"
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
	f.setExtractionLookback(0)

	f.scan()
	if got := f.provider.count(); got != 2 {
		t.Fatalf("provider requests = %d, want 2", got)
	}
}

// setLegacyMaxAge stores this fork's earlier window key the way a database
// upgraded from it holds it.
func (f *fixture) setLegacyMaxAge(days string) {
	f.t.Helper()
	f.exec(`INSERT INTO app_settings (key, value, updated_at) VALUES ('schedule_extract_max_age_days', ?, '2026-04-01T00:00:00.000Z')
		ON CONFLICT(key) DO UPDATE SET value=excluded.value`, days)
}

// A database upgraded from the fork keeps schedule_extract_max_age_days and
// has no stored lookback: the old key is the window, not the new default.
func TestScanWindowLegacyKeyAppliesWhileTheLookbackIsUnstored(t *testing.T) {
	f := messageFixture(t) // post-1 is published 2026-04-28T03:00Z
	f.addArchivePost("recent-archive", "2026-04-20T00:00:00.000Z")
	f.setLegacyMaxAge("3")
	f.at(time.Date(2026, 4, 29, 0, 0, 0, 0, time.UTC))

	if preview := f.preview(); preview.LookbackDays != 3 {
		t.Fatalf("lookback = %d, want the legacy 3 days", preview.LookbackDays)
	}
	f.scan()
	if got := f.provider.count(); got != 1 {
		t.Fatalf("provider requests = %d, want 1 (the 30-day default would read the 9-day-old post)", got)
	}
	if rows := f.number(`SELECT COUNT(*) FROM post_llm_states WHERE post_id='recent-archive'`); rows != 0 {
		t.Fatalf("post outside the legacy window has %d state rows", rows)
	}
}

// A stored lookback of 0 would scan every post; the legacy 30-day key still
// bounds the scan, as it did before the upgrade.
func TestScanWindowLegacyKeyAppliesWhenTheLookbackIsZero(t *testing.T) {
	f := messageFixture(t)
	f.addArchivePost("archived-1", "2024-03-01T00:00:00.000Z")
	f.setExtractionLookback(0)
	f.setLegacyMaxAge("30")
	f.at(backoffStart)

	if preview := f.preview(); preview.LookbackDays != 30 {
		t.Fatalf("lookback = %d, want 30", preview.LookbackDays)
	}
	f.scan()
	if got := f.provider.count(); got != 1 {
		t.Fatalf("provider requests = %d, want 1 (only the recent post)", got)
	}
}

// A lookback the operator stored wins over the legacy key.
func TestScanWindowStoredLookbackWinsOverTheLegacyKey(t *testing.T) {
	f := messageFixture(t)
	f.setExtractionLookback(10)
	f.setLegacyMaxAge("3")
	f.at(backoffStart)
	if preview := f.preview(); preview.LookbackDays != 10 {
		t.Fatalf("lookback = %d, want the stored 10", preview.LookbackDays)
	}
}
