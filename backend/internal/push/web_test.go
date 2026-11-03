package push

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	webpush "github.com/SherClockHolmes/webpush-go"

	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
)

// subscribedKeys builds the browser half of a subscription: a real P-256
// public key, which the payload encryption needs, and a 16 byte auth secret.
func subscribedKeys(t *testing.T) (string, string) {
	t.Helper()
	key, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	auth := make([]byte, 16)
	if _, err = rand.Read(auth); err != nil {
		t.Fatal(err)
	}
	return base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes()),
		base64.RawURLEncoding.EncodeToString(auth)
}

func testSender(t *testing.T, handler http.HandlerFunc) (*WebSender, *httptest.Server) {
	t.Helper()
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	private, public, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		t.Fatal(err)
	}
	sender, err := NewWebSender(WebOptions{
		PublicKey:  public,
		PrivateKey: private,
		Subject:    "mailto:test@example.com",
		Hosts:      []string{"127.0.0.1"},
		HTTPClient: server.Client(),
		Origin:     "https://kano.example",
	})
	if err != nil || sender == nil {
		t.Fatalf("sender = %v (%v)", sender, err)
	}
	return sender, server
}

// A browser is posted an encrypted payload with the VAPID authorization, the
// TTL and the topic that replaces an undelivered older notification.
func TestWebSenderEncryptsAndPosts(t *testing.T) {
	var seen http.Header
	var body []byte
	sender, server := testSender(t, func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Clone()
		body, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusCreated)
	})
	p256dh, auth := subscribedKeys(t)
	sub := sqlite.WebPushSubscription{Endpoint: server.URL + "/push", P256DH: p256dh, Auth: auth, Enabled: true}
	payload := WebPayload{Title: "鹿乃 が YouTube で配信を開始", Body: "歌枠", URL: "https://youtube.com/watch?v=1", Tag: "live"}
	if err := sender.Send(context.Background(), sub, payload); err != nil {
		t.Fatal(err)
	}
	if len(body) == 0 || seen.Get("Content-Encoding") != "aes128gcm" {
		t.Fatalf("body=%d encoding=%q", len(body), seen.Get("Content-Encoding"))
	}
	if seen.Get("TTL") != "86400" || !strings.HasPrefix(seen.Get("Authorization"), "vapid ") {
		t.Fatalf("ttl=%q auth=%q", seen.Get("TTL"), seen.Get("Authorization"))
	}
	if seen.Get("Topic") != "live" || seen.Get("Urgency") != "high" {
		t.Fatalf("topic=%q urgency=%q", seen.Get("Topic"), seen.Get("Urgency"))
	}
}

// The two answers that mean "this browser is gone" become one sentinel, so the
// dispatcher deletes the row instead of counting a failure.
func TestWebSenderReportsGone(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusGone} {
		sender, server := testSender(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
		})
		p256dh, auth := subscribedKeys(t)
		sub := sqlite.WebPushSubscription{Endpoint: server.URL + "/push", P256DH: p256dh, Auth: auth, Enabled: true}
		err := sender.Send(context.Background(), sub, WebPayload{Title: "t"})
		if !errors.Is(err, ErrWebPushGone) {
			t.Fatalf("status %d: err = %v", status, err)
		}
	}
	sender, server := testSender(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	p256dh, auth := subscribedKeys(t)
	err := sender.Send(context.Background(), sqlite.WebPushSubscription{
		Endpoint: server.URL + "/push", P256DH: p256dh, Auth: auth, Enabled: true,
	}, WebPayload{Title: "t"})
	if err == nil || errors.Is(err, ErrWebPushGone) {
		t.Fatalf("500 err = %v", err)
	}
}

// Only https on a known push service may be stored: the server posts to that
// URL, so anything else is an SSRF primitive.
func TestWebSenderEndpointAllowlist(t *testing.T) {
	private, public, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		t.Fatal(err)
	}
	sender, err := NewWebSender(WebOptions{PublicKey: public, PrivateKey: private})
	if err != nil || sender == nil {
		t.Fatalf("sender = %v (%v)", sender, err)
	}
	cases := map[string]bool{
		"https://fcm.googleapis.com/fcm/send/abc":                true,
		"https://updates.push.services.mozilla.com/wpush/v2/abc": true,
		"https://web.push.apple.com/QGxhbw":                      true,
		"https://wns2-par02p.notify.windows.com/w/?token=x":      true,
		"http://fcm.googleapis.com/fcm/send/abc":                 false,
		"https://fcm.googleapis.com.evil.example/fcm/send/abc":   false,
		"https://push.services.mozilla.com/wpush/v2/abc":         false,
		"https://user:pass@fcm.googleapis.com/fcm/send/abc":      false,
		"https://127.0.0.1/push":                                 false,
		"not a url":                                              false,
	}
	for endpoint, want := range cases {
		if got := sender.AllowsEndpoint(endpoint); got != want {
			t.Errorf("AllowsEndpoint(%q) = %v, want %v", endpoint, got, want)
		}
	}
}

// Without a key pair the feature is off rather than failing later, and a
// malformed pair is rejected at startup.
func TestWebSenderConfiguration(t *testing.T) {
	if sender, err := NewWebSender(WebOptions{}); err != nil || sender != nil {
		t.Fatalf("empty options = %v (%v)", sender, err)
	}
	if sender, err := NewWebSender(WebOptions{PublicKey: "short", PrivateKey: "short"}); err == nil || sender != nil {
		t.Fatalf("bad keys = %v (%v)", sender, err)
	}
	private, public, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		t.Fatal(err)
	}
	sender, err := NewWebSender(WebOptions{PublicKey: public, PrivateKey: private, Origin: "https://kano.example"})
	if err != nil || sender == nil || !sender.Enabled() || sender.PublicKey() != public {
		t.Fatalf("sender = %v (%v)", sender, err)
	}
	if sender.icon != "https://kano.example/assets/kano-avatar.jpg" {
		t.Fatalf("icon = %q", sender.icon)
	}
	// An update URL relative to the site is made absolute for the worker.
	if payload := TestWebPayload("ja", true); payload.Title == "" || payload.Icon != DefaultWebPushIcon {
		t.Fatalf("test payload = %+v", payload)
	}
	// Outside test mode a test is quiet, so clicking Test does not interrupt.
	if payload := TestWebPayload("ja", false); payload.Urgency != WebUrgencyNormal {
		t.Fatalf("quiet test payload = %+v", payload)
	}
}
