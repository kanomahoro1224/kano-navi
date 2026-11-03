package adminapi

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	webpush "github.com/SherClockHolmes/webpush-go"

	pushkit "github.com/yexca/kano-navi/backend/internal/push"
	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
)

// The panel reports browsers beside devices, and the test action reaches
// exactly the subscriptions registered.
func TestPushPanelReportsBrowsersAndTestsThem(t *testing.T) {
	store, err := sqlite.OpenInitialized(filepath.Join(t.TempDir(), "database", "kano.sqlite"), true, true)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	browserKey, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	auth := make([]byte, 16)
	if _, err = rand.Read(auth); err != nil {
		t.Fatal(err)
	}
	subscription := sqlite.WebPushSubscription{
		Endpoint: "https://push.example/subscription/one",
		P256DH:   base64.RawURLEncoding.EncodeToString(browserKey.PublicKey().Bytes()),
		Auth:     base64.RawURLEncoding.EncodeToString(auth),
		Locale:   "ja", Enabled: true,
		NotifyLive: true, NotifyVideo: true, NotifyPost: true,
	}
	if err = store.RegisterWebPushSubscription(context.Background(), subscription); err != nil {
		t.Fatalf("register: %v", err)
	}

	var sent int
	private, public, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		t.Fatal(err)
	}
	sender, err := pushkit.NewWebSender(pushkit.WebOptions{
		PublicKey: public, PrivateKey: private, Hosts: []string{"push.example"},
		HTTPClient: &http.Client{Transport: pushRoundTrip(func(request *http.Request) (*http.Response, error) {
			sent++
			return &http.Response{
				StatusCode: http.StatusCreated,
				Body:       io.NopCloser(strings.NewReader("")),
				Request:    request,
			}, nil
		})},
	})
	if err != nil || sender == nil {
		t.Fatalf("sender = %v (%v)", sender, err)
	}
	handler := New(Options{Store: store, Auth: newAuth(t, "development", ""), WebPush: sender})

	call := func(method, path, body string) (int, map[string]any) {
		t.Helper()
		request := httptest.NewRequest(method, path, strings.NewReader(body))
		if body != "" {
			request.Header.Set("Content-Type", "application/json")
		}
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		payload := map[string]any{}
		if recorder.Body.Len() > 0 {
			if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
				t.Fatalf("%s %s returned non-JSON %q", method, path, recorder.Body.String())
			}
		}
		return recorder.Code, payload
	}

	code, panel := call(http.MethodGet, "/api/admin/push", "")
	if code != http.StatusOK {
		t.Fatalf("panel status = %d", code)
	}
	web, _ := panel["web"].(map[string]any)
	if web["configured"] != true || web["total"] != float64(1) || web["enabled"] != float64(1) {
		t.Fatalf("web = %v", panel["web"])
	}
	rows, _ := web["subscriptions"].([]any)
	if len(rows) != 1 {
		t.Fatalf("subscriptions = %v", web["subscriptions"])
	}
	row, _ := rows[0].(map[string]any)
	if label, _ := row["endpointLabel"].(string); strings.Contains(label, "subscription/one") {
		t.Fatalf("endpoint label not masked: %q", label)
	}
	if endpoint, _ := row["endpoint"].(string); endpoint != subscription.Endpoint {
		t.Fatalf("full endpoint missing: %q", endpoint)
	}

	code, result := call(http.MethodPost, "/api/admin/push/test", `{"transport":"web"}`)
	if code != http.StatusOK || result["ok"] != true || result["subscriptions"] != float64(1) || sent != 1 {
		t.Fatalf("test result = %v (sent %d, status %d)", result, sent, code)
	}

	code, _ = call(http.MethodDelete, "/api/admin/push/web?endpoint="+strings.ReplaceAll(subscription.Endpoint, "/", "%2F"), "")
	if code != http.StatusNoContent {
		t.Fatalf("delete status = %d", code)
	}
	if subs, err := store.WebPushSubscriptions(context.Background()); err != nil || len(subs) != 0 {
		t.Fatalf("after delete: %+v (%v)", subs, err)
	}
}
