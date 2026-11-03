package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestPendingPushesWaitForTranslationOrGrace(t *testing.T) {
	ctx := context.Background()
	store, err := OpenInitialized(filepath.Join(t.TempDir(), "database", "kano.sqlite"), true, true)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err = store.DB().Exec(`INSERT INTO posts (id, source, type, text, published_at, url) VALUES ('p1', 'x', 'daily', 'こんばんは', '2026-10-01T00:00:00.000Z', 'https://x.example.invalid/p1')`); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	save := func(id string, created time.Time) {
		t.Helper()
		if err = store.SavePendingPushes(ctx, []PendingPush{{
			PostID: id, Platform: "twitter", Kind: "post", Author: "鹿乃",
			Text: "こんばんは", URL: "https://x.example.invalid/" + id, CreatedAt: ISOTime(created),
		}}); err != nil {
			t.Fatal(err)
		}
	}
	pending := func() []PendingPush {
		t.Helper()
		items, err := store.PendingPushes(ctx, now, 10*time.Minute, 8)
		if err != nil {
			t.Fatal(err)
		}
		return items
	}
	save("p1", now)
	items := pending()
	if len(items) != 1 || items[0].Ready {
		t.Fatalf("a fresh post must wait for its translation: %+v", items)
	}

	result := MachineTranslation{OwnerType: TranslationOwnerPost, OwnerID: "p1", SourceHash: TranslationHash("こんばんは"), SourceLanguage: "ja", Texts: map[string]string{"en": "Good evening", "zh-CN": "晚上好"}}
	if saved, err := store.SaveMachineTranslation(ctx, result, now); err != nil || !saved {
		t.Fatalf("saved=%v err=%v", saved, err)
	}
	items = pending()
	if len(items) != 1 || !items[0].Ready {
		t.Fatalf("a settled translation must release the post: %+v", items)
	}
	sets, err := store.PostTranslationSets(ctx, []string{"p1"})
	if err != nil {
		t.Fatal(err)
	}
	if got := sets["p1"].Locales["zh-CN"].Text; got != "晚上好" {
		t.Fatalf("zh-CN body = %q", got)
	}
	if got := sets["p1"].Locales["en"].Text; got != "Good evening" {
		t.Fatalf("en body = %q", got)
	}

	// A post whose translation never runs falls back to its own wording after
	// the grace period; a re-ingested post restarts that wait.
	save("p1", now.Add(-11*time.Minute))
	if items = pending(); len(items) != 1 || !items[0].Ready {
		t.Fatalf("an expired wait must release the post: %+v", items)
	}
	if err = store.DeletePendingPushes(ctx, []string{"p1"}); err != nil {
		t.Fatal(err)
	}
	if items = pending(); len(items) != 0 {
		t.Fatalf("deleted rows = %+v", items)
	}
}
