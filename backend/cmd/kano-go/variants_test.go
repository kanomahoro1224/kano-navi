package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestVariantsCommandWritesOnlyTheOutputRoot(t *testing.T) {
	data, out := t.TempDir(), filepath.Join(t.TempDir(), "out")
	if err := os.MkdirAll(filepath.Join(data, "lineblog"), 0o755); err != nil {
		t.Fatal(err)
	}
	if code := runVariants([]string{"-data", data, "-out", out}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if info, err := os.Stat(out); err != nil || !info.IsDir() {
		t.Fatalf("output root not created: %v", err)
	}
	if code := runVariants([]string{"-unknown"}); code != 2 {
		t.Fatalf("unknown flag exit %d", code)
	}
}
