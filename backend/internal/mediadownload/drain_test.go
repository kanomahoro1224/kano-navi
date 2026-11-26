package mediadownload

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yexca/kano-navi/backend/internal/digest"
	"github.com/yexca/kano-navi/backend/internal/mediavariant"
	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
	"github.com/yexca/kano-navi/backend/internal/testutil"
)

// queueStore opens a snapshot with n never-checked synthetic X images.
func queueStore(t *testing.T, n int) (*sqlite.Store, string) {
	t.Helper()
	p := testutil.NewSnapshot(t)
	store, err := sqlite.OpenWorkflowCore(p, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	for i := range n {
		u := fmt.Sprintf("https://pbs.twimg.com/media/drain-%d.png", i)
		testutil.Exec(t, p, `INSERT INTO media_assets(id,source,source_url,status,created_at,updated_at) VALUES(?,'x',?,'pending','2026-01-01T00:00:00.000Z','2026-01-01T00:00:00.000Z')`, digest.SHA256Hex(u), u)
	}
	return store, p
}

// pngClient answers every request with a distinct PNG, or with the status
// fail returns for that request number (1-based) when it is not zero.
func pngClient(requests *int, fail func(int) int) *Client {
	return NewClient(&http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		*requests++
		if status := fail(*requests); status != 0 {
			return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(""))}, nil
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"image/png"}}, Body: io.NopCloser(strings.NewReader("\x89PNG\r\n\x1a\n" + r.URL.Path))}, nil
	})})
}

func never(int) int { return 0 }

func TestDownloadPendingDrainsTheQueueInBatches(t *testing.T) {
	store, _ := queueStore(t, 7)
	requests := 0
	summary, err := pngClient(&requests, never).DownloadPending(context.Background(), store, Bounds{Batch: 3, Files: 100, Budget: time.Minute})
	if err != nil || summary != (Summary{Attempted: 7, Ready: 7}) || requests != 7 {
		t.Fatalf("summary=%#v requests=%d err=%v", summary, requests, err)
	}
}

func TestDownloadPendingStopsAtTheFileCapAndReportsRemaining(t *testing.T) {
	store, _ := queueStore(t, 7)
	requests := 0
	summary, err := pngClient(&requests, never).DownloadPending(context.Background(), store, Bounds{Batch: 3, Files: 5, Budget: time.Minute})
	if err != nil || summary != (Summary{Attempted: 5, Ready: 5, Remaining: 2, Stopped: StoppedFileCap}) || requests != 5 {
		t.Fatalf("summary=%#v requests=%d err=%v", summary, requests, err)
	}
}

func TestDownloadPendingStopsAtTheTimeBudget(t *testing.T) {
	store, _ := queueStore(t, 7)
	requests := 0
	client := pngClient(&requests, never)
	// Each request takes a synthetic minute; no file starts after 150 seconds.
	start := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	client.now = func() time.Time { return start.Add(time.Duration(requests) * time.Minute) }
	summary, err := client.DownloadPending(context.Background(), store, Bounds{Batch: 2, Files: 100, Budget: 150 * time.Second})
	if err != nil || summary != (Summary{Attempted: 3, Ready: 3, Remaining: 4, Stopped: StoppedTimeBudget}) || requests != 3 {
		t.Fatalf("summary=%#v requests=%d err=%v", summary, requests, err)
	}
}

func TestDownloadPendingStopsOnRateLimit(t *testing.T) {
	store, _ := queueStore(t, 7)
	requests := 0
	limited := func(n int) int {
		if n == 2 {
			return http.StatusTooManyRequests
		}
		return 0
	}
	summary, err := pngClient(&requests, limited).DownloadPending(context.Background(), store, Bounds{Batch: 5, Files: 100, Budget: time.Minute})
	if err != nil || summary != (Summary{Attempted: 2, Ready: 1, Failed: 1, Remaining: 5, Stopped: StoppedRateLimited}) || requests != 2 {
		t.Fatalf("summary=%#v requests=%d err=%v", summary, requests, err)
	}
}

