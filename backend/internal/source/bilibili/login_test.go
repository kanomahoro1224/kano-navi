package bilibili

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

const (
	syntheticJCT     = "0123456789abcdef0123456789abcdef"
	syntheticToken   = "synthetic-refresh-0001"
	syntheticFresh   = "synthetic0fresh%2C1769817600%2Cfghij%2A22"
	syntheticFreshJC = "abcdefabcdefabcdefabcdefabcdef01"
	syntheticNext    = "00112233445566778899aabbccddeeff"
	syntheticCSRF    = "b0cc8411ded2f9db2cff2edb3123acac"
	syntheticSecrets = "synthetic-secrets-key-for-tests-only"
)

// memoryRecords is a sealed login row with compare and swap. failSwap fails
// that swap (counting from 1) with a storage error.
type memoryRecords struct {
	value           string
	swaps, failSwap int
}

func (m *memoryRecords) BilibiliLoginRecord(context.Context) (string, error) { return m.value, nil }

func (m *memoryRecords) SwapBilibiliLoginRecord(_ context.Context, from, to string) (bool, error) {
	m.swaps++
	if m.swaps == m.failSwap {
		return false, errors.New("synthetic storage failure")
	}
	if m.value != from {
		return false, nil
	}
	m.value = to
	return true, nil
}

func storedLogin(now time.Time) StoredLogin {
	login, err := ImportLogin("SESSDATA="+syntheticLogin+"; bili_jct="+syntheticJCT+"; DedeUserID=1000001; buvid3=0A1B2C3D-0A1B-0A1B-0A1B-0A1B2C3D4E5F67890infoc", syntheticToken, now)
	if err != nil {
		panic(err)
	}
	return login
}

// passportCall is one login request as Bilibili received it.
type passportCall struct {
	method, host, path, cookie string
	form                       url.Values
}

// passportFake answers the device identity and the login's passport
// requests, and keeps each request.
type passportFake struct {
	calls   []passportCall
	replies map[string]string
	status  map[string]int
	set     map[string][]string
}

func newPassportFake() *passportFake {
	return &passportFake{status: map[string]int{}, set: map[string][]string{}, replies: map[string]string{
		"/x/frontend/finger/spi":                session,
		ticketPath:                              ticket,
		"/x/passport-login/web/cookie/info":     `{"code":0,"data":{"refresh":true,"timestamp":1767225600000}}`,
		"/x/passport-login/web/cookie/refresh":  `{"code":0,"message":"0","data":{"status":0,"message":"","refresh_token":"` + syntheticNext + `"}}`,
		"/x/passport-login/web/confirm/refresh": `{"code":0,"message":"0","ttl":1}`,
		"/x/passport-login/web/qrcode/generate": `{"code":0,"data":{"url":"https://passport.bilibili.com/h5-app/passport/login/scan?navhide=1&qrcode_key=8587cf8106a0b863c46d6bab913537f6&from=","qrcode_key":"8587cf8106a0b863c46d6bab913537f6"}}`,
		"/x/passport-login/web/qrcode/poll":     `{"code":0,"data":{"url":"","refresh_token":"","timestamp":0,"code":86101,"message":"未扫码"}}`,
		"/login/exit/v2":                        `{"code":0,"status":true,"ts":1767225600,"data":{"redirectUrl":"https://www.bilibili.com/"}}`,
		"/correspond/1/":                        `<html><body><div id="1-name">` + syntheticCSRF + `</div></body></html>`,
	}}
}

func (f *passportFake) client() *Client {
	return NewClient(&http.Client{Transport: fakeTransport(func(req *http.Request) (*http.Response, error) {
		call := passportCall{method: req.Method, host: req.URL.Host, path: req.URL.Path, cookie: req.Header.Get("Cookie")}
		if req.Body != nil {
			body, _ := io.ReadAll(req.Body)
			call.form, _ = url.ParseQuery(string(body))
		}
		f.calls = append(f.calls, call)
		key := req.URL.Path
		if strings.HasPrefix(key, correspondPrefix) {
			key = correspondPrefix
		}
		status := 200
		if s, ok := f.status[key]; ok {
			status = s
		}
		resp := response(status, f.replies[key])
		for _, cookie := range f.set[key] {
			resp.Header.Add("Set-Cookie", cookie)
		}
		return resp, nil
	})})
}

