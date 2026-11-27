package contentadmin

import (
	"context"
	"net/http"
	"testing"

	"github.com/yexca/kano-navi/backend/internal/digest"
)

func TestTheBilibiliAccountIsOneOfKanosOwnAvatars(t *testing.T) {
	f := newFixture(t, true)
	face := "https://i0.hdslb.com/bfs/face/synthetic.png"
	calls := 0
	f.module = New(f.store, Options{
		HTTPClient:      &http.Client{Transport: f.network},
		BilibiliAccount: "1000001",
		BilibiliAvatar: func(context.Context) (string, string, error) {
			calls++
			return face, "https://space.bilibili.com/1000001", nil
		},
	})
	image := syntheticPNG(3)
	f.serveImage(face, "image/png", image)

	listed := f.request(http.MethodGet, "/account-avatars", "", nil)
	listed.expect(t, http.StatusOK, "")
	found := false
	for _, entry := range list(t, listed.json["items"]) {
		item := object(t, entry)
		if item["source"] == "bilibili" {
			found = item["accountKey"] == "1000001" && item["role"] == "own"
		}
	}
	if !found {
		t.Fatalf("the Bilibili account is not listed as own: %s", listed.raw)
	}

	// Fetch latest reads the face through the Bilibili client and downloads it.
	fetched := f.post("/account-avatars/bilibili/1000001/discover", map[string]any{})
	fetched.expect(t, http.StatusOK, "")
	item := object(t, fetched.json["item"])
	hash := digest.SHA256Hex(image)
	if item["source"] != "bilibili" || item["publicUrl"] == nil {
		t.Fatalf("unexpected Bilibili avatar: %s", fetched.raw)
	}
	candidate := object(t, list(t, item["candidates"])[0])
	if candidate["status"] != "ready" || candidate["sourceUrl"] != face || candidate["sha256"] != hash {
		t.Fatalf("unexpected candidate: %#v", candidate)
	}
	preview := f.request(http.MethodGet, "/account-avatars/bilibili/1000001/candidates/"+candidate["id"].(string)+"/preview", "", nil)
	if preview.status != http.StatusOK {
		t.Fatalf("preview status %d %s", preview.status, preview.raw)
	}

	// Only the configured account is read.
	f.post("/account-avatars/bilibili/2000002/discover", map[string]any{}).expect(t, http.StatusInternalServerError, "internal_error")
	if calls != 1 {
		t.Fatalf("another mid reached Bilibili: %d calls", calls)
	}
}
