package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	bilisource "github.com/yexca/kano-navi/backend/internal/source/bilibili"
)

func TestTheBilibiliLoginRowSwapsOnlyFromWhatWasRead(t *testing.T) {
	store, err := OpenInitialized(filepath.Join(t.TempDir(), "database", "kano.sqlite"), true, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	ctx := context.Background()
	if value, err := store.BilibiliLoginRecord(ctx); err != nil || value != "" {
		t.Fatalf("empty=%q err=%v", value, err)
	}
	for _, step := range []struct {
		from, to string
		swapped  bool
		value    string
	}{
		{"", "v1:first", true, "v1:first"},
		{"", "v1:second", false, "v1:first"},
		{"v1:stale", "v1:second", false, "v1:first"},
		{"v1:first", "v1:second", true, "v1:second"},
		{"v1:first", "", false, "v1:second"},
		{"v1:second", "", true, ""},
	} {
		swapped, err := store.SwapBilibiliLoginRecord(ctx, step.from, step.to)
		value, _ := store.BilibiliLoginRecord(ctx)
		if err != nil || swapped != step.swapped || value != step.value {
			t.Fatalf("%q→%q: swapped=%v value=%q err=%v", step.from, step.to, swapped, value, err)
		}
	}
}

func TestBilibiliLoginChecksKeepWhichLoginTheyChecked(t *testing.T) {
	store, err := OpenInitialized(filepath.Join(t.TempDir(), "database", "kano.sqlite"), true, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	ctx := context.Background()
	now := time.Date(2026, 11, 26, 3, 0, 0, 0, time.UTC)
	state := bilisource.State{
		Login:   bilisource.LoginState{CheckedAt: now, SignedIn: true, Of: "stored:2026-11-25T00:00:00Z", Name: "synthetic"},
		Refresh: bilisource.RefreshState{Of: "stored:2026-11-25T00:00:00Z", CheckedAt: now, LastError: "bilibili_login_confirm_failed"},
	}
	if err = store.SaveBilibiliState(ctx, biliAccount, state); err != nil {
		t.Fatal(err)
	}
	// An older write keeps the newer checks.
	if err = store.SaveBilibiliState(ctx, biliAccount, bilisource.State{}); err != nil {
		t.Fatal(err)
	}
	got, err := store.BilibiliState(ctx, biliAccount)
	if err != nil || got.Login != state.Login || got.Refresh != state.Refresh {
		t.Fatalf("login=%#v refresh=%#v err=%v", got.Login, got.Refresh, err)
	}
}
