package push

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
)

// notifyStore opens a fresh writable database with the full schema.
func notifyStore(t *testing.T) *sqlite.Store {
	t.Helper()
	store, err := sqlite.OpenInitialized(filepath.Join(t.TempDir(), "snapshot.sqlite"), true, true)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

// captureSend is one push request's audience and second line.
type captureSend struct {
	Tokens map[string]bool
	Body   string
}

// notifyTransport records each request the client sends and accepts it.
func notifyTransport(t *testing.T, sends *[]captureSend) http.RoundTripper {
	t.Helper()
	return roundTripFunc(func(request *http.Request) (*http.Response, error) {
		raw, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		var payload struct {
			Payload struct {
				Notification struct {
					Body string `json:"body"`
				} `json:"notification"`
			} `json:"payload"`
			Target struct {
				Token []string `json:"token"`
			} `json:"target"`
		}
		if err = json.Unmarshal(raw, &payload); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		tokens := map[string]bool{}
		for _, token := range payload.Target.Token {
			tokens[token] = true
		}
		*sends = append(*sends, captureSend{Tokens: tokens, Body: payload.Payload.Notification.Body})
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"code":"80000000"}`)),
			Request:    request,
		}, nil
	})
}

func TestDispatchPostsSendsJapaneseNowAndOthersAfterTranslation(t *testing.T) {
	ctx := context.Background()
	store := notifyStore(t)
	if _, err := store.DB().Exec(`INSERT INTO posts (id, source, type, text, published_at, url) VALUES ('p1', 'x', 'daily', 'こんばんは', '2026-10-01T00:00:00.000Z', 'https://x.example.invalid/p1')`); err != nil {
		t.Fatal(err)
	}
	for token, locale := range map[string]string{"token-ja": "ja-JP", "example-en": "en-US", "token-zh": "zh-Hans", "token-hant": "zh-Hant"} {
		if err := store.RegisterPushDevice(ctx, sqlite.PushDevice{
			Token: token, DeviceID: "device-" + token, Locale: locale,
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
	update := Update{ID: "p1", Platform: "twitter", Kind: "post", Author: "鹿乃", Text: "こんばんは", URL: "https://x.example.invalid/p1"}

	if err = dispatcher.DispatchPosts(ctx, []Update{update}); err != nil {
		t.Fatal(err)
	}
	if len(sends) != 1 || !sends[0].Tokens["token-ja"] || len(sends[0].Tokens) != 1 || sends[0].Body != "こんばんは" {
		t.Fatalf("the Japanese device must be the only one notified now: %+v", sends)
	}

	// The translation has not run, so the deferred languages keep waiting.
	if err = dispatcher.FlushPending(ctx); err != nil {
		t.Fatal(err)
	}
	if len(sends) != 1 {
		t.Fatalf("an unsettled post must not flush: %+v", sends)
	}
	result := sqlite.MachineTranslation{OwnerType: sqlite.TranslationOwnerPost, OwnerID: "p1", SourceHash: sqlite.TranslationHash("こんばんは"), SourceLanguage: "ja", Texts: map[string]string{"en": "Good evening", "zh-CN": "晚上好"}}
	if saved, err := store.SaveMachineTranslation(ctx, result, time.Now()); err != nil || !saved {
		t.Fatalf("saved=%v err=%v", saved, err)
	}

	if err = dispatcher.FlushPending(ctx); err != nil {
		t.Fatal(err)
	}
	if len(sends) != 4 {
		t.Fatalf("each deferred language must receive one send: %+v", sends)
	}
	seen := map[string]string{}
	for _, send := range sends[1:] {
		for token := range send.Tokens {
			seen[token] = send.Body
		}
		if send.Tokens["token-ja"] {
			t.Fatalf("the deferred flush must skip Japanese devices: %+v", send)
		}
	}
	if seen["example-en"] != "Good evening" || seen["token-zh"] != "晚上好" || seen["token-hant"] != "晚上好" {
		t.Fatalf("deferred bodies = %+v", seen)
	}

	items, err := store.PendingPushes(ctx, time.Now(), pendingGrace, maxFanout)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("delivered posts stay pending: %+v", items)
	}
}

func TestFlushPendingFallsBackToThePostAfterGrace(t *testing.T) {
	ctx := context.Background()
	store := notifyStore(t)
	if _, err := store.DB().Exec(`INSERT INTO posts (id, source, type, text, published_at, url) VALUES ('p1', 'x', 'daily', 'こんばんは', '2026-10-01T00:00:00.000Z', 'https://x.example.invalid/p1')`); err != nil {
		t.Fatal(err)
	}
	if err := store.RegisterPushDevice(ctx, sqlite.PushDevice{Token: "example-en", DeviceID: "device-en", Locale: "en-US", Enabled: true, NotifyPost: true}); err != nil {
		t.Fatal(err)
	}
	if err := store.SavePendingPushes(ctx, []sqlite.PendingPush{{
		PostID: "p1", Platform: "twitter", Kind: "post", Author: "鹿乃",
		Text: "こんばんは", URL: "https://x.example.invalid/p1",
		CreatedAt: sqlite.ISOTime(time.Now().Add(-11 * time.Minute)),
	}}); err != nil {
		t.Fatal(err)
	}
	sends := []captureSend{}
	account, _ := testAccount(t)
	client, err := New(Options{Account: account, Client: &http.Client{Transport: notifyTransport(t, &sends)}})
	if err != nil {
		t.Fatal(err)
	}
	dispatcher := NewDispatcher(client, store)
	if err = dispatcher.FlushPending(ctx); err != nil {
		t.Fatal(err)
	}
	if len(sends) != 1 || sends[0].Body != "こんばんは" || !sends[0].Tokens["example-en"] {
		t.Fatalf("an expired wait must send the original wording: %+v", sends)
	}
}
