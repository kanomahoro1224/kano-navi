package outboundproxy

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yexca/kano-navi/backend/internal/httpx"
	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
)

const testKey = "synthetic-llm-secrets-key"

func openStore(t *testing.T) *sqlite.Store {
	t.Helper()
	store, err := sqlite.OpenStandalone(filepath.Join(t.TempDir(), "database", "kano.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func call(t *testing.T, admin *Admin, method, path, body string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	r := httptest.NewRequest(method, "/api/admin"+path, reader)
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	httpx.NewAdminMux(admin.AdminRoutes()).ServeHTTP(w, r)
	payload := map[string]any{}
	_ = json.Unmarshal(w.Body.Bytes(), &payload)
	return w, payload
}

func storedValue(t *testing.T, store *sqlite.Store) string {
	t.Helper()
	var value string
	if err := store.DB().QueryRow(`SELECT value FROM app_settings WHERE key = ?`, settingKey).Scan(&value); err != nil {
		t.Fatal(err)
	}
	return value
}

func TestFreshSettingsHaveNoProxiesAndEveryAreaOff(t *testing.T) {
	store := openStore(t)
	admin := NewAdmin(store, NewPool(store.DB(), testKey))
	w, payload := call(t, admin, http.MethodGet, "/proxy", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body)
	}
	if proxies := payload["proxies"].([]any); len(proxies) != 0 {
		t.Fatalf("proxies=%v", proxies)
	}
	routes := payload["routes"].(map[string]any)
	for _, area := range Areas {
		route := routes[string(area)].(map[string]any)
		if route["enabled"] != false || len(route["proxyIds"].([]any)) != 0 {
			t.Fatalf("%s route=%v", area, route)
		}
	}
}

func TestSavedPasswordIsSealedWriteOnlyAndKept(t *testing.T) {
	store := openStore(t)
	pool := NewPool(store.DB(), testKey)
	admin := NewAdmin(store, pool)
	w, payload := call(t, admin, http.MethodPut, "/proxy", `{
		"proxies":[{"name":"Tokyo","scheme":"socks5","host":"proxy.example.test","port":1080,"username":"user","password":"synthetic-pass","enabled":true}],
		"routes":{"x":{"enabled":true,"proxyIds":[]}},
		"directFallback":true
	}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body)
	}
	if strings.Contains(w.Body.String(), "synthetic-pass") || strings.Contains(storedValue(t, store), "synthetic-pass") {
		t.Fatal("password leaked into the response or storage")
	}
	proxy := payload["proxies"].([]any)[0].(map[string]any)
	id := proxy["id"].(string)
	if proxy["hasPassword"] != true || id == "" || payload["directFallback"] != true {
		t.Fatalf("payload=%v", payload)
	}

	// Saving without a password keeps it; the pool sees the change at once.
	w, _ = call(t, admin, http.MethodPut, "/proxy", `{"proxies":[{"id":"`+id+`","name":"Tokyo 2","scheme":"socks5","host":"proxy.example.test","port":1080,"username":"user","enabled":true}]}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body)
	}
	config, err := pool.Config(context.Background())
	if err != nil || config.Proxies[0].Password != "synthetic-pass" || config.Proxies[0].Name != "Tokyo 2" || !config.Routes[AreaX].Enabled {
		t.Fatalf("config=%+v err=%v", config, err)
	}
	reloaded, err := load(context.Background(), store.DB(), testKey)
	if err != nil || reloaded.Proxies[0].Password != "synthetic-pass" {
		t.Fatalf("reloaded=%+v err=%v", reloaded, err)
	}

	// The saved password never follows the proxy to another address.
	for _, moved := range []string{
		`"scheme":"socks5","host":"other.example.test","port":1080`,
		`"scheme":"socks5","host":"proxy.example.test","port":1081`,
		`"scheme":"http","host":"proxy.example.test","port":1080`,
	} {
		w, _ = call(t, admin, http.MethodPut, "/proxy", `{"proxies":[{"id":"`+id+`","name":"Tokyo 2",`+moved+`,"username":"user","enabled":true}]}`)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("%s: status=%d body=%s", moved, w.Code, w.Body)
		}
	}
	if config, err = pool.Config(context.Background()); err != nil || config.Proxies[0].Host != "proxy.example.test" {
		t.Fatalf("a refused move changed the config: %+v err=%v", config, err)
	}
	w, _ = call(t, admin, http.MethodPut, "/proxy", `{"proxies":[{"id":"`+id+`","name":"Tokyo 2","scheme":"socks5","host":"other.example.test","port":1080,"username":"user","password":"other-pass","enabled":true}]}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body)
	}
	if config, err = pool.Config(context.Background()); err != nil || config.Proxies[0].Password != "other-pass" {
		t.Fatalf("config=%+v err=%v", config, err)
	}
}

func TestChangedSecretsKeyMarksThePasswordUnreadable(t *testing.T) {
	store := openStore(t)
	admin := NewAdmin(store, NewPool(store.DB(), testKey))
	call(t, admin, http.MethodPut, "/proxy", `{"proxies":[{"id":"tokyo","scheme":"http","host":"proxy.example.test","port":8080,"username":"user","password":"synthetic-pass","enabled":true}],"routes":{"media":{"enabled":true}}}`)
	sealed := storedValue(t, store)

	other := NewAdmin(store, NewPool(store.DB(), "another-synthetic-key"))
	_, payload := call(t, other, http.MethodGet, "/proxy", "")
	proxy := payload["proxies"].([]any)[0].(map[string]any)
	if proxy["passwordUnreadable"] != true || proxy["hasPassword"] != true {
		t.Fatalf("proxy=%v", proxy)
	}
	config, _ := other.pool.Config(context.Background())
	if routed, usable := config.Resolve(AreaMedia); !routed || len(usable) != 0 {
		t.Fatalf("routed=%v usable=%v", routed, usable)
	}
	// Saving other fields keeps the sealed password for the original key.
	call(t, other, http.MethodPut, "/proxy", `{"directFallback":true}`)
	if !strings.Contains(storedValue(t, store), extractSealed(t, sealed)) {
		t.Fatal("unreadable password was not kept sealed")
	}
}

func extractSealed(t *testing.T, raw string) string {
	t.Helper()
	var stored storedConfig
	if err := json.Unmarshal([]byte(raw), &stored); err != nil || len(stored.Proxies) == 0 {
		t.Fatalf("stored=%s", raw)
	}
	return stored.Proxies[0].SealedPassword
}

func TestInvalidSettingsAreRejected(t *testing.T) {
	store := openStore(t)
	admin := NewAdmin(store, NewPool(store.DB(), testKey))
	for name, body := range map[string]string{
		"route without proxy": `{"routes":{"x":{"enabled":true}}}`,
		"unknown area":        `{"proxies":[{"scheme":"http","host":"proxy.example.test","port":8080}],"routes":{"github":{"enabled":true}}}`,
		"bad scheme":          `{"proxies":[{"scheme":"ftp","host":"proxy.example.test","port":21}]}`,
		"bad port":            `{"proxies":[{"scheme":"http","host":"proxy.example.test","port":0}]}`,
		"host with path":      `{"proxies":[{"scheme":"http","host":"proxy.example.test/path","port":8080}]}`,
		"credentials in host": `{"proxies":[{"scheme":"http","host":"user:pass@proxy.example.test","port":8080}]}`,
		"duplicate id":        `{"proxies":[{"id":"a","scheme":"http","host":"one.example.test","port":1},{"id":"a","scheme":"http","host":"two.example.test","port":2}]}`,
	} {
		w, payload := call(t, admin, http.MethodPut, "/proxy", body)
		if w.Code != http.StatusBadRequest || payload["error"] != "invalid_input" {
			t.Fatalf("%s: status=%d body=%s", name, w.Code, w.Body)
		}
	}
}

func TestProxyTestUsesOnlyThatProxyAndRecordsHealth(t *testing.T) {
	store := openStore(t)
	pool := NewPool(store.DB(), testKey)
	var seen []string
	pool.base = func(proxy Proxy) http.RoundTripper {
		return roundTripFunc(func(r *http.Request) (*http.Response, error) {
			seen = append(seen, proxy.Host+" "+r.URL.String())
			response := answer("")
			response.StatusCode = http.StatusNoContent
			return response, nil
		})
	}
	admin := NewAdmin(store, pool)
	call(t, admin, http.MethodPut, "/proxy", `{"proxies":[{"id":"tokyo","scheme":"http","host":"proxy.example.test","port":8080,"enabled":false}]}`)
	w, payload := call(t, admin, http.MethodPost, "/proxy/tokyo/test", `{"area":"youtube"}`)
	if w.Code != http.StatusOK || payload["ok"] != true || payload["status"] != float64(204) {
		t.Fatalf("status=%d body=%s", w.Code, w.Body)
	}
	if len(seen) != 1 || seen[0] != "proxy.example.test https://www.youtube.com/generate_204" {
		t.Fatalf("seen=%v", seen)
	}
	health := payload["proxy"].(map[string]any)["health"].(map[string]any)
	if health["lastSuccessAt"] == nil {
		t.Fatalf("health=%v", health)
	}
	if w, _ = call(t, admin, http.MethodPost, "/proxy/tokyo/test", `{"area":"llm"}`); w.Code != http.StatusBadRequest {
		t.Fatalf("llm test status=%d", w.Code)
	}
	if w, _ = call(t, admin, http.MethodPost, "/proxy/missing/test", `{}`); w.Code != http.StatusNotFound {
		t.Fatalf("missing status=%d", w.Code)
	}
}

func TestCorruptStoredSettingsFailClosed(t *testing.T) {
	store := openStore(t)
	if _, err := store.DB().Exec(`INSERT INTO app_settings(key,value,updated_at) VALUES(?,?,?)`, settingKey, "{not json", "2026-01-01T00:00:00.000Z"); err != nil {
		t.Fatal(err)
	}
	pool := NewPool(store.DB(), testKey)
	request, _ := http.NewRequest(http.MethodGet, "https://api.example.test/", nil)
	if _, err := pool.Transport(AreaX).RoundTrip(request); err != ErrUnavailable {
		t.Fatalf("err=%v", err)
	}
	// An operator save replaces the unreadable value.
	w, _ := call(t, NewAdmin(store, pool), http.MethodPut, "/proxy", `{"proxies":[],"routes":{}}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body)
	}
}
