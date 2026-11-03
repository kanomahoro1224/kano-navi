package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
	pushkit "github.com/yexca/kano-navi/backend/internal/push"
	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
	"github.com/yexca/kano-navi/backend/internal/testutil"
)

func testWebPushHandler(t *testing.T) (*Handler, *sqlite.Store) {
	t.Helper()
	store, err := sqlite.OpenAdminWrites(testutil.NewSnapshot(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	private, public, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		t.Fatal(err)
	}
	sender, err := pushkit.NewWebSender(pushkit.WebOptions{
		PublicKey: public, PrivateKey: private, Hosts: []string{"push.example"},
		Origin: "https://kano.example",
	})
	if err != nil || sender == nil {
		t.Fatalf("sender = %v (%v)", sender, err)
	}
	handler := NewHandlerWithAdmin(store, time.Now, nil)
	handler.webPush = sender
	handler.webPushLimits = newWebPushLimiter()
	return handler, store
}

func subscribeBody(t *testing.T, endpoint, extra string) string {
	t.Helper()
	key := make([]byte, 65)
	auth := make([]byte, 16)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	if _, err := rand.Read(auth); err != nil {
		t.Fatal(err)
	}
	return `{"endpoint":"` + endpoint + `","keys":{"p256dh":"` +
		base64.RawURLEncoding.EncodeToString(key) + `","auth":"` +
		base64.RawURLEncoding.EncodeToString(auth) + `"},"locale":"ja"` + extra + `}`
}

func webPushCall(t *testing.T, handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(recorder, request)
	return recorder
}

// The key route reports the feature off instead of failing, so the site can
// hide the button without treating the answer as an error.
func TestWebPushKeyDisabled(t *testing.T) {
	store, err := sqlite.OpenAdminWrites(testutil.NewSnapshot(t))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	handler := NewHandler(store)
	response := webPushCall(t, handler, http.MethodGet, "/api/push/web/key", "")
	if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store" ||
		!strings.Contains(response.Body.String(), `"enabled":false`) {
		t.Fatalf("status=%d headers=%v body=%s", response.Code, response.Header(), response.Body)
	}
	if response := webPushCall(t, handler, http.MethodPost, "/api/push/web/subscribe",
		subscribeBody(t, "https://push.example/one", "")); response.Code != http.StatusServiceUnavailable {
		t.Fatalf("subscribe status=%d", response.Code)
	}
}

// A registration is idempotent and an omitted switch keeps what the row had,
// the same contract as the App's device registration.
func TestWebPushSubscribeAndUnsubscribe(t *testing.T) {
	handler, store := testWebPushHandler(t)
	key := webPushCall(t, handler, http.MethodGet, "/api/push/web/key", "")
	if key.Code != http.StatusOK || !strings.Contains(key.Body.String(), `"enabled":true`) {
		t.Fatalf("key status=%d body=%s", key.Code, key.Body)
	}
	endpoint := "https://push.example/subscription/one"
	body := subscribeBody(t, endpoint, `,"video":false`)
	if response := webPushCall(t, handler, http.MethodPost, "/api/push/web/subscribe", body); response.Code != http.StatusNoContent {
		t.Fatalf("subscribe status=%d body=%s", response.Code, response.Body)
	}
	subs, err := store.WebPushSubscriptions(context.Background())
	if err != nil || len(subs) != 1 || subs[0].Locale != "ja" || subs[0].NotifyVideo || !subs[0].NotifyPost {
		t.Fatalf("subs=%+v err=%v", subs, err)
	}
	// The second post omits the switches; neither the stored locale nor the
	// switched-off category may flip back on.
	if response := webPushCall(t, handler, http.MethodPost, "/api/push/web/subscribe",
		`{"endpoint":"`+endpoint+`","keys":{"p256dh":"`+base64.RawURLEncoding.EncodeToString(make([]byte, 65))+
			`","auth":"`+base64.RawURLEncoding.EncodeToString(make([]byte, 16))+`"}}`); response.Code != http.StatusNoContent {
		t.Fatalf("resubscribe status=%d body=%s", response.Code, response.Body)
	}
	subs, err = store.WebPushSubscriptions(context.Background())
	if err != nil || len(subs) != 1 || subs[0].Locale != "ja" || subs[0].NotifyVideo {
		t.Fatalf("after resubscribe: %+v (%v)", subs, err)
	}
	if response := webPushCall(t, handler, http.MethodPost, "/api/push/web/unsubscribe",
		`{"endpoint":"`+endpoint+`"}`); response.Code != http.StatusNoContent {
		t.Fatalf("unsubscribe status=%d", response.Code)
	}
	if subs, err = store.WebPushSubscriptions(context.Background()); err != nil || len(subs) != 0 {
		t.Fatalf("after unsubscribe: %+v (%v)", subs, err)
	}
}

// The stored endpoint is a URL the server posts to, so it must be https on a
// known push service; the keys must have the sizes the encryption needs.
func TestWebPushRejectsUnsafeRegistrations(t *testing.T) {
	handler, _ := testWebPushHandler(t)
	for _, body := range []string{
		subscribeBody(t, "http://push.example/one", ""),
		subscribeBody(t, "https://evil.example/one", ""),
		`{"endpoint":"https://push.example/one","keys":{"p256dh":"short","auth":"short"}}`,
		`not json`,
		`{"endpoint":"https://push.example/one","keys":{}}`,
	} {
		if response := webPushCall(t, handler, http.MethodPost, "/api/push/web/subscribe", body); response.Code != http.StatusBadRequest {
			t.Fatalf("body=%s status=%d", body, response.Code)
		}
	}
	if response := webPushCall(t, handler, http.MethodGet, "/api/push/web/subscribe", ""); response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET status=%d", response.Code)
	}
}

// The limiter is what keeps a script from filling the table; the handler maps
// a refusal to 429.
func TestWebPushRegistrationIsRateLimited(t *testing.T) {
	handler, _ := testWebPushHandler(t)
	body := subscribeBody(t, "https://push.example/one", "")
	status := http.StatusNoContent
	for i := 0; i < webPushRateLimit && status == http.StatusNoContent; i++ {
		status = webPushCall(t, handler, http.MethodPost, "/api/push/web/subscribe", body).Code
	}
	if status != http.StatusNoContent {
		t.Fatalf("limit hit early: %d", status)
	}
	if response := webPushCall(t, handler, http.MethodPost, "/api/push/web/subscribe", body); response.Code != http.StatusTooManyRequests {
		t.Fatalf("over limit status=%d", response.Code)
	}
}
