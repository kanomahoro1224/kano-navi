package ingest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
)

// bilibiliLiveEvent is a live-room change as the collector reports one: the
// room's identity is the account and its session, not an upload id.
func bilibiliLiveEvent(kind string) eventRequest {
	return eventRequest{
		ID:          "316381099:update:1790000000.500000:2c9f",
		Platform:    "bilibili",
		Kind:        kind,
		PublishedAt: "2026-09-30T12:00:00Z",
		URL:         "https://live.bilibili.com/22603245",
		Author:      authorRequest{Handle: "鹿乃", DisplayName: "鹿乃", ExternalID: "316381099"},
		Text:        "【歌枠】夜の歌",
	}
}

// TestBilibiliLiveChangesAreAcceptedWithoutAStoredRow pins the contract the
// collector relies on: a live room is not an upload, so its events must be
// accepted (and therefore announced) without being rejected as a malformed
// post or written as a bogus video.
func TestBilibiliLiveChangesAreAcceptedWithoutAStoredRow(t *testing.T) {
	store, err := sqlite.OpenInitialized(filepath.Join(t.TempDir(), "snapshot.sqlite"), true, true)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Unix(1_790_000_000, 0).UTC()
	module := &Module{store: store, secret: []byte("shared-secret"), tolerance: 5 * time.Minute, now: func() time.Time { return now }}
	for _, kind := range []string{"live_schedule", "live_start", "live_end", "live_update"} {
		event := bilibiliLiveEvent(kind)
		result, reject := module.ingest(t.Context(), event)
		if reject != nil {
			t.Fatalf("%s rejected: %+v", kind, reject)
		}
		if result.Duplicate || result.Revision != 0 {
			t.Fatalf("%s: a live change stores nothing, got %+v", kind, result)
		}
	}
	var posts, videos int
	if err = store.DB().QueryRow(`SELECT COUNT(*) FROM posts`).Scan(&posts); err != nil {
		t.Fatal(err)
	}
	if err = store.DB().QueryRow(`SELECT COUNT(*) FROM videos`).Scan(&videos); err != nil {
		t.Fatal(err)
	}
	if posts != 0 || videos != 0 {
		t.Fatalf("a live change must store nothing: posts=%d videos=%d", posts, videos)
	}
}

// TestServeHTTPAnnouncesBilibiliLiveChanges walks the route the collector
// calls and checks that a live change is counted as accepted, which is what
// releases its notification and clears the collector's outbox.
func TestServeHTTPAnnouncesBilibiliLiveChanges(t *testing.T) {
	store, err := sqlite.OpenInitialized(filepath.Join(t.TempDir(), "snapshot.sqlite"), true, true)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Unix(1_790_000_000, 0).UTC()
	module := &Module{store: store, secret: []byte("shared-secret"), tolerance: 5 * time.Minute, now: func() time.Time { return now }}
	body, err := json.Marshal(map[string]any{
		"schema": 1, "batch_id": "b1", "source": "bambinostatus",
		"events": []eventRequest{bilibiliLiveEvent("live_update")},
	})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/internal/ingest/bambino", strings.NewReader(string(body)))
	request.Header.Set("X-Bambino-Timestamp", "1790000000")
	request.Header.Set("X-Bambino-Signature", "sha256="+signatureFor("shared-secret", "1790000000", body))
	recorder := httptest.NewRecorder()
	module.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	var response batchResponse
	if err = json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Accepted != 1 || len(response.Rejected) != 0 || response.Duplicates != 0 {
		t.Fatalf("response = %+v", response)
	}
}
