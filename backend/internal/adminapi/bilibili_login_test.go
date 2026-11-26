package adminapi

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	bilisource "github.com/yexca/kano-navi/backend/internal/source/bilibili"
	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
)

const (
	syntheticImportLogin = "synthetic0login%2C1767225600%2Cabcde%2A11"
	syntheticImportJCT   = "0123456789abcdef0123456789abcdef"
	syntheticImportToken = "synthetic-refresh-0001"
	syntheticQRLogin     = "synthetic0scan%2C1769817600%2Cfghij%2A22"
)

// bilibiliPassport answers the device identity and the login's passport
// requests and keeps each one.
type bilibiliPassport struct {
	mu    sync.Mutex
	calls []*http.Request
	forms []url.Values
}

func (p *bilibiliPassport) client() *bilisource.Client {
	return bilisource.NewClient(&http.Client{Transport: bilibiliTransport(func(r *http.Request) (*http.Response, error) {
		form := url.Values{}
		if r.Body != nil {
			body, _ := io.ReadAll(r.Body)
			form, _ = url.ParseQuery(string(body))
		}
		p.mu.Lock()
		p.calls, p.forms = append(p.calls, r), append(p.forms, form)
		p.mu.Unlock()
		header, body := http.Header{}, `{"code":-400}`
		switch r.URL.Path {
		case "/x/frontend/finger/spi":
			body = `{"code":0,"data":{"b_3":"0A1B2C3D-0A1B-0A1B-0A1B-0A1B2C3D4E5F12345infoc","b_4":"synthetic-buvid4=="}}`
		case "/x/passport-login/web/qrcode/generate":
			body = `{"code":0,"data":{"url":"https://passport.bilibili.com/h5-app/passport/login/scan?navhide=1&qrcode_key=8587cf8106a0b863c46d6bab913537f6&from=","qrcode_key":"8587cf8106a0b863c46d6bab913537f6"}}`
		case "/x/passport-login/web/qrcode/poll":
			body = `{"code":0,"data":{"url":"","refresh_token":"` + syntheticImportToken + `","timestamp":1767225600000,"code":0,"message":""}}`
			header.Add("Set-Cookie", "SESSDATA="+syntheticQRLogin+"; Path=/")
			header.Add("Set-Cookie", "bili_jct="+syntheticImportJCT+"; Path=/")
		case "/login/exit/v2":
			body = `{"code":0,"status":true,"ts":1767225600,"data":{"redirectUrl":"https://www.bilibili.com/"}}`
		}
		return &http.Response{StatusCode: 200, Header: header, Body: io.NopCloser(strings.NewReader(body))}, nil
	})})
}

func (p *bilibiliPassport) paths() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := []string{}
	for _, r := range p.calls {
		out = append(out, r.URL.Path)
	}
	return out
}

func TestTheBilibiliLoginIsImportedSignedInByQRAndForgotten(t *testing.T) {
	store, err := sqlite.OpenInitialized(filepath.Join(t.TempDir(), "database", "kano.sqlite"), true, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	interval := qrPollInterval
	qrPollInterval = 5 * time.Millisecond
	t.Cleanup(func() { qrPollInterval = interval })
	passport := &bilibiliPassport{}
	adapter := bilisource.NewAdapter(passport.client(), bilisource.Options{Account: "316381099", MinInterval: 3 * time.Hour, PageSize: 20, SESSDATA: "synthetic0env%2C1767225600%2Cabcde%2A11"})
	adapter.Logins = &bilisource.Logins{Records: store, Key: "synthetic-secrets-key-for-tests-only"}
	c := &console{store: store, handler: New(Options{Store: store, Auth: newAuth(t, "development", ""), Bilibili: adapter})}
	source := func() map[string]any {
		t.Helper()
		code, payload := c.call(t, "GET", "/api/admin/sources", "")
		status, _ := payload["bilibili"].(map[string]any)
		if code != 200 || status == nil {
			t.Fatalf("sources=%d %v", code, payload)
		}
		raw, _ := json.Marshal(status)
		for _, secret := range []string{syntheticImportLogin, syntheticImportJCT, syntheticImportToken, syntheticQRLogin, "8587cf8106a0b863c46d6bab913537f6"} {
			if strings.Contains(string(raw), secret) {
				t.Fatalf("the source status carries a secret: %s", raw)
			}
		}
		return status
	}
	if status := source(); status["loginSource"] != "environment" || status["login"] != "unchecked" || status["loginStorable"] != true {
		t.Fatalf("environment=%v", status)
	}

	for body, want := range map[string]int{
		`{"cookie":"SESSDATA=` + syntheticImportLogin + `","refreshToken":"` + syntheticImportToken + `"}`:                                      400,
		`{"cookie":"SESSDATA=` + syntheticImportLogin + `; bili_jct=` + syntheticImportJCT + `","refreshToken":"x"}`:                            400,
		`{"cookie":"SESSDATA=` + syntheticImportLogin + `; bili_jct=` + syntheticImportJCT + `"}`:                                               400,
		`{"cookie":"SESSDATA=` + syntheticImportLogin + `; bili_jct=` + syntheticImportJCT + `","refreshToken":"` + syntheticImportToken + `"}`: 200,
	} {
		if code, payload := c.call(t, "PUT", "/api/admin/sources/bilibili/login", body); code != want {
			t.Fatalf("%s=%d %v", body, code, payload)
		}
	}
	if paths := passport.paths(); len(paths) != 0 {
		t.Fatalf("an import made requests: %v", paths)
	}
	if status := source(); status["loginSource"] != "stored" || status["loginVia"] != "import" || status["login"] != "unchecked" || status["loginStoredAt"] == nil {
		t.Fatalf("imported=%v", status)
	}
	if sealed, _ := store.BilibiliLoginRecord(t.Context()); sealed == "" || strings.Contains(sealed, syntheticImportLogin) {
		t.Fatalf("the stored login is not sealed: %q", sealed)
	}
	if code, payload := c.call(t, "DELETE", "/api/admin/sources/bilibili/login", ""); code != 200 || payload["loginSource"] != "environment" {
		t.Fatalf("forget=%d %v", code, payload)
	}

	code, payload := c.call(t, "POST", "/api/admin/sources/bilibili/login/qr", "")
	picture, _ := payload["qr"].(map[string]any)
	if code != 200 || payload["state"] != "waiting" || picture == nil || picture["size"].(float64) < 21 || !strings.HasPrefix(picture["path"].(string), "M") || payload["expiresAt"] == nil {
		t.Fatalf("qr=%d %v", code, payload)
	}
	deadline := time.Now().Add(5 * time.Second)
	for payload["state"] != "signed_in" && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
		_, payload = c.call(t, "GET", "/api/admin/sources/bilibili/login/qr", "")
	}
	if payload["state"] != "signed_in" || payload["qr"] != nil {
		t.Fatalf("qr status=%v", payload)
	}
	if status := source(); status["loginSource"] != "stored" || status["loginVia"] != "qr" {
		t.Fatalf("signed in=%v", status)
	}

	before := len(passport.paths())
	if code, payload = c.call(t, "DELETE", "/api/admin/sources/bilibili/login?signOut=1", ""); code != 200 || payload["loginSource"] != "environment" {
		t.Fatalf("sign out=%d %v", code, payload)
	}
	passport.mu.Lock()
	exit, form := passport.calls[len(passport.calls)-1], passport.forms[len(passport.forms)-1]
	passport.mu.Unlock()
	if len(passport.paths()) <= before || exit.URL.Path != "/login/exit/v2" || form.Get("biliCSRF") != syntheticImportJCT || !strings.Contains(exit.Header.Get("Cookie"), "SESSDATA="+syntheticQRLogin) {
		t.Fatalf("exit=%s form=%v", exit.URL.Path, form)
	}
}

