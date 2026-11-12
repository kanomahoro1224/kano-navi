package schedule

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
)

func (f *fixture) setExtractionLookback(days int) {
	f.t.Helper()
	settings, err := f.store.SyncSettings(context.Background())
	if err != nil {
		f.t.Fatal(err)
	}
	settings.ExtractionLookbackDays = days
	if err = f.store.SaveSyncSettings(context.Background(), settings); err != nil {
		f.t.Fatal(err)
	}
}

func previewIDs(preview QueuePreview) []string {
	out := []string{}
	for _, entry := range preview.Run {
		out = append(out, entry.PostID)
	}
	return out
}

// Unrequested posts published before the lookback are not candidates; a
// reprocess request bypasses it; the preview lists what the run processes;
// 0 sets no limit.
func TestExtractionLookbackBoundsUnrequestedPosts(t *testing.T) {
	f := messageFixture(t) // post-1 is published 2026-04-28T03:00Z
	f.at(time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC))
	f.setExtractionLookback(30)
	f.addPost("post-edge", "", "明日 21時から歌枠配信します", "2026-04-01T00:00:00.000Z", "")
	f.addPost("post-old", "", "明日 21時から歌枠配信します", "2026-03-31T23:59:59.000Z", "")
	f.addPost("post-requested", "", "明日 21時から歌枠配信します", "2015-01-01T00:00:00.000Z", "")
	f.requestReprocess("post-requested")

	preview := f.preview()
	if preview.LookbackDays != 30 || preview.Candidates != 3 || preview.Later != 0 {
		t.Fatalf("preview = %+v", preview)
	}
	want := []string{"post-requested", "post-1", "post-edge"}
	if got := previewIDs(preview); !reflect.DeepEqual(got, want) {
		t.Fatalf("preview run = %v, want %v", got, want)
	}

	result := f.scan()
	if got := countOf(result.Counts, "attempted"); got != 3 || f.provider.count() != 3 {
		t.Fatalf("attempted=%d requests=%d counts=%v", got, f.provider.count(), result.Counts)
	}
	attempted := []string{}
	rows, err := f.db.Query(`SELECT post_id FROM post_llm_states WHERE last_attempt_at IS NOT NULL ORDER BY post_id`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		attempted = append(attempted, id)
	}
	rows.Close()
	if !reflect.DeepEqual(attempted, []string{"post-1", "post-edge", "post-requested"}) {
		t.Fatalf("the run processed %v, the preview listed %v", attempted, want)
	}

	f.setExtractionLookback(0)
	preview = f.preview()
	if preview.LookbackDays != 0 || !reflect.DeepEqual(previewIDs(preview), []string{"post-old"}) {
		t.Fatalf("no-limit preview = %+v", preview)
	}
}

// A post without a stored setting uses the 30-day default.
func TestExtractionLookbackDefaultsToThirtyDays(t *testing.T) {
	f := messageFixture(t)
	f.exec(`DELETE FROM app_settings WHERE key = 'sync_extraction_lookback_days'`)
	f.at(time.Date(2026, 5, 28, 3, 0, 1, 0, time.UTC))
	if preview := f.preview(); preview.LookbackDays != sqlite.DefaultSyncSettings().ExtractionLookbackDays || preview.Candidates != 0 {
		t.Fatalf("preview = %+v", preview)
	}
	f.at(time.Date(2026, 5, 28, 3, 0, 0, 0, time.UTC))
	if preview := f.preview(); preview.Candidates != 1 {
		t.Fatalf("preview = %+v", preview)
	}
}