func (f *passportFake) paths() []string {
	out := []string{}
	for _, call := range f.calls {
		path := call.path
		if strings.HasPrefix(path, correspondPrefix) {
			path = correspondPrefix
		}
		out = append(out, path)
	}
	return out
}

func (f *passportFake) call(path string) *passportCall {
	for i := range f.calls {
		if f.calls[i].path == path {
			return &f.calls[i]
		}
	}
	return nil
}

func refreshingFake() *passportFake {
	fake := newPassportFake()
	fake.set["/x/passport-login/web/cookie/refresh"] = []string{
		"SESSDATA=" + syntheticFresh + "; Path=/; Domain=bilibili.com; HttpOnly; Secure",
		"bili_jct=" + syntheticFreshJC + "; Path=/; Domain=bilibili.com",
		"DedeUserID=1000001; Path=/; Domain=bilibili.com",
		"sid=synthetic; Path=/; Domain=bilibili.com",
	}
	return fake
}

func TestCorrespondPathOpensWithTheKeyPairOnly(t *testing.T) {
	if refreshKey.N.BitLen() != 1024 {
		t.Fatalf("refresh key bits=%d", refreshKey.N.BitLen())
	}
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	path, err := correspondPath(&key.PublicKey, 1767225600000, rand.Reader)
	if err != nil || !correspondPathPattern.MatchString(path) || len(path) != 256 {
		t.Fatalf("path=%q err=%v", path, err)
	}
	sealed, _ := hex.DecodeString(path)
	plain, err := rsa.DecryptOAEP(sha256.New(), nil, key, sealed, nil)
	if err != nil || string(plain) != "refresh_1767225600000" {
		t.Fatalf("plain=%q err=%v", plain, err)
	}
}

func TestImportKeepsOnlyTheLoginAndAWellFormedToken(t *testing.T) {
	now := time.Date(2026, 11, 26, 0, 0, 0, 0, time.UTC)
	login, err := ImportLogin("buvid3=0A1B2C3D-0A1B-0A1B-0A1B-0A1B2C3D4E5F67890infoc; SESSDATA="+syntheticLogin+"; bili_jct="+syntheticJCT+"; theme=dark; DedeUserID=1000001", ` "`+syntheticToken+`" `, now)
	if err != nil || login.SESSDATA != syntheticLogin || login.RefreshToken != syntheticToken || login.Via != ViaImport || !login.StoredAt.Equal(now) {
		t.Fatalf("login=%#v err=%v", login, err)
	}
	if _, ok := login.Cookies["theme"]; ok || login.Cookies["bili_jct"] != syntheticJCT || login.Cookies["buvid3"] == "" {
		t.Fatalf("cookies=%v", login.Cookies)
	}
	if _, err = ImportLogin("SESSDATA="+syntheticLogin, syntheticToken, now); ErrorCode(err) != "bilibili_login_cookie_invalid" {
		t.Fatalf("without bili_jct err=%v", err)
	}
	if _, err = ImportLogin("SESSDATA="+syntheticLogin+"; bili_jct="+syntheticJCT, "not a token; x=1", now); ErrorCode(err) != "bilibili_login_token_invalid" {
		t.Fatalf("bad token err=%v", err)
	}
}

func TestStoredLoginIsSealedAndSwappedOnlyFromWhatWasRead(t *testing.T) {
	now := time.Date(2026, 11, 26, 0, 0, 0, 0, time.UTC)
	records := &memoryRecords{}
	logins := &Logins{Records: records, Key: syntheticSecrets}
	if _, _, err := logins.Load(context.Background()); !errors.Is(err, ErrNoLogin) {
		t.Fatalf("empty err=%v", err)
	}
	if err := logins.Put(context.Background(), storedLogin(now)); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(records.value, syntheticLogin) || strings.Contains(records.value, syntheticToken) {
		t.Fatal("the record holds the login in plaintext")
	}
	login, version, err := logins.Load(context.Background())
	if err != nil || login.SESSDATA != syntheticLogin || version != records.value {
		t.Fatalf("login=%#v err=%v", login, err)
	}
	if _, err = logins.Replace(context.Background(), "stale", login); !errors.Is(err, ErrLoginChanged) {
		t.Fatalf("stale replace err=%v", err)
	}
	if unreadable, _ := (&Logins{Records: records, Key: "another-synthetic-secrets-key"}).Unreadable(context.Background()); !unreadable {
		t.Fatal("a record sealed with another key reads")
	}
	if err = logins.Forget(context.Background(), version); err != nil || records.value != "" {
		t.Fatalf("forget err=%v value=%q", err, records.value)
	}
}

