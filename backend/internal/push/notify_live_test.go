package push

import (
	"bytes"
	"context"
	"log"
	"net/http"
	"strings"
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

// TestLiveEndBodyIsTheStreamDuration pins the body of a finished stream: its
// length in every language, never the stream title.
func TestLiveEndBodyIsTheStreamDuration(t *testing.T) {
	update := Update{Platform: "youtube", Kind: "live_end", Author: "鹿乃", Title: "【歌枠】夜の歌", DurationSeconds: 2*3600 + 5*60 + 8}
	want := map[string]string{
		"zh":      "直播总时长：2小时05分钟08秒",
		"zh-Hant": "直播總時長：2小時05分鐘08秒",
		"ja":      "配信時間：2時間05分08秒",
		"en":      "Total stream time: 2h 05m 08s",
	}
	for locale, body := range want {
		title, got := wording(update, locale)
		if got != body {
			t.Fatalf("%s: body = %q, want %q", locale, got, body)
		}
		if title == "" || strings.Contains(got, update.Title) {
			t.Fatalf("%s: title=%q body=%q", locale, title, got)
		}
		// A deferred pass would give the same line.
		if got = translatedBody(update, locale); got != body {
			t.Fatalf("%s: translated body = %q", locale, got)
		}
	}
}

// TestLiveEndWithoutADurationFallsBack keeps the body Push Kit requires when
// the collector did not report a length: the usual fallback, not the title.
func TestLiveEndWithoutADurationFallsBack(t *testing.T) {
	want := map[string]string{
		"zh":      "详情请在 App 中查看",
		"zh-Hant": "詳情請在 App 中查看",
		"ja":      "詳細はアプリでご確認ください",
		"en":      "Open the app for details",
	}
	for _, seconds := range []int{0, -5} {
		update := Update{Platform: "youtube", Kind: "live_end", Author: "鹿乃", Title: "【歌枠】夜の歌", DurationSeconds: seconds}
		for locale, body := range want {
			if _, got := wording(update, locale); got != body {
				t.Fatalf("%d %s: body = %q, want %q", seconds, locale, got, body)
			}
			if got := translatedBody(update, locale); got != body {
				t.Fatalf("%d %s: translated body = %q", seconds, locale, got)
			}
		}
	}
}

// TestDurationTextPadsMinutesAndSeconds keeps all three parts: hours
// unpadded, minutes and seconds two digits.
func TestDurationTextPadsMinutesAndSeconds(t *testing.T) {
	for _, tc := range []struct {
		seconds int
		locale  string
		want    string
	}{
		{3661, "zh", "直播总时长：1小时01分钟01秒"},
		{59, "zh", "直播总时长：0小时00分钟59秒"},
		{600, "zh", "直播总时长：0小时10分钟00秒"},
		{36000 + 59*60 + 9, "zh", "直播总时长：10小时59分钟09秒"},
		{3661, "zh-Hant", "直播總時長：1小時01分鐘01秒"},
		{59, "ja", "配信時間：0時間00分59秒"},
		{3661, "en", "Total stream time: 1h 01m 01s"},
		{0, "zh", ""},
	} {
		if got := durationText(tc.seconds, tc.locale); got != tc.want {
			t.Fatalf("%d %s: %q, want %q", tc.seconds, tc.locale, got, tc.want)
		}
	}
}

// TestLiveEndDispatchSendsAndLogsTheDuration follows a live end through the
// dispatcher: each language gets its length line at once, and the log names
// the body that went out.
func TestLiveEndDispatchSendsAndLogsTheDuration(t *testing.T) {
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
	var logged bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&logged)
	t.Cleanup(func() { log.SetOutput(previous) })
	dispatcher := NewDispatcher(client, store)
	update := Update{
		ID: "316381099:end:1", Platform: "bilibili", Kind: "live_end",
		Author: "鹿乃", Title: "【歌枠】夜の歌", URL: "https://live.bilibili.com/22603245", DurationSeconds: 3661,
	}
	if err = dispatcher.DispatchPosts(ctx, []Update{update}); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"token-zh":   "直播总时长：1小时01分钟01秒",
		"token-hant": "直播總時長：1小時01分鐘01秒",
		"token-ja":   "配信時間：1時間01分01秒",
		"token-en":   "Total stream time: 1h 01m 01s",
	}
	if len(sends) != 4 {
		t.Fatalf("sends = %+v", sends)
	}
	for _, send := range sends {
		for token := range send.Tokens {
			if send.Body != want[token] {
				t.Fatalf("%s: body = %q, want %q", token, send.Body, want[token])
			}
		}
	}
	text := logged.String()
	if !strings.Contains(text, "push kit sent") || !strings.Contains(text, `body="直播总时长：1小时01分钟01秒"`) || !strings.Contains(text, "title=") {
		t.Fatalf("log does not name the body: %s", text)
	}
	for token := range want {
		if strings.Contains(text, token) {
			t.Fatalf("log leaked a device token: %s", text)
		}
	}
}
