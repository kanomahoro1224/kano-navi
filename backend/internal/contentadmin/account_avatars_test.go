package contentadmin

import (
	"context"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
)

func TestAccountAvatarsManagementWorkflow(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store, err := sqlite.OpenInitialized(filepath.Join(base, "database", "kano.sqlite"), true, true)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()
	if _, err = store.SetXRelatedAccounts(ctx, []sqlite.XRelatedAccount{{Handle: "partner_x", Keywords: []string{"kano"}}}, []string{"kano_2525", "_Kanotic"}); err != nil {
		t.Fatal(err)
	}

	network := &recorder{routes: map[string]func(*http.Request) *http.Response{}}
	avatarA := "https://pbs.twimg.com/profile_images/101/kano_400x400.png"
	avatarB := "https://pbs.twimg.com/profile_images/102/partner_400x400.png"
	pngA := syntheticPNG('A')
	pngB := syntheticPNG('B')
	pngUpload := syntheticPNG('U')

	network.routes["https://api.fxtwitter.com/kano_2525"] = func(*http.Request) *http.Response {
		return reply(http.StatusOK, map[string]string{"Content-Type": "application/json"}, []byte(`{"code":200,"user":{"avatar_url":"https://pbs.twimg.com/profile_images/101/kano_normal.png"}}`))
	}
	network.routes[avatarA] = func(*http.Request) *http.Response {
		return reply(http.StatusOK, map[string]string{"Content-Type": "image/png"}, pngA)
	}
	network.routes[avatarB] = func(*http.Request) *http.Response {
		return reply(http.StatusOK, map[string]string{"Content-Type": "image/png"}, pngB)
	}

	mod := New(store, Options{
		HTTPClient: &http.Client{Transport: network},
		XHandles:   []string{"kano_2525", "_Kanotic"},
	})
	f := &fixture{t: t, store: store, module: mod, dataRoot: store.DataRoot(), network: network}

	// GET /account-avatars surfaces own XHandles and configured related accounts.
	listed := f.request(http.MethodGet, "/account-avatars", "", nil)
	listed.expect(t, http.StatusOK, "")
	rawItems := listed.json["items"].([]any)
	if len(rawItems) != 3 {
		t.Fatalf("expected 3 account avatar items, got %d", len(rawItems))
	}
	first := rawItems[0].(map[string]any)
	second := rawItems[1].(map[string]any)
	third := rawItems[2].(map[string]any)
	if first["accountKey"] != "kano_2525" || first["role"] != "own" || second["accountKey"] != "_kanotic" || third["accountKey"] != "partner_x" || third["role"] != "related" {
		t.Fatalf("unexpected item ordering/roles: %#v", rawItems)
	}

	// Discover kano_2525 via FxTwitter JSON -> downloads PNG and activates automatically.
	disc := f.post("/account-avatars/x/kano_2525/discover", map[string]any{})
	disc.expect(t, http.StatusOK, "")
	discItem := disc.json["item"].(map[string]any)
	if discItem["mode"] != "auto" || discItem["publicUrl"] == nil {
		t.Fatalf("expected active publicUrl after discover, got %#v", discItem)
	}
	cands := discItem["candidates"].([]any)
	if len(cands) != 1 {
		t.Fatalf("candidates = %+v", cands)
	}
	candA := cands[0].(map[string]any)
	if candA["sourceUrl"] != avatarA || candA["status"] != "ready" {
		t.Fatalf("candA = %#v", candA)
	}
	preview := f.request(http.MethodGet, "/account-avatars/x/kano_2525/candidates/"+candA["id"].(string)+"/preview", "", nil)
	if preview.status != http.StatusOK || string(preview.raw) != string(pngA) {
		t.Fatalf("preview status=%d len=%d", preview.status, len(preview.raw))
	}

	// Reject untrusted remote hosts when adding a URL manually.
	f.post("/account-avatars/x/kano_2525/url", map[string]any{"sourceUrl": "https://untrusted.invalid/a.png"}).expectInvalid(t, "sourceUrl must be an allowed HTTPS avatar URL")

	// Add URL for related account -> downloads and activates manually.
	added := f.post("/account-avatars/x/partner_x/url", map[string]any{"sourceUrl": avatarB})
	added.expect(t, http.StatusCreated, "")
	addedItem := added.json["item"].(map[string]any)
	if addedItem["mode"] != "manual" || addedItem["publicUrl"] == nil {
		t.Fatalf("addedItem = %#v", addedItem)
	}

	// Upload custom avatar for _Kanotic -> stores verified PNG, exposes sha256, and locks to manual.
	uploaded := f.request(http.MethodPost, "/account-avatars/x/_Kanotic/upload", "image/png", pngUpload)
	uploaded.expect(t, http.StatusCreated, "")
	upItem := uploaded.json["item"].(map[string]any)
	if upItem["mode"] != "manual" || upItem["publicUrl"] == nil {
		t.Fatalf("upItem = %#v", upItem)
	}
	upCandA := upItem["candidates"].([]any)[0].(map[string]any)
	shaUploadA, ok := upCandA["sha256"].(string)
	if !ok || shaUploadA == "" {
		t.Fatalf("expected non-empty sha256 on uploaded candidate, got %#v", upCandA)
	}

	// Apply _Kanotic uploaded avatar A, then upload a second distinct avatar B -> hashes stay distinct.
	appliedUploadA := f.post("/account-avatars/x/_Kanotic/apply-profile", map[string]any{})
	appliedUploadA.expect(t, http.StatusOK, "")
	activeUploadA := appliedUploadA.json["profile"].(map[string]any)["active"].(map[string]any)["avatar"].(map[string]any)
	if activeUploadA["activeSha256"] != shaUploadA {
		t.Fatalf("expected activeSha256=%q, got %#v", shaUploadA, activeUploadA)
	}
	uploadedB := f.request(http.MethodPost, "/account-avatars/x/_Kanotic/upload", "image/png", syntheticPNG('V'))
	uploadedB.expect(t, http.StatusCreated, "")
	upCandB := uploadedB.json["item"].(map[string]any)["candidates"].([]any)[0].(map[string]any)
	if upCandB["sha256"] == shaUploadA {
		t.Fatalf("expected second uploaded candidate to have distinct sha256 from %q", shaUploadA)
	}

	// Apply kano_2525 avatar as global site profile avatar.
	applied := f.post("/account-avatars/x/kano_2525/apply-profile", map[string]any{})
	applied.expect(t, http.StatusOK, "")
	profileData := applied.json["profile"].(map[string]any)
	active := profileData["active"].(map[string]any)
	if active["avatar"] == nil {
		t.Fatalf("expected active profile avatar after apply-profile, got %#v", active)
	}

	// Mode switching to hidden clears publicUrl, then back to auto restores newest ready.
	hidden := f.put("/account-avatars/x/kano_2525/mode", map[string]any{"mode": "hidden"})
	hidden.expect(t, http.StatusOK, "")
	hiddenItem := hidden.json["item"].(map[string]any)
	if hiddenItem["mode"] != "hidden" || hiddenItem["publicUrl"] != nil {
		t.Fatalf("expected hidden item without publicUrl, got %#v", hiddenItem)
	}
	restored := f.put("/account-avatars/x/kano_2525/mode", map[string]any{"mode": "auto"})
	restored.expect(t, http.StatusOK, "")
	restoredItem := restored.json["item"].(map[string]any)
	if restoredItem["mode"] != "auto" || restoredItem["publicUrl"] == nil {
		t.Fatalf("expected restored auto publicUrl, got %#v", restoredItem)
	}
}