func TestConcurrentQRStartsOpenOneSignIn(t *testing.T) {
	store, err := sqlite.OpenInitialized(filepath.Join(t.TempDir(), "database", "kano.sqlite"), true, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	interval := qrPollInterval
	qrPollInterval = time.Hour
	t.Cleanup(func() { qrPollInterval = interval })
	passport := &bilibiliPassport{}
	adapter := bilisource.NewAdapter(passport.client(), bilisource.Options{Account: "316381099", MinInterval: 3 * time.Hour, PageSize: 20})
	adapter.Logins = &bilisource.Logins{Records: store, Key: "synthetic-secrets-key-for-tests-only"}
	c := &console{store: store, handler: New(Options{Store: store, Auth: newAuth(t, "development", ""), Bilibili: adapter})}
	var wg sync.WaitGroup
	codes := make([]int, 4)
	for i := range codes {
		wg.Go(func() { codes[i], _ = c.call(t, "POST", "/api/admin/sources/bilibili/login/qr", "") })
	}
	wg.Wait()
	generated := 0
	for _, path := range passport.paths() {
		if path == "/x/passport-login/web/qrcode/generate" {
			generated++
		}
	}
	if generated != 1 || codes[0] != 200 || codes[1] != 200 || codes[2] != 200 || codes[3] != 200 {
		t.Fatalf("generated=%d codes=%v", generated, codes)
	}
	c.call(t, "DELETE", "/api/admin/sources/bilibili/login/qr", "")
}

func TestAnUnreadableBilibiliLoginIsReportedAndForgotten(t *testing.T) {
	store, err := sqlite.OpenInitialized(filepath.Join(t.TempDir(), "database", "kano.sqlite"), true, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	passport := &bilibiliPassport{}
	adapter := bilisource.NewAdapter(passport.client(), bilisource.Options{Account: "316381099", MinInterval: 3 * time.Hour, PageSize: 20})
	adapter.Logins = &bilisource.Logins{Records: store, Key: "synthetic-secrets-key-for-tests-only"}
	c := &console{store: store, handler: New(Options{Store: store, Auth: newAuth(t, "development", ""), Bilibili: adapter})}
	body := `{"cookie":"SESSDATA=` + syntheticImportLogin + `; bili_jct=` + syntheticImportJCT + `","refreshToken":"` + syntheticImportToken + `"}`
	if code, payload := c.call(t, "PUT", "/api/admin/sources/bilibili/login", body); code != 200 || payload["loginUnreadable"] != false {
		t.Fatalf("import=%d %v", code, payload)
	}
	adapter.Logins.Key = "another-synthetic-secrets-key-for-tests"
	code, payload := c.call(t, "GET", "/api/admin/sources", "")
	status, _ := payload["bilibili"].(map[string]any)
	if code != 200 || status["loginUnreadable"] != true || status["loginSource"] != nil {
		t.Fatalf("unreadable=%d %v", code, status)
	}
	if code, payload = c.call(t, "DELETE", "/api/admin/sources/bilibili/login?signOut=1", ""); code != 409 || payload["error"] != "bilibili_login_unreadable" {
		t.Fatalf("sign out=%d %v", code, payload)
	}
	if code, payload = c.call(t, "DELETE", "/api/admin/sources/bilibili/login", ""); code != 200 || payload["loginUnreadable"] != false {
		t.Fatalf("forget=%d %v", code, payload)
	}
	if sealed, _ := store.BilibiliLoginRecord(t.Context()); sealed != "" || len(passport.paths()) != 0 {
		t.Fatalf("sealed=%q requests=%v", sealed, passport.paths())
	}
}
