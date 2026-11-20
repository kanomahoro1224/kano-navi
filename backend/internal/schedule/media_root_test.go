package schedule

import (
	"os"
	"path/filepath"
	"testing"
)

// The cache file rules hold with each directory resolved once per scan: a
// regular file inside the data root resolves, a symlink is followed only
// while it stays inside the root, and every answer repeats from the cache.
func TestMediaRootResolvesCacheFilesInsideTheDataRoot(t *testing.T) {
	f := newFixture(t)
	root := f.store.DataRoot()
	write := func(relative string) string {
		t.Helper()
		path := filepath.Join(root, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(pngHeader), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	write("x/sha256/ab/inside.png")
	write("x/sha256/ab/target.png")
	write("cache/media/sha256/cd/shared.png")
	if err := os.MkdirAll(filepath.Join(root, "x", "sha256", "ab", "folder"), 0o755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.png")
	if err := os.WriteFile(outside, []byte(pngHeader), 0o644); err != nil {
		t.Fatal(err)
	}
	cases := map[string]bool{
		"x/sha256/ab/inside.png":      true,
		`x\sha256\ab\inside.png`:      true,
		"sha256/cd/shared.png":        true,
		"x/sha256/ab/missing.png":     false,
		"x/sha256/ab/folder":          false,
		"x/../x/sha256/ab/inside.png": false,
		"other/inside.png":            false,
		"/x/sha256/ab/inside.png":     false,
		"":                            false,
	}
	linked := os.Symlink(filepath.Join(root, "x", "sha256", "ab", "target.png"), filepath.Join(root, "x", "sha256", "ab", "link-in.png")) == nil &&
		os.Symlink(outside, filepath.Join(root, "x", "sha256", "ab", "link-out.png")) == nil &&
		os.Symlink(filepath.Dir(outside), filepath.Join(root, "x", "escape")) == nil
	if linked {
		cases["x/sha256/ab/link-in.png"] = true
		cases["x/sha256/ab/link-out.png"] = false
		cases["x/escape/outside.png"] = false
	} else {
		t.Log("symlinks unavailable; their cases are skipped")
	}
	files := f.run.mediaRoot()
	for pass := 0; pass < 2; pass++ {
		for relative, want := range cases {
			path, ok := files.file(relative)
			if ok != want {
				t.Errorf("pass %d: %q resolved=%v, want %v", pass, relative, ok, want)
			}
			if fresh, freshOK := f.run.cachedFile(relative); fresh != path || freshOK != ok {
				t.Errorf("pass %d: %q gives %q,%v, a fresh root %q,%v", pass, relative, path, ok, fresh, freshOK)
			}
		}
	}
	if path, ok := files.file("x/sha256/ab/link-in.png"); linked && (!ok || filepath.Base(path) != "target.png") {
		t.Errorf("link inside resolved to %q, %v", path, ok)
	}
}