func TestTheLoginGoesOnlyToItsDestinations(t *testing.T) {
	s := Session{Buvid3: "0A1B2C3D-0A1B-0A1B-0A1B-0A1B2C3D4E5F12345infoc", Buvid4: "synthetic-buvid4", Login: syntheticLogin}
	for _, to := range [][2]string{{apiHost, "/x/web-interface/nav"}, {passportHost, "/x/passport-login/web/cookie/info"}, {wwwHost, correspondPrefix + "00ff"}} {
		if !strings.Contains(s.cookie(true, to[0], to[1]), "SESSDATA=") {
			t.Fatalf("%s%s has no login", to[0], to[1])
		}
	}
	for _, to := range [][2]string{{wwwHost, "/opus/900000000000000010"}, {liveHost, "/room/v1/Room/get_status_info_by_uids"}, {appHost, "/x/v2/space/archive/cursor"}} {
		if strings.Contains(s.cookie(true, to[0], to[1]), "SESSDATA=") {
			t.Fatalf("%s%s carries the login", to[0], to[1])
		}
	}
}

func TestAStoredLoginWinsAndAnotherLoginsCheckDoesNotCount(t *testing.T) {
	now := time.Date(2026, 11, 26, 0, 0, 0, 0, time.UTC)
	records := &memoryRecords{}
	a := NewAdapter(NewClient(nil), Options{Account: DefaultAccount, SESSDATA: "synthetic0env%2C1767225600%2Cabcde%2A11"})
	a.Logins = &Logins{Records: records, Key: syntheticSecrets}
	checked := State{Login: LoginState{CheckedAt: now, SignedIn: true}}
	if info := a.Login(context.Background(), checked); info.Status != "signed_in" || info.Source != LoginEnvironment {
		t.Fatalf("environment info=%#v", info)
	}
	if err := a.Logins.Put(context.Background(), storedLogin(now)); err != nil {
		t.Fatal(err)
	}
	view := a.resolved(context.Background())
	if view.Options.SESSDATA != syntheticLogin || a.Options.SESSDATA == syntheticLogin {
		t.Fatalf("read login=%q environment=%q", view.Options.SESSDATA, a.Options.SESSDATA)
	}
	if info := a.Login(context.Background(), checked); info.Status != "unchecked" || info.Source != LoginStored || info.Via != ViaImport {
		t.Fatalf("stored info=%#v", info)
	}
	checked.Login = view.checked(now, NavLogin{SignedIn: true, Name: "synthetic"})
	if info := a.Login(context.Background(), checked); info.Status != "signed_in" || info.Name != "synthetic" {
		t.Fatalf("checked info=%#v", info)
	}
}

func TestMaintenanceStoresTheRefreshBeforeItConfirms(t *testing.T) {
	now := time.Date(2026, 11, 26, 0, 0, 0, 0, time.UTC)
	fake := refreshingFake()
	records := &memoryRecords{}
	a := NewAdapter(fake.client(), Options{Account: DefaultAccount})
	a.Now = func() time.Time { return now }
	a.Logins = &Logins{Records: records, Key: syntheticSecrets}
	if err := a.Logins.Put(context.Background(), storedLogin(now.Add(-48*time.Hour))); err != nil {
		t.Fatal(err)
	}
	result, err := a.MaintainLogin(context.Background(), State{})
	if err != nil || result.Outcome != RefreshRefreshed || result.State.Refresh.LastError != "" || result.State.Refresh.CheckedAt != now {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	want := []string{"/x/frontend/finger/spi", ticketPath, "/x/passport-login/web/cookie/info", correspondPrefix, "/x/passport-login/web/cookie/refresh", "/x/passport-login/web/confirm/refresh"}
	if got := fake.paths(); strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("requests=%v", got)
	}
	refresh := fake.call("/x/passport-login/web/cookie/refresh")
	if refresh.form.Get("csrf") != syntheticJCT || refresh.form.Get("refresh_csrf") != syntheticCSRF || refresh.form.Get("refresh_token") != syntheticToken || refresh.form.Get("source") != "main_web" || !strings.Contains(refresh.cookie, "SESSDATA="+syntheticLogin) {
		t.Fatalf("refresh=%#v", refresh)
	}
	confirm := fake.call("/x/passport-login/web/confirm/refresh")
	if confirm.form.Get("csrf") != syntheticFreshJC || confirm.form.Get("refresh_token") != syntheticToken || !strings.Contains(confirm.cookie, "SESSDATA="+syntheticFresh) {
		t.Fatalf("confirm=%#v", confirm)
	}
	stored, _, err := a.Logins.Load(context.Background())
	if err != nil || stored.SESSDATA != syntheticFresh || stored.Cookies["bili_jct"] != syntheticFreshJC || stored.RefreshToken != syntheticNext || stored.PendingConfirm != "" || !stored.RefreshedAt.Equal(now) || stored.Cookies["buvid3"] == "" {
		t.Fatalf("stored=%#v err=%v", stored, err)
	}
	if again, _ := a.MaintainLogin(context.Background(), result.State); again.Outcome != "" || again.Requested != 0 {
		t.Fatalf("a second check the same day=%#v", again)
	}
}

