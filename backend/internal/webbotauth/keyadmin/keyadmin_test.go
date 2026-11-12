package keyadmin

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yexca/kano-navi/backend/internal/httpx"
	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
	"github.com/yexca/kano-navi/backend/internal/webbotauth"
)

const (
	secrets = "synthetic-llm-secrets-key"
	origin  = "https://board.example.test"
)

func openStore(t *testing.T) *sqlite.Store {
	t.Helper()
	store, err := sqlite.OpenStandalone(filepath.Join(t.TempDir(), "database", "kano.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

type fixture struct {
	admin  *Admin
	holder *webbotauth.Holder
	now    *time.Time
}

func load(t *testing.T, store *sqlite.Store, key string, env *webbotauth.Key, publicOrigin string) fixture {
	t.Helper()
	holder := webbotauth.NewHolder()
	admin := New(store, holder, key, env, publicOrigin)
	now := time.Date(2026, 11, 11, 3, 0, 0, 0, time.UTC)
	admin.now = func() time.Time { return now }
	holder.Now = admin.now
	if err := admin.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	return fixture{admin, holder, &now}
}

func call(t *testing.T, admin *Admin, method, path, body string) (int, map[string]any) {
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
	return w.Code, payload
}

func settings(t *testing.T, store *sqlite.Store) map[string]string {
	t.Helper()
	rows, err := store.DB().Query(`SELECT key, value FROM app_settings WHERE key LIKE 'web_bot_auth_%'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			t.Fatal(err)
		}
		out[key] = value
	}
	return out
}

func directory(t *testing.T, holder *webbotauth.Holder) (int, []webbotauth.JWK, string) {
	t.Helper()
	w := httptest.NewRecorder()
	holder.ServeDirectory(w, httptest.NewRequest(http.MethodGet, webbotauth.DirectoryPath, nil))
	var body struct{ Keys []webbotauth.JWK }
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	return w.Code, body.Keys, w.Header().Get("Signature-Input")
}

func TestGenerateRotateAndRemoveSwapTheSigner(t *testing.T) {
	store := openStore(t)
	f := load(t, store, secrets, nil, "")
	code, status := call(t, f.admin, http.MethodGet, Path, "")
	if code != 200 || status["state"] != StateNoKey || status["keyId"] != nil || status["source"] != nil || f.holder.Current() != nil {
		t.Fatalf("empty: %d %v", code, status)
	}
	if code, _, _ := directory(t, f.holder); code != http.StatusNotFound {
		t.Fatalf("empty directory=%d", code)
	}

	// A key without an https origin waits for one.
	code, status = call(t, f.admin, http.MethodPost, Path+"/key", "")
	first, _ := status["keyId"].(string)
	if code != 200 || status["state"] != StateNoOrigin || first == "" || status["source"] != SourceStored || status["createdAt"] != "2026-11-11T03:00:00.000Z" || f.holder.Current() != nil {
		t.Fatalf("generated: %d %v", code, status)
	}
	code, status = call(t, f.admin, http.MethodPut, Path, `{"origin":" https://Board.Example.Test/ "}`)
	if code != 200 || status["state"] != StateActive || status["origin"] != origin || status["originSetting"] != origin ||
		status["directoryUrl"] != origin+webbotauth.DirectoryPath {
		t.Fatalf("origin: %d %v", code, status)
	}
	// The running holder signs with the new key without a restart.
	signer := f.holder.Current()
	if signer == nil || signer.Key().Thumbprint() != first || signer.Agent() != origin {
		t.Fatalf("signer=%v", signer)
	}
	seed := signer.Key().Seed()
	for key, value := range settings(t, store) {
		if strings.Contains(value, seed) {
			t.Fatalf("%s holds the plaintext seed", key)
		}
		if (key == settingKey || key == settingPrevious) && !strings.HasPrefix(value, "v1:") {
			t.Fatalf("%s is not sealed: %q", key, value)
		}
	}
	raw, _ := json.Marshal(status)
	if strings.Contains(string(raw), seed) {
		t.Fatal("status carries the seed")
	}

	// Rotating signs with the new key at once and keeps the old one in the
	// directory for the grace period.
	*f.now = f.now.Add(time.Minute)
	code, status = call(t, f.admin, http.MethodPost, Path+"/key", "")
	second, _ := status["keyId"].(string)
	previous, _ := status["previous"].(map[string]any)
	if code != 200 || second == "" || second == first || previous["keyId"] != first || previous["until"] != "2026-11-12T03:01:00.000Z" || f.holder.Current().Key().Thumbprint() != second {
		t.Fatalf("rotated: %d %v", code, status)
	}
	code, keys, input := directory(t, f.holder)
	if code != 200 || len(keys) != 2 || webbotauth.Thumbprint(decodeX(t, keys[0].X)) != second || webbotauth.Thumbprint(decodeX(t, keys[1].X)) != first ||
		!strings.Contains(input, `sig1=`) || !strings.Contains(input, `sig2=`) || !strings.Contains(input, first) {
		t.Fatalf("grace directory: %d %v %s", code, keys, input)
	}
	// A restart reads the same state.
	again := load(t, store, secrets, nil, "")
	*again.now = *f.now
	if _, status := call(t, again.admin, http.MethodGet, Path, ""); status["keyId"] != second || status["previous"] == nil || again.holder.Current() == nil {
		t.Fatalf("reloaded: %v", status)
	}
	// After the grace only the current key is listed.
	*f.now = f.now.Add(Grace)
	if _, keys, _ := directory(t, f.holder); len(keys) != 1 {
		t.Fatalf("after grace keys=%v", keys)
	}
	if _, status := call(t, f.admin, http.MethodGet, Path, ""); status["previous"] != nil {
		t.Fatalf("after grace: %v", status)
	}

	// Removing stops signing and the directory; the origin setting stays.
	code, status = call(t, f.admin, http.MethodDelete, Path+"/key", "")
	if code != 200 || status["state"] != StateNoKey || f.holder.Current() != nil || status["originSetting"] != origin || status["directoryUrl"] != nil {
		t.Fatalf("removed: %d %v", code, status)
	}
	if code, _, _ := directory(t, f.holder); code != http.StatusNotFound {
		t.Fatalf("removed directory=%d", code)
	}
	if stored := settings(t, store); len(stored) != 1 || stored[settingOrigin] != origin {
		t.Fatalf("stored after remove=%v", stored)
	}
}

func decodeX(t *testing.T, x string) []byte {
	t.Helper()
	raw, err := base64.RawURLEncoding.DecodeString(x)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestOriginDefaultsToAnHTTPSPublicOriginAndIsValidated(t *testing.T) {
	store := openStore(t)
	f := load(t, store, secrets, nil, origin)
	if _, status := call(t, f.admin, http.MethodPost, Path+"/key", ""); status["state"] != StateActive || status["origin"] != origin || status["originSetting"] != nil {
		t.Fatalf("public origin: %v", status)
	}
	for _, body := range []string{`{"origin":"http://board.example.test"}`, `{"origin":"https://board.example.test/path"}`, `{"origin":"board.example.test"}`, `{"origin":1}`, `{"other":"x"}`, `{"origin":"https://board.example.test","other":1}`} {
		if code, _ := call(t, f.admin, http.MethodPut, Path, body); code != http.StatusBadRequest {
			t.Fatalf("%s: %d", body, code)
		}
	}
	// The operator's origin wins; clearing it falls back to PUBLIC_ORIGIN.
	if _, status := call(t, f.admin, http.MethodPut, Path, `{"origin":"https://agent.example.test"}`); status["origin"] != "https://agent.example.test" || f.holder.Current().Agent() != "https://agent.example.test" {
		t.Fatalf("operator origin: %v", status)
	}
	if _, status := call(t, f.admin, http.MethodPut, Path, `{"origin":null}`); status["origin"] != origin || status["originSetting"] != nil {
		t.Fatalf("cleared: %v", status)
	}
	// An http PUBLIC_ORIGIN is no signing origin.
	plain := load(t, store, secrets, nil, "http://board.example.test")
	if _, status := call(t, plain.admin, http.MethodGet, Path, ""); status["state"] != StateNoOrigin || plain.holder.Current() != nil {
		t.Fatalf("http public origin: %v", status)
	}
}

func TestEnvironmentKeyOverridesTheStoredOne(t *testing.T) {
	store := openStore(t)
	stored := load(t, store, secrets, nil, origin)
	_, status := call(t, stored.admin, http.MethodPost, Path+"/key", "")
	storedID := status["keyId"]
	env, err := webbotauth.GenerateKey(bytes.NewReader(bytes.Repeat([]byte{5}, 32)))
	if err != nil {
		t.Fatal(err)
	}
	f := load(t, store, secrets, env, origin)
	_, status = call(t, f.admin, http.MethodGet, Path, "")
	if status["state"] != StateActive || status["source"] != SourceEnvironment || status["keyId"] != env.Thumbprint() || status["createdAt"] != nil ||
		status["environmentManaged"] != true || f.holder.Current().Key().Thumbprint() != env.Thumbprint() {
		t.Fatalf("env: %v", status)
	}
	for _, method := range []string{http.MethodPost, http.MethodDelete} {
		if code, payload := call(t, f.admin, method, Path+"/key", ""); code != http.StatusConflict || payload["error"] != "environment_managed" {
			t.Fatalf("%s: %d %v", method, code, payload)
		}
	}
	// The origin stays the operator's to set, and the stored key is kept.
	if code, _ := call(t, f.admin, http.MethodPut, Path, `{"origin":"https://agent.example.test"}`); code != 200 || f.holder.Current().Agent() != "https://agent.example.test" {
		t.Fatalf("env origin: %d", code)
	}
	if _, status := call(t, load(t, store, secrets, nil, origin).admin, http.MethodGet, Path, ""); status["keyId"] != storedID {
		t.Fatalf("stored key changed: %v", status)
	}
	// Without any origin the environment key waits too.
	if _, status := call(t, load(t, openStore(t), secrets, env, "").admin, http.MethodGet, Path, ""); status["state"] != StateNoOrigin {
		t.Fatalf("env without origin: %v", status)
	}
}

// A key sealed under another LLM_SECRETS_KEY turns signing off with a
// visible state instead of failing the start; generating replaces it.
func TestUnreadableKeyDisablesSigning(t *testing.T) {
	store := openStore(t)
	f := load(t, store, secrets, nil, origin)
	call(t, f.admin, http.MethodPost, Path+"/key", "")
	call(t, f.admin, http.MethodPost, Path+"/key", "")
	changed := load(t, store, "another-synthetic-key", nil, origin)
	_, status := call(t, changed.admin, http.MethodGet, Path, "")
	if status["state"] != StateKeyUnreadable || status["keyId"] != nil || status["previous"] != nil || changed.holder.Current() != nil {
		t.Fatalf("unreadable: %v", status)
	}
	if code, _, _ := directory(t, changed.holder); code != http.StatusNotFound {
		t.Fatalf("unreadable directory=%d", code)
	}
	_, status = call(t, changed.admin, http.MethodPost, Path+"/key", "")
	if status["state"] != StateActive || status["previous"] != nil || changed.holder.Current() == nil {
		t.Fatalf("replaced: %v", status)
	}
	// Without LLM_SECRETS_KEY nothing can be sealed.
	if code, payload := call(t, load(t, openStore(t), "", nil, origin).admin, http.MethodPost, Path+"/key", ""); code != http.StatusServiceUnavailable || payload["error"] != "secrets_key_unavailable" {
		t.Fatalf("no secrets key: %d %v", code, payload)
	}
}

func TestReadOnlyStoreRefusesWrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "database", "kano.sqlite")
	writable, err := sqlite.OpenStandalone(path)
	if err != nil {
		t.Fatal(err)
	}
	_ = writable.Close()
	store, err := sqlite.OpenReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	f := load(t, store, secrets, nil, origin)
	if code, _ := call(t, f.admin, http.MethodPost, Path+"/key", ""); code == http.StatusOK {
		t.Fatal("read-only store generated a key")
	}
	if code, status := call(t, f.admin, http.MethodGet, Path, ""); code != 200 || status["state"] != StateNoKey {
		t.Fatalf("read-only status: %d %v", code, status)
	}
}
