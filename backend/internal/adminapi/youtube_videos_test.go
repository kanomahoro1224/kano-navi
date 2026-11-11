package adminapi

import (
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	ytsource "github.com/yexca/kano-navi/backend/internal/source/youtube"
	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
)

func TestYoutubeVideosAreAddedByOperators(t *testing.T) {
	store, err := sqlite.OpenInitialized(filepath.Join(t.TempDir(), "database", "kano.sqlite"), true, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	const channel = "UCAAAAAAAAAAAAAAAAAAAAAA"
	owner, title, requests := channel, "歌枠 synthetic", 0
	client := ytsource.NewClient(&http.Client{Transport: bilibiliTransport(func(r *http.Request) (*http.Response, error) {
		requests++
		if r.URL.Path != "/watch" {
			t.Fatalf("unexpected request %s", r.URL)
		}
		id := r.URL.Query().Get("v")
		body := `var ytInitialPlayerResponse={"videoDetails":{"videoId":"` + id + `","channelId":"` + owner + `","title":"` + title + `","lengthSeconds":"3600"},"playabilityStatus":{"status":"OK"},"microformat":{"playerMicroformatRenderer":{"publishDate":"2026-01-02T00:00:00+00:00"}}};`
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})})
	adapter := ytsource.NewAdapter(client, ytsource.Options{ChannelID: channel, MaxDetailRequests: 12})
	c := &console{store: store, handler: New(Options{Store: store, Auth: newAuth(t, "development", ""), Youtube: adapter})}

	code, payload := c.call(t, "POST", "/api/admin/videos/youtube", `{"input":"https://youtu.be/AAAAAAAAAAA?si=synthetic"}`)
	video, _ := payload["video"].(map[string]any)
	if code != 200 || video["id"] != "AAAAAAAAAAA" || video["source"] != "youtube" || video["title"] != "歌枠 synthetic" || video["durationSeconds"] != float64(3600) {
		t.Fatalf("add=%d %v", code, payload)
	}
	// Reading a stored video again keeps the replaced title as history.
	title = "歌枠 renamed"
	if code, payload = c.call(t, "POST", "/api/admin/videos/youtube", `{"input":"AAAAAAAAAAA"}`); code != 200 || payload["video"].(map[string]any)["title"] != "歌枠 renamed" {
		t.Fatalf("again=%d %v", code, payload)
	}
	if code, payload = c.call(t, "GET", "/api/admin/videos/AAAAAAAAAAA/history", ""); code != 200 || !strings.Contains(strings.Join(historyTitles(payload), ","), "歌枠 synthetic") {
		t.Fatalf("history=%d %v", code, payload)
	}
	before := requests
	for body, want := range map[string]int{`{"input":"https://example.invalid/watch?v=AAAAAAAAAAA"}`: 400, `{}`: 400, `{"input":"AAAAAAAAAAA","force":true}`: 400} {
		if code, payload = c.call(t, "POST", "/api/admin/videos/youtube", body); code != want {
			t.Fatalf("%s=%d %v", body, code, payload)
		}
	}
	if requests != before {
		t.Fatalf("invalid input made %d requests", requests-before)
	}
	// Another channel's video is refused and not stored.
	owner = "UCBBBBBBBBBBBBBBBBBBBBBB"
	if code, payload = c.call(t, "POST", "/api/admin/videos/youtube", `{"input":"BBBBBBBBBBB"}`); code != 400 || payload["message"] != "this video was not uploaded by the configured YouTube channel" {
		t.Fatalf("other channel=%d %v", code, payload)
	}
	if code, _ = c.call(t, "GET", "/api/admin/videos/BBBBBBBBBBB/history", ""); code != 404 {
		t.Fatalf("other channel stored: %d", code)
	}
	// A process that skips YouTube refuses before any request.
	skipped := &console{store: store, handler: New(Options{Store: store, Auth: newAuth(t, "development", ""), Youtube: adapter, SkippedSteps: []string{"youtube"}})}
	before = requests
	if code, _ = skipped.call(t, "POST", "/api/admin/videos/youtube", `{"input":"CCCCCCCCCCC"}`); code != 400 || requests != before {
		t.Fatalf("skipped=%d requests %d→%d", code, before, requests)
	}
}

// historyTitles lists the replaced titles of a video history answer.
func historyTitles(payload map[string]any) []string {
	out := []string{}
	items, _ := payload["revisions"].([]any)
	for _, item := range items {
		if entry, ok := item.(map[string]any); ok {
			if title, ok := entry["title"].(string); ok {
				out = append(out, title)
			}
		}
	}
	return out
}
