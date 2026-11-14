package httpapi

import (
	"context"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
)

func TestPublicAccountAvatarsServingAndProjections(t *testing.T) {
	ctx := context.Background()
	dataRoot := filepath.Join(t.TempDir(), "data")
	dbPath := filepath.Join(dataRoot, "database", "snapshot.sqlite")
	store, err := sqlite.OpenInitialized(dbPath, true, true)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Seed a pending X avatar and download it so it becomes active.
	rawURL := "https://pbs.twimg.com/profile_images/42/main_400x400.jpg"
	_, cand, err := store.RecordAccountAvatarURL(ctx, "x", "kano_2525", sqlite.AccountAvatarOriginSync, rawURL, false)
	if err != nil {
		t.Fatal(err)
	}
	pending, err := store.PendingMedia(ctx, 10)
	if err != nil || len(pending) != 1 {
		t.Fatalf("pending = %+v, err=%v", pending, err)
	}
	pngBytes := append([]byte("\x89PNG\r\n\x1a\n\x00\x00\x00\x0dIHDR\x00\x00\x00\x02\x00\x00\x00\x03\x08\x06\x00\x00\x00\x00\x00\x00\x00"), '1')
	if err = store.SaveMediaDownload(ctx, pending[0], pngBytes, "image/png", `"e1"`, "", "ready", ""); err != nil {
		t.Fatal(err)
	}

	// Mark a second account explicitly hidden so the public map preserves nil.
	if _, _, err = store.RecordAccountAvatarURL(ctx, "x", "hidden_one", sqlite.AccountAvatarOriginSync, "https://pbs.twimg.com/profile_images/43/h_400x400.jpg", false); err != nil {
		t.Fatal(err)
	}
	if _, err = store.SetAccountAvatarMode(ctx, "x", "hidden_one", sqlite.AccountAvatarModeHidden); err != nil {
		t.Fatal(err)
	}

	ro, err := sqlite.OpenReadOnly(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer ro.Close()
	handler := NewHandler(ro)

	// /api/dashboard exposes ready route for kano_2525 and explicit nil for hidden_one.
	status, _, dash, _ := dashboardPayload(t, handler, "/api/dashboard")
	if status != http.StatusOK {
		t.Fatalf("dashboard status = %d", status)
	}
	avatars, ok := dash["accountAvatars"].(map[string]any)
	if !ok {
		t.Fatalf("accountAvatars missing or invalid: %T", dash["accountAvatars"])
	}
	route, ok := avatars["x:kano_2525"].(string)
	if !ok || route != cand.ID[:0]+route || route == "" {
		t.Fatalf("expected string route for x:kano_2525, got %#v", avatars["x:kano_2525"])
	}
	if val, exists := avatars["x:hidden_one"]; !exists || val != nil {
		t.Fatalf("expected explicit nil for hidden account, exists=%v val=%#v", exists, val)
	}

	// /api/posts also includes accountAvatars alongside accountCounts.
	postsStatus, _, postsBody, _ := dashboardPayload(t, handler, "/api/posts?from=2026-10-01&to=2026-10-31")
	if postsStatus != http.StatusOK {
		t.Fatalf("posts status = %d", postsStatus)
	}
	postAvatars, ok := postsBody["accountAvatars"].(map[string]any)
	if !ok || postAvatars["x:kano_2525"] != route {
		t.Fatalf("posts accountAvatars = %#v", postsBody["accountAvatars"])
	}

	// GET /media/account-avatar/{id}?v={sha256} streams verified image bytes and rejects bad versions.
	rec := request(t, handler, http.MethodGet, route)
	if rec.Code != http.StatusOK || rec.Body.String() != string(pngBytes) {
		t.Fatalf("avatar media status=%d len=%d", rec.Code, rec.Body.Len())
	}
	if cc := rec.Header().Get("Cache-Control"); cc == "" || rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("unexpected headers: %v", rec.Header())
	}
	badVer := request(t, handler, http.MethodGet, "/media/account-avatar/"+cand.ID+"?v=0000")
	if badVer.Code != http.StatusNotFound {
		t.Fatalf("expected 404 on invalid version, got %d", badVer.Code)
	}
}
