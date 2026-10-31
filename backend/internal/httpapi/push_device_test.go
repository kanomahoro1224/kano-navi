package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
	"github.com/yexca/kano-navi/backend/internal/testutil"
)

func postPushDevice(t *testing.T, handler http.Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/push/device", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(recorder, req)
	return recorder
}

// The App re-registers on every launch, so the route validates the token and
// keeps the switches it did not send on their default rather than off.
func TestPushDeviceRegistersOneDeviceAndDefaultsItsSwitches(t *testing.T) {
	store, err := sqlite.OpenAdminWrites(testutil.NewSnapshot(t))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	handler := NewHandler(store)

	if method := request(t, handler, http.MethodGet, "/api/push/device"); method.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET status=%d", method.Code)
	}
	for _, body := range []string{`{"deviceId":"device-1"}`, `{"token":"token-1"}`, `not json`} {
		if response := postPushDevice(t, handler, body); response.Code != http.StatusBadRequest {
			t.Fatalf("%s status=%d body=%s", body, response.Code, response.Body)
		}
	}
	response := postPushDevice(t, handler, `{"token":"token-1","deviceId":"device-1","locale":"zh-CN","video":false}`)
	if response.Code != http.StatusOK {
		t.Fatalf("register status=%d body=%s", response.Code, response.Body)
	}
	devices, err := store.PushDevices(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 1 {
		t.Fatalf("devices = %+v", devices)
	}
	device := devices[0]
	if device.Token != "token-1" || device.Platform != "harmonyos" || device.Locale != "zh-CN" ||
		!device.Enabled || !device.NotifyLive || !device.NotifyPost || device.NotifyVideo {
		t.Fatalf("device = %+v", device)
	}
}
