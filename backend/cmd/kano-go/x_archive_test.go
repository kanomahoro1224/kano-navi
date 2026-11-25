package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
)

// The check's pace shortens slowly after a run of answers, never below its
// floor, and doubles after a failure up to its ceiling, waiting at least as
// long as a refusal asks.
func TestPaceSpeedsUpSlowlyAndBacksOff(t *testing.T) {
	p := &pace{interval: 6 * time.Second, min: 4 * time.Second, max: 30 * time.Second}
	for range paceStreak - 1 {
		p.success()
	}
	if p.interval != 6*time.Second {
		t.Fatalf("shortened early: %s", p.interval)
	}
	p.success()
	if p.interval != 5400*time.Millisecond {
		t.Fatalf("after a streak: %s", p.interval)
	}
	for range 20 * paceStreak {
		p.success()
	}
	if p.interval != 4*time.Second {
		t.Fatalf("below the floor: %s", p.interval)
	}
	if wait := p.backoff(time.Minute); wait != time.Minute || p.interval != 8*time.Second {
		t.Fatalf("refusal: wait=%s interval=%s", wait, p.interval)
	}
	for range 5 {
		p.backoff(0)
	}
	if p.interval != 30*time.Second {
		t.Fatalf("above the ceiling: %s", p.interval)
	}
	for range 100 {
		if next := p.next(); next < 21*time.Second || next > 39*time.Second {
			t.Fatalf("spread outside 70-130%%: %s", next)
		}
	}
}

// A decisions file keeps each post's latest outcome and skips lines it
// cannot read; a missing file has none.
func TestReadDecisionsKeepsTheLatestOutcome(t *testing.T) {
	path := filepath.Join(t.TempDir(), "decisions.jsonl")
	if decided, err := readDecisions(path); err != nil || len(decided) != 0 {
		t.Fatalf("missing file: %v %v", decided, err)
	}
	var lines bytes.Buffer
	if err := writeDecisions(&lines, []sqlite.XReconcileOutcome{
		{ID: "1900000000000000901", Outcome: "gone"},
		{ID: "1900000000000000901", Outcome: "top_level"},
		{ID: "1900000000000000902", Outcome: "reply"},
	}); err != nil {
		t.Fatal(err)
	}
	lines.WriteString("not json\n{\"id\":\"1900000000000000903\"}\n")
	if err := os.WriteFile(path, lines.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	decided, err := readDecisions(path)
	if err != nil || len(decided) != 2 || decided["1900000000000000901"].Outcome != "top_level" {
		t.Fatalf("decided=%v err=%v", decided, err)
	}
}

// The capture's reposts are its items with a repost object, read from every
// archive page under responses/posts.
func TestCaptureRepostsListsTheMarkedItems(t *testing.T) {
	dir := t.TempDir()
	pages := filepath.Join(dir, "responses", "posts", "2020-02")
	if err := os.MkdirAll(pages, 0o755); err != nil {
		t.Fatal(err)
	}
	page := `{"items":[{"id":"1900000000000000901","repost":{"id":"1900000000000000901"}},{"id":"1900000000000000902","repost":null},{"id":"1900000000000000903"}]}`
	if err := os.WriteFile(filepath.Join(pages, "page-0001.json"), []byte(page), 0o600); err != nil {
		t.Fatal(err)
	}
	reposted, err := captureReposts(dir)
	if err != nil || len(reposted) != 1 || !reposted["1900000000000000901"] {
		t.Fatalf("reposted=%v err=%v", reposted, err)
	}
	if _, err = captureReposts(filepath.Join(dir, "missing")); err == nil {
		t.Fatal("a missing capture reads as empty")
	}
}
