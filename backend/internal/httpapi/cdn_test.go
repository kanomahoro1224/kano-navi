package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