func TestDownloadPendingDoesNotRepeatAnAssetThatStaysQueued(t *testing.T) {
	store, p := queueStore(t, 0)
	// A watch page read "after" every check (a skewed clock) keeps the
	// thumbnail due for revalidation however often it answers 304.
	now := time.Now().UTC()
	testutil.Exec(t, p, `INSERT INTO videos(id,source,title,published_at,url,live_checked_at) VALUES('skewed','youtube','synthetic',?,'https://www.youtube.com/watch?v=skewed',?)`, now.Add(-72*time.Hour).Format(time.RFC3339), now.Add(time.Hour).Format(time.RFC3339))
	u := "https://i.ytimg.com/vi/skewedvideo/hqdefault.jpg"
	id := digest.SHA256Hex(u)
	testutil.Exec(t, p, `INSERT INTO media_assets(id,source,source_url,status,etag,last_checked_at,created_at,updated_at) VALUES(?,'youtube',?,'ready','"synthetic"',?,'2026-01-01T00:00:00.000Z','2026-01-01T00:00:00.000Z')`, id, u, now.Add(-time.Hour).Format(time.RFC3339))
	testutil.Exec(t, p, `INSERT INTO media_links(media_id,owner_type,owner_id,role,position,created_at,updated_at) VALUES(?,'video','skewed','thumbnail',0,'2026-01-01T00:00:00.000Z','2026-01-01T00:00:00.000Z')`, id)
	requests := 0
	client := NewClient(&http.Client{Transport: transport(func(*http.Request) (*http.Response, error) {
		requests++
		return &http.Response{StatusCode: http.StatusNotModified, Body: io.NopCloser(strings.NewReader(""))}, nil
	})})
	summary, err := client.DownloadPending(context.Background(), store, Bounds{Batch: 5, Files: 100, Budget: time.Minute})
	if err != nil || summary != (Summary{Attempted: 1, Unchanged: 1, Stopped: StoppedNoProgress}) || requests != 1 {
		t.Fatalf("summary=%#v requests=%d err=%v", summary, requests, err)
	}
	if again, _ := store.PendingMedia(context.Background(), 5); len(again) != 1 {
		t.Fatalf("fixture no longer stays queued: %#v", again)
	}
}

func TestDownloadPendingMakesCopiesWithinItsBounds(t *testing.T) {
	store, _ := queueStore(t, 3)
	requests := 0
	summary, err := pngClient(&requests, never).DownloadPending(context.Background(), store, Bounds{Batch: 3, Files: 100, Budget: time.Minute, Variants: 2, VariantBudget: time.Minute})
	if err != nil || summary.Ready != 3 || summary.Resized != 2 {
		t.Fatalf("summary=%#v err=%v", summary, err)
	}
	// The synthetic files cannot be decoded, so each width keeps its
	// original; the next step makes the image the bound left.
	markers, _ := filepath.Glob(filepath.Join(store.DataRoot(), "cache", "variants", "*", "*.orig"))
	if len(markers) != 2*len(mediavariant.Widths) {
		t.Fatalf("markers %v", markers)
	}
	summary, err = pngClient(&requests, never).DownloadPending(context.Background(), store, Bounds{Batch: 3, Files: 100, Budget: time.Minute, Variants: 2, VariantBudget: time.Minute})
	if err != nil || summary.Resized != 1 {
		t.Fatalf("backfill summary=%#v err=%v", summary, err)
	}
}

// contentPath is where the step stores bytes under data/.
func contentPath(snapshot string, content []byte) string {
	hash := digest.SHA256Hex(content)
	return filepath.Join(filepath.Dir(filepath.Dir(snapshot)), "x", "sha256", hash[:2], hash+".png")
}

// A corrupt file at an asset's content address is replaced by the verified
// download instead of blocking the asset.
func TestDownloadPendingReplacesACorruptFileAtTheContentAddress(t *testing.T) {
	store, p := queueStore(t, 1)
	content := []byte("\x89PNG\r\n\x1a\n/media/drain-0.png")
	target := contentPath(p, content)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("\x89PNG\r\n\x1a\n/media/drain-X.png"), 0o600); err != nil {
		t.Fatal(err)
	}
	requests := 0
	summary, err := pngClient(&requests, never).DownloadPending(context.Background(), store, Bounds{Batch: 3, Files: 100, Budget: time.Minute})
	if err != nil || summary != (Summary{Attempted: 1, Ready: 1}) {
		t.Fatalf("summary=%#v err=%v", summary, err)
	}
	if stored, err := os.ReadFile(target); err != nil || string(stored) != string(content) {
		t.Fatalf("stored=%q err=%v", stored, err)
	}
}

// An asset whose bytes cannot be published records a retryable failure, and
// the step goes on with the rest of the queue.
func TestDownloadPendingContinuesPastAFileItCannotPublish(t *testing.T) {
	store, p := queueStore(t, 3)
	// A directory at the first asset's content address blocks the rename.
	blocked := contentPath(p, []byte("\x89PNG\r\n\x1a\n/media/drain-0.png"))
	if err := os.MkdirAll(filepath.Join(blocked, "occupied"), 0o755); err != nil {
		t.Fatal(err)
	}
	requests := 0
	summary, err := pngClient(&requests, never).DownloadPending(context.Background(), store, Bounds{Batch: 3, Files: 100, Budget: time.Minute})
	if err != nil || summary != (Summary{Attempted: 3, Ready: 2, Failed: 1}) {
		t.Fatalf("summary=%#v err=%v", summary, err)
	}
	var status, code string
	var retryAt sql.NullString
	if err = store.DB().QueryRow(`SELECT status,last_error,next_retry_at FROM media_assets WHERE source_url='https://pbs.twimg.com/media/drain-0.png'`).Scan(&status, &code, &retryAt); err != nil {
		t.Fatal(err)
	}
	if status != "failed" || code != "media_write_failed" || !retryAt.Valid {
		t.Fatalf("status=%s code=%s retry=%v", status, code, retryAt)
	}
}
