package adminapi

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yexca/kano-navi/backend/internal/variants"
)

func TestVariantStatusReadsNoOriginalAndListsAgainAfterARun(t *testing.T) {
	c := newConsole(t, "development", "", &gatedSteps{})
	// Without the originals' locations the status is unavailable.
	if code, payload := c.call(t, "GET", "/api/admin/media/variants", ""); code != 503 || payload["error"] != "unavailable" {
		t.Fatalf("no roots=%d %v", code, payload)
	}
	data := t.TempDir()
	// Not images: the status lists the files without reading them.
	archive := func(name string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Join(data, "lineblog"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(data, "lineblog", name), []byte("synthetic"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	lineblog := func(path string) map[string]any {
		t.Helper()
		code, payload := c.call(t, "GET", path, "")
		groups, _ := payload["groups"].([]any)
		if code != 200 || len(groups) != 3 || payload["checkedAt"] == nil {
			t.Fatalf("status=%d %v", code, payload)
		}
		return groups[0].(map[string]any)
	}
	archive("0123456789ab.jpg")
	c.handler = New(Options{Store: c.store, Auth: newAuth(t, "development", ""), Runs: c.runs, Variants: &variants.Roots{Data: data}})
	if group := lineblog("/api/admin/media/variants"); group["group"] != "lineblog" || group["originals"] != float64(1) || group["ready"] != float64(0) || group["unhashed"] != float64(1) {
		t.Fatalf("lineblog=%v", group)
	}
	if entries, _ := os.ReadDir(data); len(entries) != 1 {
		t.Fatalf("status wrote under the data directory: %v", entries)
	}
	// Until a run finishes the last listing is answered; refresh lists anew.
	archive("abcdefabcdef.jpg")
	if group := lineblog("/api/admin/media/variants"); group["originals"] != float64(1) {
		t.Fatalf("memo=%v", group)
	}
	if group := lineblog("/api/admin/media/variants?refresh=1"); group["originals"] != float64(2) {
		t.Fatalf("refresh=%v", group)
	}
	archive("ba9876543210.png")
	code, payload := c.call(t, "POST", "/api/admin/runs", `{"kind":"variants"}`)
	if code != 202 {
		t.Fatalf("start=%d %v", code, payload)
	}
	run := c.waitForRun(t, runID(t, payload), "completed")
	if steps, _ := run["steps"].([]any); run["kind"] != "variants" || len(steps) != 1 || steps[0] != "variants" {
		t.Fatalf("run=%v", run)
	}
	if group := lineblog("/api/admin/media/variants"); group["originals"] != float64(3) {
		t.Fatalf("after run=%v", group)
	}
}
