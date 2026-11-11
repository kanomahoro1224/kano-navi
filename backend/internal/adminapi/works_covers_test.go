package adminapi

import (
	"context"
	"testing"
	"time"

	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
	"github.com/yexca/kano-navi/backend/internal/works"
)

func TestWorksCoversListsUploadsWithoutCovers(t *testing.T) {
	c := newConsole(t, "development", "", &gatedSteps{})
	// Without the packaged list the report is unavailable.
	if code, payload := c.call(t, "GET", "/api/admin/media/works-covers", ""); code != 503 || payload["error"] != "unavailable" {
		t.Fatalf("no list=%d %v", code, payload)
	}
	index := works.Index{Uploads: []works.Upload{
		{ID: "2026-01-02-bbbbbb", Date: "2026-01-02", Videos: []works.Video{{Platform: works.Niconico, ID: "sm2"}, {Platform: works.Bilibili, ID: "BV1BBBBBBBBB"}}},
		{ID: "2026-01-01-aaaaaa", Date: "2026-01-01", Videos: []works.Video{{Platform: works.Niconico, ID: "sm1"}}},
	}}
	if err := c.store.SaveWorkCoverLookup(context.Background(), sqlite.WorkVideoRef{Platform: works.Niconico, ID: "sm1"}, sqlite.WorkLookupMissing, "", "niconico_not_found", time.Now()); err != nil {
		t.Fatal(err)
	}
	c.handler = New(Options{Store: c.store, Auth: newAuth(t, "development", ""), Runs: c.runs, Works: &index, SkippedSteps: []string{"media"}})
	code, payload := c.call(t, "GET", "/api/admin/media/works-covers", "")
	if code != 200 || payload["total"] != float64(2) || payload["due"] != float64(1) || payload["uncovered"] != float64(1) || payload["covered"] != float64(0) || payload["skipped"] != true || payload["checkedAt"] == nil {
		t.Fatalf("report=%d %v", code, payload)
	}
	uploads, _ := payload["uploads"].([]any)
	if len(uploads) != 2 {
		t.Fatalf("uploads=%v", payload["uploads"])
	}
	due, gone := uploads[0].(map[string]any), uploads[1].(map[string]any)
	dueVideos, _ := due["videos"].([]any)
	if due["id"] != "2026-01-02-bbbbbb" || due["state"] != "due" || len(dueVideos) != 2 || dueVideos[0].(map[string]any)["state"] != "due" || dueVideos[1].(map[string]any)["platform"] != "bilibili" {
		t.Fatalf("due=%v", due)
	}
	goneVideo := gone["videos"].([]any)[0].(map[string]any)
	if gone["state"] != "uncovered" || goneVideo["state"] != "gone" || goneVideo["code"] != "niconico_not_found" || goneVideo["retryAt"] != nil {
		t.Fatalf("gone=%v", gone)
	}
}
