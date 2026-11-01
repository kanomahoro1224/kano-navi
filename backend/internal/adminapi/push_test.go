package adminapi

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	pushkit "github.com/yexca/kano-navi/backend/internal/push"
	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
)

type pushRoundTrip func(*http.Request) (*http.Response, error)

func (f pushRoundTrip) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func testPushKey(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
}

// The panel must never show a usable token, and the test button must send
// exactly one notification to the registered device.
func TestPushPanelMasksDevicesAndSendsTestNotification(t *testing.T) {
	store, err := sqlite.OpenInitialized(filepath.Join(t.TempDir(), "database", "kano.sqlite"), true, true)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	const token = "synthetic-token-0123456789abcdef"
	device := sqlite.PushDevice{
		Token: token, DeviceID: "device-1", Platform: "harmonyos", Locale: "zh-Hans",
		Enabled: true, NotifyLive: true, NotifyVideo: true, NotifyPost: true,
	}
	if err = store.RegisterPushDevice(context.Background(), device); err != nil {
		t.Fatalf("register: %v", err)
	}

	var mutex sync.Mutex
	var bodies []string
	pushClient, err := pushkit.New(pushkit.Options{
		Account: pushkit.ServiceAccount{
			ProjectID: "101653523865498303", KeyID: "key-id", SubAccount: "sub-account",
			PrivateKey: testPushKey(t),
		},
		Client: &http.Client{Transport: pushRoundTrip(func(request *http.Request) (*http.Response, error) {
			raw, _ := io.ReadAll(request.Body)
			mutex.Lock()
			bodies = append(bodies, string(raw))
			mutex.Unlock()
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"code":"80000000"}`)),
				Request:    request,
			}, nil
		})},
	})
	if err != nil {
		t.Fatalf("push client: %v", err)
	}
	handler := New(Options{Store: store, Auth: newAuth(t, "development", ""), Push: pushClient})

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
	if panel["configured"] != true || panel["projectId"] != "101653523865498303" {
		t.Fatalf("panel = %v", panel)
	}
	devices, _ := panel["devices"].([]any)
	if len(devices) != 1 {
		t.Fatalf("devices = %v", panel["devices"])
	}
	shown, _ := devices[0].(map[string]any)
	if shown["token"] == token || !strings.Contains(shown["token"].(string), "…") {
		t.Fatalf("token was not masked: %v", shown["token"])
	}
	if strings.Contains(toJSON(t, panel), token) {
		t.Fatal("the panel leaked a full push token")
	}

	code, sent := call(http.MethodPost, "/api/admin/push/test", `{}`)
	if code != http.StatusOK || sent["ok"] != true || sent["devices"] != float64(1) {
		t.Fatalf("test send = %d %v", code, sent)
	}
	mutex.Lock()
	defer mutex.Unlock()
	if len(bodies) != 1 {
		t.Fatalf("push requests = %d, want 1", len(bodies))
	}
	if !strings.Contains(bodies[0], "推送测试") || !strings.Contains(bodies[0], `"notifyId":1`) {
		t.Fatalf("test body = %s", bodies[0])
	}

	if code, _ = call(http.MethodDelete, "/api/admin/push/devices/device-1", ""); code != http.StatusNoContent {
		t.Fatalf("delete status = %d", code)
	}
	remaining, err := store.AdminPushDevices(context.Background())
	if err != nil || len(remaining) != 0 {
		t.Fatalf("devices after delete = %d, %v", len(remaining), err)
	}
}

func toJSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(raw)
}
