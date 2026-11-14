package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yexca/kano-navi/backend/internal/lineblog"
	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
	"github.com/yexca/kano-navi/backend/internal/testutil"
)

// TestPublicReadsPutMediaOnTheCDN stores a post with one ready cached picture
// and reads its archive with and without CDN_PUBLIC_BASE. The projection is
// the same, so only the media route's base changes.
func TestPublicReadsPutMediaOnTheCDN(t *testing.T) {
	path := testutil.NewSnapshot(t)
	root := filepath.Dir(filepath.Dir(path))
	body := []byte("synthetic-image-bytes")
	sum := sha256.Sum256(body)
	hash := hex.EncodeToString(sum[:])
	id := strings.Repeat("c", 64)
	relative := filepath.ToSlash(filepath.Join("x", "sha256", hash[:2], hash+".jpg"))
	full := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, body, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(full) })
	testutil.Exec(t, path, `
		INSERT INTO media_assets (id,source,source_url,cache_path,mime_type,byte_size,sha256,status,fetched_at,created_at,updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,'2026-01-01T00:00:00.000Z','2026-01-01T00:00:00.000Z')`,
		id, "x", "https://source.example.invalid/source.jpg", relative, "image/jpeg", len(body), hash, "ready", "2026-10-10T00:00:00Z")
	testutil.Exec(t, path, `
		INSERT INTO posts (id,source,account_handle,type,label,text,published_at,url,likes,reposts,replies)
		VALUES ('post-1','x','kano_2525','daily','label','hello','2026-10-10T12:00:00.000Z','https://x.com/kano_2525/status/1',1,2,3)`)
	testutil.Exec(t, path, `
		INSERT INTO media_links (media_id,owner_type,owner_id,role,position,alt,created_at,updated_at,kind)
		VALUES (?,'post','post-1','post-image',0,'alt','2026-10-10T00:00:00.000Z','2026-10-10T00:00:00.000Z','photo')`, id)

	store, err := sqlite.OpenReadOnly(path)
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer store.Close()

	route := "/media/" + id + "?v=" + hash
	cdnRoute := "https://cdn.example.invalid/kano" + route
	const archive = "/api/posts?from=2026-10-01&to=2026-10-31"

	cdns := NewStandaloneHandler(store, nil, Standalone{CDNBase: "https://cdn.example.invalid/kano"})
	rewritten := request(t, cdns, http.MethodGet, archive)
	if rewritten.Code != http.StatusOK || !strings.Contains(rewritten.Body.String(), `"url":"`+cdnRoute+`"`) {
		t.Fatalf("status=%d body=%s", rewritten.Code, rewritten.Body)
	}
	if strings.Contains(rewritten.Body.String(), `"url":"/media/`) {
		t.Fatalf("a media route stayed at the origin: %s", rewritten.Body)
	}

	// Without CDN_PUBLIC_BASE every read keeps the origin routes.
	origin := NewStandaloneHandler(store, nil, Standalone{})
	plain := request(t, origin, http.MethodGet, archive)
	if plain.Code != http.StatusOK || !strings.Contains(plain.Body.String(), `"url":"`+route+`"`) {
		t.Fatalf("status=%d body=%s", plain.Code, plain.Body)
	}
	if strings.Contains(plain.Body.String(), "https://cdn.example.invalid") {
		t.Fatalf("an unconfigured deployment named a CDN: %s", plain.Body)
	}

	// The ETag covers the rewritten body, so a CDN deployment and an origin
	// deployment never validate each other's cached reads.
	if rewritten.Header().Get("ETag") == plain.Header().Get("ETag") {
		t.Fatal("the CDN read shares the origin read's ETag")
	}
}

