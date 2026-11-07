package push

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
)

// TestLiveAndVideoUpdatesGoOutAtOnceInEveryLanguage pins the second decision
// of the dispatch: only a post's wording is translated, so only a post waits.
// A live room's title reaches every device immediately.
func TestLiveAndVideoUpdatesGoOutAtOnceInEveryLanguage(t *testing.T) {
	ctx := context.Background()
	store := notifyStore(t)
	for _, device := range []struct{ token, locale string }{
		{"token-ja", "ja-JP"}, {"token-en", "en-US"}, {"token-zh", "zh-Hans"}, {"token-hant", "zh-Hant"},
	} {
		if err := store.RegisterPushDevice(ctx, sqlite.PushDevice{
			Token: device.token, DeviceID: "device-" + device.token, Locale: device.locale,
			Enabled: true, NotifyLive: true, NotifyVideo: true, NotifyPost: true,
		}); err != nil {
			t.Fatal(err)
		}
	}
	sends := []captureSend{}
	account, _ := testAccount(t)
	client, err := New(Options{Account: account, Client: &http.Client{Transport: notifyTransport(t, &sends)}})
	if err != nil {
		t.Fatal(err)
	}
	dispatcher := NewDispatcher(client, store)
	update := Update{
		ID: "316381099:update:1790000000.500000:2c9f", Platform: "bilibili", Kind: "live_update",
		Author: "鹿乃", Text: "【歌枠】夜の歌", URL: "https://live.bilibili.com/22603245",
	}
	if err = dispatcher.DispatchPosts(ctx, []Update{update}); err != nil {
		t.Fatal(err)
	}
	if len(sends) != 4 {
		t.Fatalf("every language must be notified at once: %+v", sends)
	}
	seen := map[string]bool{}
	for _, send := range sends {
		for token := range send.Tokens {
			seen[token] = true
		}
		if send.Body != "【歌枠】夜の歌" {
			t.Fatalf("body = %q", send.Body)
		}
	}
	if len(seen) != 4 {
		t.Fatalf("tokens = %+v", seen)
	}
	items, err := store.PendingPushes(ctx, time.Now(), pendingGrace, maxFanout)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("a live change is never deferred: %+v", items)
	}
}

// TestWordingAlwaysCarriesABody guards the payload Push Kit accepts: a change
// without a title or text still says what happened, in the reader's language.
func TestWordingAlwaysCarriesABody(t *testing.T) {
	update := Update{Platform: "bilibili", Kind: "live_update", Author: "鹿乃"}
	for _, locale := range []string{"ja", "en", "zh", "zh-Hant"} {
		title, body := wording(update, locale)
		if title == "" || body == "" {
			t.Fatalf("%s: title=%q body=%q", locale, title, body)
		}
	}
	if body := func() string { _, value := wording(update, "zh"); return value }(); body != "直播间信息有更新" {
		t.Fatalf("body = %q", body)
	}
	// An unknown change still carries a body rather than an empty one.
	if _, body := wording(Update{Platform: "x", Kind: "post", Author: "鹿乃"}, "en"); body == "" {
		t.Fatal("an update without wording left the body empty")
	}
}
