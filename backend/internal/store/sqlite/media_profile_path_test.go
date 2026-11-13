package sqlite

import (
	"os"
	"path/filepath"
	"testing"
)

// Profile media resolve only inside a real data/avatar/ directory, while the
// data root itself may be reached through a symlink, as macOS reaches its
// temporary directories through /var.
func TestResolveProfileMediaPathAllowsALinkedDataRootButNotALinkedAvatarRoot(t *testing.T) {
	base := t.TempDir()
	real := filepath.Join(base, "real")
	outside := filepath.Join(base, "outside")
	for _, dir := range []string{filepath.Join(real, "avatar"), outside} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, file := range []string{filepath.Join(real, "avatar", "avatar.png"), filepath.Join(outside, "avatar.png")} {
		if err := os.WriteFile(file, []byte("synthetic"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if path, err := (&Store{dataRoot: real}).resolveProfileMediaPath("avatar/avatar.png"); err != nil || filepath.Base(path) != "avatar.png" {
		t.Fatalf("real data root: path=%q err=%v", path, err)
	}
	// Links are optional on the host: Windows needs a privilege for them.
	linkedData := filepath.Join(base, "linked-data")
	if os.Symlink(real, linkedData) != nil {
		t.Skip("symlinks unavailable")
	}
	if path, err := (&Store{dataRoot: linkedData}).resolveProfileMediaPath("avatar/avatar.png"); err != nil || filepath.Base(path) != "avatar.png" {
		t.Fatalf("linked data root: path=%q err=%v", path, err)
	}
	linkedAvatar := filepath.Join(base, "linked-avatar")
	if err := os.MkdirAll(linkedAvatar, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(linkedAvatar, "avatar")); err != nil {
		t.Fatal(err)
	}
	if path, err := (&Store{dataRoot: linkedAvatar}).resolveProfileMediaPath("avatar/avatar.png"); err == nil {
		t.Fatalf("symlinked avatar root resolved to %q", path)
	}
}
