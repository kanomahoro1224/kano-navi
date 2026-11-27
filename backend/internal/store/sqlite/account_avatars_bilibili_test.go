package sqlite

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeAccountAvatarKeyTakesABilibiliMid(t *testing.T) {
	for _, tc := range []struct {
		source, handle string
		ok             bool
	}{
		{"bilibili", "1000001", true},
		{" Bilibili ", " 1000001 ", true},
		{"bilibili", "@1000001", false},
		{"bilibili", "kano", false},
		{"bilibili", "", false},
		{"bilibili", strings.Repeat("1", 21), false},
		{"youtube", "1000001", false},
	} {
		src, key, display, ok := NormalizeAccountAvatarKey(tc.source, tc.handle)
		if ok != tc.ok || (ok && (src != "bilibili" || key != "1000001" || display != "1000001")) {
			t.Errorf("%q %q = %q %q %q %v", tc.source, tc.handle, src, key, display, ok)
		}
	}
}

func TestABilibiliFaceBecomesTheAccountsPublicAvatar(t *testing.T) {
	ctx := context.Background()
	dataRoot := filepath.Join(t.TempDir(), "data")
	store, err := OpenInitialized(filepath.Join(dataRoot, "database", "snapshot.sqlite"), true, true)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	face := "https://i0.hdslb.com/bfs/face/synthetic.jpg"
	_, cand, err := store.RecordAccountAvatarURL(ctx, "bilibili", "1000001", AccountAvatarOriginSync, face, false)
	if err != nil || cand.Status != "pending" {
		t.Fatalf("record cand=%+v err=%v", cand, err)
	}
	// The face is a Bilibili asset, so the media step stores it under
	// data/bilibili/.
	pending, err := store.PendingMedia(ctx, 10)
	if err != nil || len(pending) != 1 || pending[0].Source != "bilibili" || pending[0].SourceURL != face {
		t.Fatalf("pending=%+v err=%v", pending, err)
	}
	content, hash := testPNGBytes('B')
	if err = store.SaveMediaDownload(ctx, pending[0], content, "image/png", "", "", "ready", ""); err != nil {
		t.Fatal(err)
	}
	public, err := store.AccountAvatars(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := "/media/account-avatar/" + cand.ID + "?v=" + hash
	if got := public["bilibili:1000001"]; got == nil || *got != want {
		t.Fatalf("public avatars=%v", public)
	}
	stream, err := store.OpenAccountAvatarMedia(ctx, cand.ID, hash)
	if err != nil {
		t.Fatalf("the face is not served: %v", err)
	}
	stream.File.Close()
}
