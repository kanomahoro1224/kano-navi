package push

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
)

// receiptTestKey mints a throwaway Base64-encoded key the way AGC issues
// callback keys, so the suite never needs the real receipt credential, which
// must not live in the repository.
func receiptTestKey(t *testing.T) string {
	t.Helper()
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return base64.StdEncoding.EncodeToString(raw)
}

// receiptStore opens a fresh writable store so the suite never touches a real
// database.
func receiptStore(t *testing.T) *sqlite.Store {
	t.Helper()
	store, err := sqlite.OpenInitialized(filepath.Join(t.TempDir(), "snapshot.sqlite"), true, true)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

// signReceipt builds the callback header the way Huawei's receipt server
// does, decoding the Base64 key AGC issues.
func signReceipt(secret, user, timestamp, nonce string) string {
	key, err := base64.StdEncoding.DecodeString(secret)
	if err != nil {
		key = []byte(secret)
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(timestamp + nonce + user))
	return "timestamp=" + timestamp + "; nonce=" + nonce + "; value=" +
		base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// postReceipt submits one receipt batch.
func postReceipt(handler http.Handler, header, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, ReceiptPath, bytes.NewReader([]byte(body)))
	request.Header.Set(receiptHeaderName, header)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func TestNewReceiptHandlerStaysNilWithoutSecret(t *testing.T) {
	if NewReceiptHandler(nil, "", "kano") != nil {
		t.Fatal("handler should be nil without a secret")
	}
	if NewReceiptHandler(receiptStore(t), "  ", "kano") != nil {
		t.Fatal("handler should be nil without a secret")
	}
}

func TestReceiptRetiresUninstalledAndMismatchedTokens(t *testing.T) {
	store := receiptStore(t)
	for _, token := range []string{"example-live", "token-dead", "token-mismatch"} {
		if err := store.RegisterPushDevice(t.Context(), sqlite.PushDevice{Token: token, DeviceID: "device-" + token, Enabled: true, NotifyLive: true, NotifyVideo: true, NotifyPost: true}); err != nil {
			t.Fatalf("register %s: %v", token, err)
		}
	}
	secret := receiptTestKey(t)
	handler := NewReceiptHandler(store, secret, "kano")
	body := `{"statuses":[` +
		`{"token":"example-live","pushType":0,"appPackageName":"com.example","requestId":"r1","deliveryStatus":{"result":0,"timestamp":1700000000000}},` +
		`{"token":"token-dead","pushType":0,"appPackageName":"com.example","requestId":"r2","deliveryStatus":{"result":2,"timestamp":1700000000000}},` +
		`{"token":"token-mismatch","pushType":0,"appPackageName":"com.example","requestId":"r3","deliveryStatus":{"result":5,"timestamp":1700000000000}}]}`
	response := postReceipt(handler, signReceipt(secret, "kano", "1700000000000", "nonce-1"), body)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var result receiptResult
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatalf("response json: %v", err)
	}
	if result.Code != "0" || result.Message != "success" {
		t.Fatalf("response = %+v, want code 0 success", result)
	}
	devices, err := store.PushDevices(t.Context())
	if err != nil {
		t.Fatalf("list devices: %v", err)
	}
	if len(devices) != 1 || devices[0].Token != "example-live" {
		t.Fatalf("devices = %+v, want only example-live", devices)
	}
}

func TestReceiptRejectsForgedSignature(t *testing.T) {
	store := receiptStore(t)
	if err := store.RegisterPushDevice(t.Context(), sqlite.PushDevice{Token: "example-live", DeviceID: "device-1", Enabled: true}); err != nil {
		t.Fatalf("register: %v", err)
	}
	secret := receiptTestKey(t)
	handler := NewReceiptHandler(store, secret, "kano")
	body := `{"statuses":[{"token":"example-live","deliveryStatus":{"result":2,"timestamp":1700000000000}}]}`
	// A receipt signed for another user name must not retire anything.
	response := postReceipt(handler, signReceipt(secret, "other-user", "1700000000000", "nonce-1"), body)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", response.Code)
	}
	devices, err := store.PushDevices(t.Context())
	if err != nil {
		t.Fatalf("list devices: %v", err)
	}
	if len(devices) != 1 {
		t.Fatalf("devices = %+v, want the token kept", devices)
	}
	// A missing header is equally rejected.
	response = postReceipt(handler, "", body)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status without header = %d, want 401", response.Code)
	}
}

func TestReceiptAcceptsRawSecret(t *testing.T) {
	store := receiptStore(t)
	// A secret that does not decode is the raw key of an older configuration.
	secret := "not-a-real-callback-key"
	handler := NewReceiptHandler(store, secret, "kano")
	response := postReceipt(handler, signReceipt(secret, "kano", "1700000000000", "nonce-1"), `{}`)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
}

func TestReceiptAcknowledgesEmptyBody(t *testing.T) {
	store := receiptStore(t)
	secret := receiptTestKey(t)
	handler := NewReceiptHandler(store, secret, "kano")
	response := postReceipt(handler, signReceipt(secret, "kano", "1700000000000", "nonce-1"), "")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	var result receiptResult
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatalf("response json: %v", err)
	}
	if result.Code != "0" || result.Message != "success" {
		t.Fatalf("response = %+v, want code 0 success", result)
	}
}

func TestReceiptRequiresPost(t *testing.T) {
	handler := NewReceiptHandler(receiptStore(t), receiptTestKey(t), "kano")
	request := httptest.NewRequest(http.MethodGet, ReceiptPath, nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", recorder.Code)
	}
	if allow := recorder.Header().Get("Allow"); allow != http.MethodPost {
		t.Fatalf("allow = %q, want POST", allow)
	}
}