func TestAFailedWriteSendsNoConfirmAndKeepsTheOldLogin(t *testing.T) {
	now := time.Date(2026, 11, 26, 0, 0, 0, 0, time.UTC)
	fake := refreshingFake()
	records := &memoryRecords{}
	a := NewAdapter(fake.client(), Options{Account: DefaultAccount})
	a.Now = func() time.Time { return now }
	a.Logins = &Logins{Records: records, Key: syntheticSecrets}
	if err := a.Logins.Put(context.Background(), storedLogin(now)); err != nil {
		t.Fatal(err)
	}
	records.failSwap = records.swaps + 1
	result, err := a.MaintainLogin(context.Background(), State{})
	if ErrorCode(err) != "bilibili_login_save_failed" || result.State.Refresh.LastError != "bilibili_login_save_failed" {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	if fake.call("/x/passport-login/web/confirm/refresh") != nil {
		t.Fatal("a refresh that was not stored was confirmed")
	}
	if stored, _, _ := a.Logins.Load(context.Background()); stored.SESSDATA != syntheticLogin || stored.RefreshToken != syntheticToken {
		t.Fatalf("stored=%#v", stored)
	}
}

func TestAFailedConfirmIsSentAgainFirst(t *testing.T) {
	now := time.Date(2026, 11, 26, 0, 0, 0, 0, time.UTC)
	fake := refreshingFake()
	fake.replies["/x/passport-login/web/confirm/refresh"] = `{"code":-111,"message":"csrf"}`
	a := NewAdapter(fake.client(), Options{Account: DefaultAccount})
	a.Now = func() time.Time { return now }
	a.Logins = &Logins{Records: &memoryRecords{}, Key: syntheticSecrets}
	if err := a.Logins.Put(context.Background(), storedLogin(now)); err != nil {
		t.Fatal(err)
	}
	result, err := a.MaintainLogin(context.Background(), State{})
	if ErrorCode(err) != "bilibili_login_confirm_failed" || result.Outcome != RefreshFailed {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	stored, _, _ := a.Logins.Load(context.Background())
	if stored.SESSDATA != syntheticFresh || stored.PendingConfirm != syntheticToken {
		t.Fatalf("stored=%#v", stored)
	}
	fake.calls = nil
	fake.replies["/x/passport-login/web/confirm/refresh"] = `{"code":0}`
	fake.replies["/x/passport-login/web/cookie/info"] = `{"code":0,"data":{"refresh":false,"timestamp":1767225600000}}`
	a.Now = func() time.Time { return now.Add(25 * time.Hour) }
	result, err = a.MaintainLogin(context.Background(), result.State)
	if err != nil || result.Outcome != RefreshConfirmed {
		t.Fatalf("retry=%#v err=%v", result, err)
	}
	got := []string{}
	for _, path := range fake.paths() {
		if strings.HasPrefix(path, "/x/passport-login/") {
			got = append(got, path)
		}
	}
	if len(got) != 2 || got[0] != "/x/passport-login/web/confirm/refresh" || got[1] != "/x/passport-login/web/cookie/info" {
		t.Fatalf("requests=%v", got)
	}
	if confirm := fake.call("/x/passport-login/web/confirm/refresh"); confirm.form.Get("refresh_token") != syntheticToken || confirm.form.Get("csrf") != syntheticFreshJC {
		t.Fatalf("confirm=%#v", confirm)
	}
	if stored, _, _ = a.Logins.Load(context.Background()); stored.PendingConfirm != "" {
		t.Fatalf("pending=%q", stored.PendingConfirm)
	}
}

func TestAConfirmRefusedAgainIsDroppedAndTheCheckGoesOn(t *testing.T) {
	now := time.Date(2026, 11, 26, 0, 0, 0, 0, time.UTC)
	fake := refreshingFake()
	fake.replies["/x/passport-login/web/confirm/refresh"] = `{"code":-111,"message":"csrf"}`
	a := NewAdapter(fake.client(), Options{Account: DefaultAccount})
	a.Now = func() time.Time { return now }
	a.Logins = &Logins{Records: &memoryRecords{}, Key: syntheticSecrets}
	if err := a.Logins.Put(context.Background(), storedLogin(now)); err != nil {
		t.Fatal(err)
	}
	result, err := a.MaintainLogin(context.Background(), State{})
	if ErrorCode(err) != "bilibili_login_confirm_failed" {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	fake.calls = nil
	fake.replies["/x/passport-login/web/cookie/info"] = `{"code":0,"data":{"refresh":false,"timestamp":1767225600000}}`
	a.Now = func() time.Time { return now.Add(25 * time.Hour) }
	result, err = a.MaintainLogin(context.Background(), result.State)
	if err != nil || result.Outcome != RefreshCurrent || result.State.Refresh.LastError != "" {
		t.Fatalf("retry=%#v err=%v", result, err)
	}
	if fake.call("/x/passport-login/web/cookie/info") == nil {
		t.Fatal("the refused confirm stopped the check")
	}
	stored, _, _ := a.Logins.Load(context.Background())
	if stored.PendingConfirm != "" || stored.SESSDATA != syntheticFresh {
		t.Fatalf("stored=%#v", stored)
	}
}

func TestAConfirmAnsweredWithAnExpiredLoginMarksItExpired(t *testing.T) {
	now := time.Date(2026, 11, 26, 0, 0, 0, 0, time.UTC)
	fake := refreshingFake()
	fake.replies["/x/passport-login/web/confirm/refresh"] = `{"code":-111,"message":"csrf"}`
	a := NewAdapter(fake.client(), Options{Account: DefaultAccount})
	a.Now = func() time.Time { return now }
	a.Logins = &Logins{Records: &memoryRecords{}, Key: syntheticSecrets}
	if err := a.Logins.Put(context.Background(), storedLogin(now)); err != nil {
		t.Fatal(err)
	}
	result, _ := a.MaintainLogin(context.Background(), State{})
	fake.calls = nil
	fake.replies["/x/passport-login/web/confirm/refresh"] = `{"code":-101,"message":"not signed in"}`
	a.Now = func() time.Time { return now.Add(25 * time.Hour) }
	result, err := a.MaintainLogin(context.Background(), result.State)
	if ErrorCode(err) != "bilibili_login_expired" || result.State.Refresh.LastError != "bilibili_login_expired" || fake.call("/x/passport-login/web/cookie/info") != nil {
		t.Fatalf("retry=%#v err=%v", result, err)
	}
	if status := a.LoginStatus(context.Background(), result.State); status != "expired" {
		t.Fatalf("status=%s", status)
	}
}

func TestAConfirmWithoutAnAnswerStaysPending(t *testing.T) {
	now := time.Date(2026, 11, 26, 0, 0, 0, 0, time.UTC)
	fake := refreshingFake()
	fake.replies["/x/passport-login/web/confirm/refresh"] = `{"code":-111,"message":"csrf"}`
	a := NewAdapter(fake.client(), Options{Account: DefaultAccount})
	a.Now = func() time.Time { return now }
	a.Logins = &Logins{Records: &memoryRecords{}, Key: syntheticSecrets}
	if err := a.Logins.Put(context.Background(), storedLogin(now)); err != nil {
		t.Fatal(err)
	}
	result, _ := a.MaintainLogin(context.Background(), State{})
	fake.calls = nil
	fake.status = map[string]int{"/x/passport-login/web/confirm/refresh": http.StatusBadGateway}
	a.Now = func() time.Time { return now.Add(25 * time.Hour) }
	result, err := a.MaintainLogin(context.Background(), result.State)
	if ErrorCode(err) != "bilibili_upstream_5xx" || fake.call("/x/passport-login/web/cookie/info") != nil {
		t.Fatalf("retry=%#v err=%v", result, err)
	}
	if stored, _, _ := a.Logins.Load(context.Background()); stored.PendingConfirm != syntheticToken {
		t.Fatalf("stored=%#v", stored)
	}
}

func TestARejectedRefreshAndAnExpiredLoginAreRecorded(t *testing.T) {
	now := time.Date(2026, 11, 26, 0, 0, 0, 0, time.UTC)
	for reply, want := range map[string]string{
		`{"code":86095,"message":"refresh_csrf"}`: "bilibili_login_refresh_rejected",
		`{"code":-101,"message":"not signed in"}`: "bilibili_login_expired",
	} {
		fake := refreshingFake()
		fake.replies["/x/passport-login/web/cookie/refresh"] = reply
		a := NewAdapter(fake.client(), Options{Account: DefaultAccount})
		a.Now = func() time.Time { return now }
		a.Logins = &Logins{Records: &memoryRecords{}, Key: syntheticSecrets}
		if err := a.Logins.Put(context.Background(), storedLogin(now)); err != nil {
			t.Fatal(err)
		}
		result, err := a.MaintainLogin(context.Background(), State{})
		if ErrorCode(err) != want || result.State.Refresh.LastError != want || fake.call("/x/passport-login/web/confirm/refresh") != nil {
			t.Fatalf("%s: result=%#v err=%v", reply, result, err)
		}
		if expired := want == "bilibili_login_expired"; expired != (a.LoginStatus(context.Background(), result.State) == "expired") {
			t.Fatalf("%s: status=%s", reply, a.LoginStatus(context.Background(), result.State))
		}
	}
}

func TestMaintenanceWaitsOutTheBackoffAndLeavesTheEnvironmentAlone(t *testing.T) {
	now := time.Date(2026, 11, 26, 0, 0, 0, 0, time.UTC)
	fake := refreshingFake()
	a := NewAdapter(fake.client(), Options{Account: DefaultAccount, SESSDATA: syntheticLogin})
	a.Now = func() time.Time { return now }
	a.Logins = &Logins{Records: &memoryRecords{}, Key: syntheticSecrets}
	if result, _ := a.MaintainLogin(context.Background(), State{}); result.Outcome != "" || len(fake.calls) != 0 {
		t.Fatalf("environment login result=%#v calls=%v", result, fake.paths())
	}
	if err := a.Logins.Put(context.Background(), storedLogin(now)); err != nil {
		t.Fatal(err)
	}
	if result, _ := a.MaintainLogin(context.Background(), State{BlockedUntil: now.Add(time.Hour)}); result.Outcome != "" || len(fake.calls) != 0 {
		t.Fatalf("blocked result=%#v calls=%v", result, fake.paths())
	}
	fake.status["/x/passport-login/web/cookie/info"] = http.StatusPreconditionFailed
	result, err := a.MaintainLogin(context.Background(), State{})
	if !IsRiskControl(ErrorCode(err)) || !result.State.BlockedUntil.After(now) {
		t.Fatalf("captcha result=%#v err=%v", result, err)
	}
}

func TestAScannedCodeSignsInWithTheServersIdentity(t *testing.T) {
	now := time.Date(2026, 11, 26, 0, 0, 0, 0, time.UTC)
	fake := newPassportFake()
	c := fake.client()
	device := Session{Buvid3: "0A1B2C3D-0A1B-0A1B-0A1B-0A1B2C3D4E5F12345infoc", Buvid4: "synthetic-buvid4", Nut: 1767225600}
	code, err := c.QRGenerate(context.Background(), device)
	if err != nil || code.Key != "8587cf8106a0b863c46d6bab913537f6" || !strings.HasPrefix(code.URL, "https://passport.bilibili.com/") {
		t.Fatalf("code=%#v err=%v", code, err)
	}
	if state, _, err := c.QRPoll(context.Background(), device, code.Key, now); err != nil || state != QRWaiting {
		t.Fatalf("waiting=%s err=%v", state, err)
	}
	fake.replies["/x/passport-login/web/qrcode/poll"] = `{"code":0,"data":{"url":"","refresh_token":"` + syntheticNext + `","timestamp":1767225600000,"code":0,"message":""}}`
	fake.set["/x/passport-login/web/qrcode/poll"] = []string{"SESSDATA=" + syntheticFresh + "; Path=/", "bili_jct=" + syntheticFreshJC + "; Path=/", "DedeUserID=1000001; Path=/"}
	state, login, err := c.QRPoll(context.Background(), device, code.Key, now)
	if err != nil || state != QRSignedIn || login.SESSDATA != syntheticFresh || login.RefreshToken != syntheticNext || login.Via != ViaQR || login.Cookies["buvid3"] != device.Buvid3 || login.Cookies["b_nut"] != "1767225600" {
		t.Fatalf("state=%s login=%#v err=%v", state, login, err)
	}
	for _, call := range fake.calls {
		if strings.Contains(call.cookie, "SESSDATA=") {
			t.Fatalf("%s sent a login", call.path)
		}
	}
	fake.replies["/x/passport-login/web/qrcode/generate"] = `{"code":0,"data":{"url":"https://example.com/scan","qrcode_key":"8587cf8106a0b863c46d6bab913537f6"}}`
	if _, err = c.QRGenerate(context.Background(), device); ErrorCode(err) != "bilibili_qr_failed" {
		t.Fatalf("foreign host err=%v", err)
	}
}

func TestSignOutCountsARefusedLoginAsSignedOut(t *testing.T) {
	now := time.Date(2026, 11, 26, 0, 0, 0, 0, time.UTC)
	fake := newPassportFake()
	a := NewAdapter(fake.client(), Options{Account: DefaultAccount})
	a.Now = func() time.Time { return now }
	if _, err := a.SignOut(context.Background(), State{}, storedLogin(now)); err != nil {
		t.Fatal(err)
	}
	exit := fake.call("/login/exit/v2")
	if exit == nil || exit.method != http.MethodPost || exit.form.Get("biliCSRF") != syntheticJCT || !strings.Contains(exit.cookie, "SESSDATA="+syntheticLogin) {
		t.Fatalf("exit=%#v", exit)
	}
	fake.replies["/login/exit/v2"] = `<!DOCTYPE html><html><body>sign in</body></html>`
	if _, err := a.SignOut(context.Background(), State{}, storedLogin(now)); err != nil {
		t.Fatalf("refused login err=%v", err)
	}
	fake.replies["/login/exit/v2"] = `{"code":2202,"message":"csrf"}`
	if _, err := a.SignOut(context.Background(), State{}, storedLogin(now)); ErrorCode(err) != "bilibili_sign_out_failed" {
		t.Fatalf("csrf err=%v", err)
	}
}

func TestCheckLoginAsksNavOnlyWithALoginAndOutsideTheBlock(t *testing.T) {
	now := time.Date(2026, 11, 26, 0, 0, 0, 0, time.UTC)
	fake := newPassportFake()
	fake.replies["/x/web-interface/nav"] = `{"code":-101,"data":{"isLogin":false,"wbi_img":{"img_url":"https://i0.hdslb.com/bfs/wbi/0123456789abcdef0123456789abcdef.png","sub_url":"https://i0.hdslb.com/bfs/wbi/fedcba9876543210fedcba9876543210.png"}}}`
	a := NewAdapter(fake.client(), Options{Account: DefaultAccount})
	a.Now = func() time.Time { return now }
	a.Logins = &Logins{Records: &memoryRecords{}, Key: syntheticSecrets}
	if _, requests, err := a.CheckLogin(context.Background(), State{}); ErrorCode(err) != "bilibili_login_required" || requests != 0 {
		t.Fatalf("without a login requests=%d err=%v", requests, err)
	}
	if err := a.Logins.Put(context.Background(), storedLogin(now)); err != nil {
		t.Fatal(err)
	}
	if _, requests, err := a.CheckLogin(context.Background(), State{BlockedUntil: now.Add(time.Hour)}); ErrorCode(err) != "bilibili_blocked" || requests != 0 {
		t.Fatalf("blocked requests=%d err=%v", requests, err)
	}
	if len(fake.calls) != 0 {
		t.Fatalf("a refused check made requests: %v", fake.paths())
	}
	state, requests, err := a.CheckLogin(context.Background(), State{})
	if err != nil || requests == 0 || !state.WBI.valid() {
		t.Fatalf("check state=%#v requests=%d err=%v", state, requests, err)
	}
	if nav := fake.call("/x/web-interface/nav"); nav == nil || !strings.Contains(nav.cookie, "SESSDATA="+syntheticLogin) {
		t.Fatalf("nav=%v", nav)
	}
	if info := a.Login(context.Background(), state); info.Status != "expired" {
		t.Fatalf("a refused login reads %#v", info)
	}
}
