package contentadmin

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/yexca/kano-navi/backend/internal/digest"
)

func TestDiscoverBilibiliAvatarDownloadsTheAccountFace(t *testing.T) {
	f := newFixture(t, true)
	f.exec(`UPDATE profiles SET avatar_url=?`, "https://pbs.twimg.com/profile_images/9/existing.jpg")
	face, space := "https://i0.hdslb.com/bfs/face/synthetic.png", "https://space.bilibili.com/1000001"
	calls := 0
	var avatarErr error
	f.module = New(f.store, Options{HTTPClient: &http.Client{Transport: f.network}, BilibiliAvatar: func(context.Context) (string, string, error) {
		calls++
		return face, space, avatarErr
	}})
	image := syntheticPNG(1)
	f.serveImage(face, "image/png", image)

	out := f.post("/profile-media/discover", map[string]any{"source": "bilibili"})
	out.expect(t, http.StatusOK, "")
	if out.json["slot"] != "avatar" || out.json["source"] != "bilibili" || out.json["sourceUrl"] != space {
		t.Fatalf("unexpected discovery summary: %s", out.raw)
	}
	candidates := list(t, out.json["candidates"])
	if len(candidates) != 1 {
		t.Fatalf("expected only the Bilibili face, got %s", out.raw)
	}
	// The face is downloaded with the check, so it can be previewed and used
	// at once; the profile's existing avatar is not fetched.
	candidate := object(t, candidates[0])
	hash := digest.SHA256Hex(image)
	if candidate["slot"] != "avatar" || candidate["source"] != "bilibili" || candidate["sourceRef"] != space || candidate["sourceUrl"] != face ||
		candidate["status"] != "ready" || candidate["cachePath"] != "bilibili/sha256/"+hash[:2]+"/"+hash+".png" {
		t.Fatalf("unexpected candidate: %#v", candidate)
	}
	if len(f.network.visited) != 1 || !f.network.reached(face) {
		t.Fatalf("discovery fetched %v", f.network.visited)
	}
	id := candidate["id"].(string)
	preview := f.request(http.MethodGet, "/profile-media/"+id+"/preview", "", nil)
	if preview.status != http.StatusOK || !bytes.Equal(preview.raw, image) {
		t.Fatalf("preview status %d %s", preview.status, preview.raw)
	}
	selected := f.request(http.MethodPost, "/profile-media/"+id+"/select", "", nil)
	selected.expect(t, http.StatusOK, "")
	if item := object(t, selected.json["item"]); item["isActive"] != true || item["activeCachePath"] != "avatar/avatar.png" {
		t.Fatalf("unexpected selected face: %#v", item)
	}

	// A face already downloaded is not fetched again.
	f.network.visited = nil
	f.post("/profile-media/discover", map[string]any{"source": "bilibili"}).expect(t, http.StatusOK, "")
	if len(f.network.visited) != 0 {
		t.Fatalf("a ready face was fetched again: %v", f.network.visited)
	}

	f.post("/profile-media/discover", map[string]any{"slot": "banner", "source": "bilibili"}).expectInvalid(t, "bilibili only offers an avatar")
	f.post("/profile-media/discover", map[string]any{"slot": "avatar", "source": "bilibili", "handle": "kano_2525"}).expectInvalid(t, "handle is only supported for x")
	if calls != 2 {
		t.Fatalf("invalid requests reached Bilibili: %d calls", calls)
	}

	avatarErr = errors.New("bilibili_risk_control_http")
	f.post("/profile-media/discover", map[string]any{"slot": "avatar", "source": "bilibili"}).expect(t, http.StatusInternalServerError, "internal_error")
	f.module = New(f.store, Options{HTTPClient: &http.Client{Transport: f.network}})
	f.post("/profile-media/discover", map[string]any{"slot": "avatar", "source": "bilibili"}).expect(t, http.StatusInternalServerError, "internal_error")
	if f.text(`SELECT CAST(COUNT(*) AS TEXT) FROM profile_media`) != "1" {
		t.Fatal("failed discovery must not register candidates")
	}
}

func TestAFailedAutomaticDownloadIsRecordedOnTheCandidate(t *testing.T) {
	f := newFixture(t, true)
	face := "https://i0.hdslb.com/bfs/face/missing.png"
	f.module = New(f.store, Options{HTTPClient: &http.Client{Transport: f.network}, BilibiliAvatar: func(context.Context) (string, string, error) {
		return face, "https://space.bilibili.com/1000001", nil
	}})
	f.network.routes[face] = func(*http.Request) *http.Response {
		return reply(http.StatusNotFound, nil, nil)
	}
	out := f.post("/profile-media/discover", map[string]any{"source": "bilibili"})
	out.expect(t, http.StatusOK, "")
	candidate := object(t, list(t, out.json["candidates"])[0])
	if candidate["status"] != "failed" || candidate["lastError"] == nil {
		t.Fatalf("a failed download must stay on its candidate: %#v", candidate)
	}
}

func TestDiscoverYouTubeDownloadsTheImagesItFinds(t *testing.T) {
	f := newFixture(t, true)
	f.exec(`UPDATE profiles SET youtube_url=?`, "https://www.youtube.com/channel/synthetic")
	found := "https://i.ytimg.com/vi/synthetic/avatar.png"
	f.network.routes["https://www.youtube.com/channel/synthetic"] = func(*http.Request) *http.Response {
		return reply(http.StatusOK, map[string]string{"Content-Type": "text/html"}, []byte(`<meta property="og:image" content="`+found+`">`))
	}
	image := syntheticPNG(2)
	f.serveImage(found, "image/png", image)
	out := f.post("/profile-media/discover", map[string]any{"slot": "avatar", "source": "youtube"})
	out.expect(t, http.StatusOK, "")
	candidate := object(t, list(t, out.json["candidates"])[0])
	hash := digest.SHA256Hex(image)
	if candidate["sourceUrl"] != found || candidate["status"] != "ready" || candidate["cachePath"] != "youtube/sha256/"+hash[:2]+"/"+hash+".png" {
		t.Fatalf("unexpected YouTube candidate: %#v", candidate)
	}
}
