package totpadmin

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yexca/kano-navi/backend/internal/adminauth"
	"github.com/yexca/kano-navi/backend/internal/httpx"
	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
)

const (
	password = "synthetic-admin-password"
	secrets  = "synthetic-secrets-key-for-tests-only"
)

// totpAt is the RFC 6238 code of a base32 secret for the step of now plus
// offset steps.
func totpAt(t *testing.T, secret string, now time.Time, offset int64) string {
	t.Helper()
	raw, err := adminauth.ParseTOTPSecret(secret)
	if err != nil {
		t.Fatal(err)
	}
	var counter [8]byte
	binary.BigEndian.PutUint64(counter[:], uint64(now.Unix()/30+offset))
	mac := hmac.New(sha1.New, raw)
	mac.Write(counter[:])
	sum := mac.Sum(nil)
	at := sum[len(sum)-1] & 0x0f
	return fmt.Sprintf("%06d", (binary.BigEndian.Uint32(sum[at:at+4])&0x7fffffff)%1000000)
}

type fixture struct {
	t      *testing.T
	store  *sqlite.Store
	auth   *adminauth.Auth
	admin  *Admin
	now    *time.Time
	routes http.Handler
}

func newFixture(t *testing.T, envSecret []byte) *fixture {
	t.Helper()
	store, err := sqlite.OpenInitialized(filepath.Join(t.TempDir(), "database", "kano.sqlite"), true, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	now := time.Date(2026, 11, 27, 9, 0, 10, 0, time.UTC)
	f := &fixture{t: t, store: store, now: &now}
	f.auth = f.newAuth(envSecret)
	f.admin = New(store, secrets, f.auth)
	f.admin.now = func() time.Time { return *f.now }
	f.routes = httpx.NewAdminMux(f.admin.AdminRoutes())
	return f
}

func (f *fixture) newAuth(envSecret []byte) *adminauth.Auth {
	cfg, err := adminauth.NewConfig("production", password, "")
	if err != nil {
		f.t.Fatal(err)
	}
	cfg.TOTPSecret, cfg.Now = envSecret, func() time.Time { return *f.now }
	auth, err := adminauth.New(cfg)
	if err != nil {
		f.t.Fatal(err)
	}
	return auth
}

// login signs in and returns the session cookie.
func (f *fixture) login(totp string) *http.Cookie {
	f.t.Helper()
	body, _ := json.Marshal(map[string]string{"password": password, "code": totp})
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/admin/session", strings.NewReader(string(body)))
	r.RemoteAddr = "192.0.2.10:4000"
	f.auth.Login(rec, r)
	if rec.Code != 200 {
		f.t.Fatalf("login=%d %s", rec.Code, rec.Body)
	}
	return rec.Result().Cookies()[0]
}

// call sends one request with session and answers the status and body.
func (f *fixture) call(session *http.Cookie, method, path string, body any) (int, map[string]any) {
	f.t.Helper()
	text := ""
	if body != nil {
		raw, _ := json.Marshal(body)
		text = string(raw)
	}
	r := httptest.NewRequest(method, httpx.AdminPrefix+path, strings.NewReader(text))
	r.RemoteAddr = "192.0.2.10:4000"
	if session != nil {
		r.AddCookie(session)
	}
	rec := httptest.NewRecorder()
	f.routes.ServeHTTP(rec, r)
	out := map[string]any{}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

// signedIn reports whether a session cookie still opens the console.
func (f *fixture) signedIn(session *http.Cookie) bool {
	r := httptest.NewRequest(http.MethodGet, "/api/admin/session", nil)
	r.AddCookie(session)
	return f.auth.SessionPayload(r)["authenticated"] == true
}

func (f *fixture) sealed() string {
	var value string
	_ = f.store.DB().QueryRow(`SELECT value FROM app_settings WHERE key=?`, setting).Scan(&value)
	return value
}

func TestTheConsoleBindsReplacesAndTurnsOffTheCode(t *testing.T) {
	f := newFixture(t, nil)
	if code, status := f.call(nil, "GET", Path, nil); code != 200 || status["state"] != StateOff || status["asked"] != false || status["enrollment"] != nil {
		t.Fatalf("off=%d %v", code, status)
	}
	session, other := f.login(""), f.login("")

	code1, started := f.call(session, "POST", Path+"/enrollment", nil)
	secret, _ := started["secret"].(string)
	picture, _ := started["qr"].(map[string]any)
	if code1 != 200 || len(secret) != 32 || picture == nil || started["expiresAt"] == nil {
		t.Fatalf("start=%d %v", code1, started)
	}
	if _, status := f.call(session, "GET", Path, nil); status["enrollment"] == nil || strings.Contains(fmt.Sprint(status), secret) {
		t.Fatalf("status during enrollment=%v", status)
	}
	// A wrong code is refused before the password is looked at; the right
	// code with a wrong password stores nothing.
	if code, out := f.call(session, "PUT", Path, map[string]string{"password": password, "code": "000000"}); code != 400 || out["error"] != "totp_code_invalid" {
		t.Fatalf("wrong code=%d %v", code, out)
	}
	if code, out := f.call(session, "PUT", Path, map[string]string{"password": "wrong-password", "code": totpAt(t, secret, *f.now, 0)}); code != 403 || out["error"] != "invalid_credentials" || f.sealed() != "" {
		t.Fatalf("wrong password=%d %v", code, out)
	}
	confirm := totpAt(t, secret, *f.now, 0)
	codeOK, bound := f.call(session, "PUT", Path, map[string]string{"password": password, "code": confirm})
	if codeOK != 200 || bound["state"] != StateConsole || bound["asked"] != true || bound["boundAt"] == nil || bound["enrollment"] != nil {
		t.Fatalf("confirm=%d %v", codeOK, bound)
	}
	if sealed := f.sealed(); sealed == "" || strings.Contains(sealed, secret) {
		t.Fatalf("the stored secret is not sealed: %q", sealed)
	}
	if !f.signedIn(session) || f.signedIn(other) {
		t.Fatal("binding must keep the confirming session and end the others")
	}

	// Login now needs the code, and the code that confirmed is spent.
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/admin/session", strings.NewReader(`{"password":"`+password+`","code":"`+confirm+`"}`))
	f.auth.Login(rec, r)
	if rec.Code != 401 {
		t.Fatalf("the confirming code signed in: %d", rec.Code)
	}
	*f.now = f.now.Add(30 * time.Second)
	other = f.login(totpAt(t, secret, *f.now, 0))

	// A restarted process binds the stored secret again.
	restarted := f.newAuth(nil)
	if err := New(f.store, secrets, restarted).Load(context.Background()); err != nil || !restarted.RequiresCode() {
		t.Fatalf("reload err=%v asked=%v", err, restarted.RequiresCode())
	}
	if _, _, err := Load(context.Background(), f.store.DB(), "another-secrets-key-for-tests", f.newAuth(nil)); err != ErrUnreadable {
		t.Fatalf("a secret sealed with another key loaded: %v", err)
	}

	// Replacing needs the current code as well.
	*f.now = f.now.Add(30 * time.Second)
	_, next := f.call(session, "POST", Path+"/enrollment", nil)
	replacement, _ := next["secret"].(string)
	if code, out := f.call(session, "PUT", Path, map[string]string{"password": password, "code": totpAt(t, replacement, *f.now, 0)}); code != 403 || out["error"] != "invalid_credentials" {
		t.Fatalf("replace without the current code=%d %v", code, out)
	}
	if code, out := f.call(session, "PUT", Path, map[string]string{"password": password, "code": totpAt(t, replacement, *f.now, 0), "currentCode": totpAt(t, secret, *f.now, 0)}); code != 200 || out["state"] != StateConsole {
		t.Fatalf("replace=%d %v", code, out)
	}
	if f.signedIn(other) {
		t.Fatal("replacing must end the other sessions")
	}

	// Turning off needs the code of the secret now in use.
	*f.now = f.now.Add(30 * time.Second)
	if code, out := f.call(session, "DELETE", Path, map[string]string{"password": password, "code": totpAt(t, secret, *f.now, 0)}); code != 403 || out["error"] != "invalid_credentials" {
		t.Fatalf("turn off with the old secret's code=%d %v", code, out)
	}
	if code, out := f.call(session, "DELETE", Path, map[string]string{"password": password, "code": totpAt(t, replacement, *f.now, 0)}); code != 200 || out["state"] != StateOff || out["asked"] != false {
		t.Fatalf("turn off=%d %v", code, out)
	}
	if f.sealed() != "" || f.auth.RequiresCode() {
		t.Fatal("turning off left the code in use")
	}
}

func TestWrongCodesEndAnEnrollment(t *testing.T) {
	f := newFixture(t, nil)
	session := f.login("")
	_, started := f.call(session, "POST", Path+"/enrollment", nil)
	secret, _ := started["secret"].(string)
	wrong := "000000"
	if wrong == totpAt(t, secret, *f.now, 0) {
		wrong = "111111"
	}
	for range confirmAttempts {
		if status, _ := f.call(session, "PUT", Path, map[string]string{"password": password, "code": wrong}); status != 400 {
			t.Fatalf("wrong code=%d", status)
		}
	}
	if status, out := f.call(session, "PUT", Path, map[string]string{"password": password, "code": totpAt(t, secret, *f.now, 0)}); status != 409 || out["error"] != "totp_enrollment_missing" {
		t.Fatalf("after the last attempt=%d %v", status, out)
	}
	// An enrollment also ends when it expires, and on cancel.
	f.call(session, "POST", Path+"/enrollment", nil)
	*f.now = f.now.Add(EnrollmentLifetime)
	if _, status := f.call(session, "GET", Path, nil); status["enrollment"] != nil {
		t.Fatalf("an expired enrollment is still open: %v", status)
	}
	f.call(session, "POST", Path+"/enrollment", nil)
	if _, status := f.call(session, "DELETE", Path+"/enrollment", nil); status["enrollment"] != nil {
		t.Fatalf("cancel left the enrollment open: %v", status)
	}
}

func TestTheEnvironmentsCodeLocksTheConsole(t *testing.T) {
	f := newFixture(t, []byte("12345678901234567890"))
	session := f.login(totpAt(t, "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ", *f.now, 0))
	for _, request := range []struct{ method, path string }{{"POST", Path + "/enrollment"}, {"PUT", Path}, {"DELETE", Path}} {
		if status, out := f.call(session, request.method, request.path, map[string]string{}); status != 409 || out["error"] != "environment_managed" {
			t.Fatalf("%s %s=%d %v", request.method, request.path, status, out)
		}
	}
	if _, status := f.call(session, "GET", Path, nil); status["state"] != StateEnvironment || status["asked"] != true || status["storedUnused"] != false {
		t.Fatalf("status=%v", status)
	}
	// A secret stored before the environment set one stays unused and does
	// not need to open.
	if _, err := f.store.DB().Exec(`INSERT INTO app_settings(key,value,updated_at) VALUES(?,?,?)`, setting, "sealed-with-another-key", "2026-11-27T00:00:00.000Z"); err != nil {
		t.Fatal(err)
	}
	if err := f.admin.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, status := f.call(session, "GET", Path, nil); status["storedUnused"] != true {
		t.Fatalf("status=%v", status)
	}
	if removed, err := Reset(context.Background(), f.store.DB()); err != nil || !removed {
		t.Fatalf("reset removed=%v err=%v", removed, err)
	}
}