// TestDashboardPutsDefaultProfileImagesOnTheCDN reads a dashboard whose
// profile still names the build's default avatar and banner. With a CDN base
// those fixed public images come from it too, as the frontend's own fallbacks
// do, so the page never fetches them from both hosts.
func TestDashboardPutsDefaultProfileImagesOnTheCDN(t *testing.T) {
	source := testutil.NewSnapshot(t)
	site := t.TempDir()
	path := filepath.Join(site, "data", "db", "snapshot.sqlite")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
	// The store looks for /assets/ files in the public directory beside data/.
	for _, name := range []string{"kano-avatar.jpg", "kano-banner.jpg"} {
		file := filepath.Join(site, "public", "assets", name)
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte("synthetic"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	testutil.Exec(t, path, `
		INSERT INTO profiles (id,display_name,avatar_url,banner_url,updated_at)
		VALUES ('p','Synthetic','/assets/kano-avatar.jpg','/assets/kano-banner.jpg','2026-01-02T00:00:00.000Z')`)
	store, err := sqlite.OpenReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	plain := request(t, NewStandaloneHandler(store, nil, Standalone{}), http.MethodGet, "/api/dashboard")
	if plain.Code != http.StatusOK || !strings.Contains(plain.Body.String(), `"/assets/kano-avatar.jpg"`) ||
		!strings.Contains(plain.Body.String(), `"/assets/kano-banner.jpg"`) {
		t.Fatalf("status=%d body=%s", plain.Code, plain.Body)
	}
	for _, handler := range []http.Handler{
		NewStandaloneHandler(store, nil, Standalone{CDNBase: "https://cdn.example.invalid/kano"}),
		// Read-only mode takes the same base.
		NewHandlerWithAdmin(store, nil, nil).WithCDNBase("https://cdn.example.invalid/kano"),
	} {
		cdns := request(t, handler, http.MethodGet, "/api/dashboard")
		body := cdns.Body.String()
		if cdns.Code != http.StatusOK || !strings.Contains(body, `"https://cdn.example.invalid/kano/assets/kano-avatar.jpg"`) ||
			!strings.Contains(body, `"https://cdn.example.invalid/kano/assets/kano-banner.jpg"`) || strings.Contains(body, `"/assets/`) {
			t.Fatalf("status=%d body=%s", cdns.Code, body)
		}
		if cdns.Header().Get("ETag") == plain.Header().Get("ETag") {
			t.Fatal("the CDN dashboard shares the origin dashboard's ETag")
		}
	}
}

// TestPackagedArchivesPutMediaOnTheCDN reads the packaged Journey, Works and
// LINE BLOG archives with and without a CDN base, in standalone and read-only
// mode: only the media routes move, and the ETag follows the body as sent.
func TestPackagedArchivesPutMediaOnTheCDN(t *testing.T) {
	syntheticLineblogMedia(t)
	store, _ := openFixture(t)
	const base = "https://cdn.example.invalid/kano"
	origin := NewStandaloneHandler(store, nil, Standalone{})
	for _, handler := range []http.Handler{
		NewStandaloneHandler(store, nil, Standalone{CDNBase: base}),
		NewHandlerWithAdmin(store, nil, nil).WithCDNBase(base),
	} {
		for path, media := range map[string]string{
			"/api/journey":            `"` + base + `/media/journey/`,
			"/api/works":              `"` + base + `/media/works/`,
			"/api/lineblog":           `"` + base + `/media/lineblog/0123456789ab.jpg"`,
			"/api/lineblog/posts/200": `"` + base + `/media/lineblog/abcdef012345.jpg"`,
		} {
			plain := request(t, origin, http.MethodGet, path)
			cdns := request(t, handler, http.MethodGet, path)
			if plain.Code != http.StatusOK || cdns.Code != http.StatusOK {
				t.Fatalf("%s status=%d/%d", path, plain.Code, cdns.Code)
			}
			if !strings.Contains(cdns.Body.String(), media) || strings.Contains(cdns.Body.String(), `"/media/`) {
				t.Fatalf("%s kept media at the origin: %.400s", path, cdns.Body)
			}
			if strings.Contains(plain.Body.String(), "cdn.example.invalid") {
				t.Fatalf("%s named a CDN without one: %.400s", path, plain.Body)
			}
			etag := cdns.Header().Get("ETag")
			if etag == "" || etag == plain.Header().Get("ETag") || etag != revalidatedETag(cdns.Body.Bytes()) {
				t.Fatalf("%s ETag %q does not follow the rewritten body (origin %q)", path, etag, plain.Header().Get("ETag"))
			}
			// Wording that only mentions a route is left alone.
			if path == "/api/lineblog/posts/200" && !strings.Contains(cdns.Body.String(), `"caption":"see /media/lineblog/abcdef012345.jpg"`) {
				t.Fatalf("caption changed: %s", cdns.Body)
			}
			// The same document apart from the base.
			if strings.ReplaceAll(cdns.Body.String(), `"`+base+`/media/`, `"/media/`) != plain.Body.String() {
				t.Fatalf("%s changed more than its media routes", path)
			}
			again := httptest.NewRequest(http.MethodGet, path, nil)
			again.Header.Set("If-None-Match", etag)
			revalidated := httptest.NewRecorder()
			handler.ServeHTTP(revalidated, again)
			if revalidated.Code != http.StatusNotModified {
				t.Fatalf("%s revalidation status=%d", path, revalidated.Code)
			}
		}
	}
}

// syntheticLineblogMedia is a one-article archive that names recovered media
// in its index and its article.
func syntheticLineblogMedia(t *testing.T) {
	t.Helper()
	content, err := json.Marshal(map[string]any{
		"updatedAt": "2019-01-02", "originalBlogUrl": "https://blog.example.test/",
		"posts": []any{map[string]any{"id": "200", "published": "2019-01-01T00:00:00.000Z", "title": map[string]any{"ja": "新"},
			"thumbnail": "/media/lineblog/0123456789ab.jpg", "commentCount": 0}},
		"months": []any{map[string]any{"month": "2019-01", "total": 1}},
		"articles": []any{map[string]any{"id": "200", "previousId": nil, "nextId": nil, "comments": []any{}, "post": map[string]any{
			"id": "200", "blocks": []any{map[string]any{"type": "image", "url": "/media/lineblog/abcdef012345.jpg", "caption": "see /media/lineblog/abcdef012345.jpg"}},
		}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	archive := lineblog.Parse(content)
	previous := lineblogArchiveData
	lineblogArchiveData = func() lineblog.Archive { return archive }
	t.Cleanup(func() { lineblogArchiveData = previous })
}
