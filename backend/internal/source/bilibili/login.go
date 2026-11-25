package bilibili

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/yexca/kano-navi/backend/internal/secretstore"
	"github.com/yexca/kano-navi/backend/internal/sourcecapture"
)

// A login the server owns. The operator gives it once, by importing a
// private browser window's cookies and refresh token or by scanning a QR
// code, and no browser uses it afterwards, so only the server refreshes it.
// It is kept sealed with LLM_SECRETS_KEY in one app_settings row (Logins)
// and wins over the environment's login for every read while it is stored.
// Its maintenance requests go to passport.bilibili.com and the correspond
// page; their answers carry the refresh token and the cookies they set, so
// none of them is ever captured (sourcecapture.WithoutCapture).

// refreshKeyPEM is the public key Bilibili's web script encrypts the
// correspond path with. When Bilibili replaces it, the correspond page
// answers 404 and refreshes fail until this constant is updated.
const refreshKeyPEM = `-----BEGIN PUBLIC KEY-----
MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQDLgd2OAkcGVtoE3ThUREbio0Eg
Uc/prcajMKXvkCKFCWhJYJcLkcM2DKKcSeFpD/j6Boy538YXnR6VhcuUJOhH2x71
nzPjfdTcqMz7djHum0qSZA0AyCBDABUqCrfNgCiJ00Ra7GmRj+YCK1NJEuewlb40
JNrRuoEUXpabUzGB8QIDAQAB
-----END PUBLIC KEY-----`

var refreshKey = func() *rsa.PublicKey {
	block, _ := pem.Decode([]byte(refreshKeyPEM))
	if block == nil {
		panic("bilibili: malformed refresh key")
	}
	key, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		panic("bilibili: malformed refresh key")
	}
	return key.(*rsa.PublicKey)
}()

// correspondPath is the path of the page that renders the refresh's CSRF
// token, which is refresh_<timestamp> encrypted with RSA-OAEP (SHA-256) in
// lowercase hex.
func correspondPath(key *rsa.PublicKey, timestamp int64, random io.Reader) (string, error) {
	sealed, err := rsa.EncryptOAEP(sha256.New(), random, key, []byte("refresh_"+strconv.FormatInt(timestamp, 10)), nil)
	if err != nil {
		return "", &Error{"bilibili_login_refresh_failed"}
	}
	return hex.EncodeToString(sealed), nil
}

// StoredLogin is a login the server owns. It is sealed at rest and never
// logged, captured or returned.
type StoredLogin struct {
	SESSDATA string `json:"sessdata"`
	// Cookies are the account cookies (bili_jct, DedeUserID,
	// DedeUserID__ckMd5, sid) and the device identity the login signed in
	// with.
	Cookies      LoginCookies `json:"cookies"`
	RefreshToken string       `json:"refreshToken"`
	// PendingConfirm is the refresh token a refresh replaced, kept until
	// its confirm succeeds.
	PendingConfirm string `json:"pendingConfirm,omitempty"`
	// Via is how the operator gave the login: "import" or "qr".
	Via         string    `json:"via"`
	StoredAt    time.Time `json:"storedAt"`
	RefreshedAt time.Time `json:"refreshedAt"`
}

// Ways a login is given.
const (
	ViaImport = "import"
	ViaQR     = "qr"
)

// refreshTokenPattern is a refresh token (localStorage ac_time_value).
// Bilibili's are 32 hex digits; the pattern allows any short token without
// separators.
var refreshTokenPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{16,128}$`)

func (l StoredLogin) csrf() string { return (&l.Cookies).value("bili_jct") }

func (l StoredLogin) valid() bool {
	return sessdata.MatchString(l.SESSDATA) && l.csrf() != "" && refreshTokenPattern.MatchString(l.RefreshToken) && !l.StoredAt.IsZero()
}

// key names the login in the check bookkeeping, so a check of one login
// never counts for another.
func (l StoredLogin) key() string {
	return "stored:" + l.StoredAt.UTC().Format(time.RFC3339Nano)
}

// signed is session with the login's cookies.
func (l StoredLogin) signed(session Session) Session {
	cookies := LoginCookies{}
	for name, value := range l.Cookies {
		cookies[name] = value
	}
	session.Login, session.Account = l.SESSDATA, &cookies
	return session
}

// ImportLogin reads an operator's import: the Cookie header of a private
// window signed in to the account, and that window's localStorage
// ac_time_value. SESSDATA, bili_jct and a well-formed token are required.
func ImportLogin(cookie, token string, now time.Time) (StoredLogin, error) {
	login, cookies := ParseLoginCookies(cookie)
	token = strings.Trim(strings.TrimSpace(token), `"`)
	switch {
	case login == "" || cookies["bili_jct"] == "":
		return StoredLogin{}, &Error{"bilibili_login_cookie_invalid"}
	case !refreshTokenPattern.MatchString(token):
		return StoredLogin{}, &Error{"bilibili_login_token_invalid"}
	}
	return StoredLogin{SESSDATA: login, Cookies: cookies, RefreshToken: token, Via: ViaImport, StoredAt: now.UTC()}, nil
}

// LoginRecords is where the sealed login is kept. sqlite.Store implements
// it with one app_settings row.
type LoginRecords interface {
	// BilibiliLoginRecord is the sealed record, or "" when none is stored.
	BilibiliLoginRecord(ctx context.Context) (string, error)
	// SwapBilibiliLoginRecord replaces the record with to when it still is
	// from ("" for none); an empty to deletes it. It reports whether the
	// record was replaced.
	SwapBilibiliLoginRecord(ctx context.Context, from, to string) (bool, error)
}

var (
	// ErrNoLogin is returned while no login is stored.
	ErrNoLogin = errors.New("bilibili: no stored login")
	// ErrLoginUnreadable is a stored record that does not open with the
	// current LLM_SECRETS_KEY or is malformed.
	ErrLoginUnreadable = errors.New("bilibili: stored login unreadable")
	// ErrLoginChanged is a write that lost to another one since its read.
	ErrLoginChanged = errors.New("bilibili: stored login changed")
)

// Logins keeps the stored login sealed with Key (LLM_SECRETS_KEY). Every
// write is a compare and swap on the sealed value it read, so a second
// process can never put an older login back.
type Logins struct {
	Records LoginRecords
	Key     string
}

// Load returns the stored login and its version (the sealed value).
func (l *Logins) Load(ctx context.Context) (StoredLogin, string, error) {
	if l == nil || l.Records == nil {
		return StoredLogin{}, "", ErrNoLogin
	}
	sealed, err := l.Records.BilibiliLoginRecord(ctx)
	if err != nil {
		return StoredLogin{}, "", err
	}
	if sealed == "" {
		return StoredLogin{}, "", ErrNoLogin
	}
	plain, err := secretstore.Decrypt(sealed, l.Key)
	if err != nil {
		return StoredLogin{}, sealed, ErrLoginUnreadable
	}
	var login StoredLogin
	if json.Unmarshal([]byte(plain), &login) != nil || !login.valid() {
		return StoredLogin{}, sealed, ErrLoginUnreadable
	}
	return login, sealed, nil
}

// Unreadable reports a stored record that does not open.
func (l *Logins) Unreadable(ctx context.Context) (bool, error) {
	_, _, err := l.Load(ctx)
	switch {
	case errors.Is(err, ErrLoginUnreadable):
		return true, nil
	case err == nil || errors.Is(err, ErrNoLogin):
		return false, nil
	}
	return false, err
}

// Replace stores next when the record still is version ("" for none), and
// returns the new version; ErrLoginChanged when another write came first.
func (l *Logins) Replace(ctx context.Context, version string, next StoredLogin) (string, error) {
	if l == nil || l.Records == nil {
		return "", ErrNoLogin
	}
	plain, err := json.Marshal(next)
	if err != nil {
		return "", err
	}
	sealed, err := secretstore.Encrypt(string(plain), l.Key)
	if err != nil {
		return "", err
	}
	swapped, err := l.Records.SwapBilibiliLoginRecord(ctx, version, sealed)
	if err != nil {
		return "", err
	}
	if !swapped {
		return "", ErrLoginChanged
	}
	return sealed, nil
}

// Put stores login in place of whatever is stored, readable or not.
func (l *Logins) Put(ctx context.Context, login StoredLogin) error {
	if !login.valid() {
		return &Error{"bilibili_login_cookie_invalid"}
	}
	for range 3 {
		_, version, err := l.Load(ctx)
		if err != nil && !errors.Is(err, ErrNoLogin) && !errors.Is(err, ErrLoginUnreadable) {
			return err
		}
		if _, err = l.Replace(ctx, version, login); !errors.Is(err, ErrLoginChanged) {
			return err
		}
	}
	return ErrLoginChanged
}

// Forget deletes the record when it still is version.
func (l *Logins) Forget(ctx context.Context, version string) error {
	if l == nil || l.Records == nil || version == "" {
		return ErrNoLogin
	}
	swapped, err := l.Records.SwapBilibiliLoginRecord(ctx, version, "")
	if err != nil {
		return err
	}
	if !swapped {
		return ErrLoginChanged
	}
	return nil
}

// passport makes one request to passport.bilibili.com or the correspond
// page with the login, never captured. It answers the body and the cookies
// set.
func (c *Client) passport(ctx context.Context, r request) ([]byte, []*http.Cookie, error) {
	r.login, r.referer = true, "https://www.bilibili.com/"
	if r.accept == "" {
		r.accept = "application/json"
	}
	return c.send(sourcecapture.WithoutCapture(ctx), r)
}

// passportCode reads a passport answer's code: 0 returns data, -101 is
// bilibili_login_expired, the captcha answers keep their codes, and any
// other code is failure.
func passportCode(body []byte, failure string) (json.RawMessage, int, error) {
	var out struct {
		Code *int            `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	if json.Unmarshal(body, &out) != nil || out.Code == nil {
		if _, err := envelope(body); err != nil && IsRiskControl(ErrorCode(err)) {
			return nil, 0, err
		}
		return nil, 0, &Error{failure}
	}
	switch *out.Code {
	case 0:
		return out.Data, 0, nil
	case -101:
		return nil, -101, &Error{"bilibili_login_expired"}
	case -352, -412, -799, -509:
		_, err := envelope(body)
		return nil, *out.Code, err
	}
	return nil, *out.Code, &Error{failure}
}

// CookieInfo asks whether Bilibili wants the login refreshed, and the
// timestamp (milliseconds) a refresh encrypts.
func (c *Client) CookieInfo(ctx context.Context, session Session) (bool, int64, error) {
	// csrf is optional here, and a cookie never goes into a URL.
	body, _, err := c.passport(ctx, request{method: http.MethodGet, host: passportHost, path: "/x/passport-login/web/cookie/info", session: &session})
	if err != nil {
		return false, 0, err
	}
	data, _, err := passportCode(body, "bilibili_login_refresh_failed")
	if err != nil {
		return false, 0, err
	}
	var out struct {
		Refresh   bool  `json:"refresh"`
		Timestamp int64 `json:"timestamp"`
	}
	if json.Unmarshal(data, &out) != nil || (out.Refresh && out.Timestamp <= 0) {
		return false, 0, &Error{"bilibili_login_refresh_failed"}
	}
	return out.Refresh, out.Timestamp, nil
}

// refreshCSRFPattern finds the token the correspond page renders.
var refreshCSRFPattern = regexp.MustCompile(`<div id="1-name">\s*([0-9a-f]{32})\s*</div>`)

// RefreshCSRF reads the refresh's CSRF token from the correspond page of
// path.
func (c *Client) RefreshCSRF(ctx context.Context, session Session, path string) (string, error) {
	if !correspondPathPattern.MatchString(path) {
		return "", &Error{"bilibili_login_refresh_failed"}
	}
	body, _, err := c.passport(ctx, request{method: http.MethodGet, host: wwwHost, path: correspondPrefix + path, accept: "text/html", session: &session})
	if err != nil {
		if code := ErrorCode(err); IsRiskControl(code) || code == "bilibili_timeout" || code == "bilibili_unavailable" {
			return "", err
		}
		return "", &Error{"bilibili_login_refresh_failed"}
	}
	match := refreshCSRFPattern.FindSubmatch(body)
	if match == nil {
		return "", &Error{"bilibili_login_refresh_failed"}
	}
	return string(match[1]), nil
}

var correspondPathPattern = regexp.MustCompile(`^[0-9a-f]{64,512}$`)

// RefreshCookie trades the login's refresh token for new cookies: SESSDATA
// and the account cookies the answer sets, and the next refresh token.
// 86095 (the token does not belong to the cookie, usually because something
// else refreshed the same login) is bilibili_login_refresh_rejected.
func (c *Client) RefreshCookie(ctx context.Context, session Session, refreshCSRF, token string) (string, LoginCookies, string, error) {
	form := url.Values{"csrf": {session.Account.value("bili_jct")}, "refresh_csrf": {refreshCSRF}, "source": {"main_web"}, "refresh_token": {token}}
	body, set, err := c.passport(ctx, request{method: http.MethodPost, host: passportHost, path: "/x/passport-login/web/cookie/refresh", form: form, session: &session})
	if err != nil {
		return "", nil, "", err
	}
	data, code, err := passportCode(body, "bilibili_login_refresh_failed")
	if code == 86095 {
		return "", nil, "", &Error{"bilibili_login_refresh_rejected"}
	}
	if err != nil {
		return "", nil, "", err
	}
	var out struct {
		RefreshToken string `json:"refresh_token"`
	}
	login, cookies := setLoginCookies(set)
	if json.Unmarshal(data, &out) != nil || !refreshTokenPattern.MatchString(out.RefreshToken) || login == "" || cookies["bili_jct"] == "" {
		return "", nil, "", &Error{"bilibili_login_refresh_failed"}
	}
	return login, cookies, out.RefreshToken, nil
}

// ConfirmRefresh tells Bilibili the refresh arrived: session carries the
// new cookies and oldToken is the token the refresh replaced. Bilibili then
// invalidates the old cookie. A request that got no answer keeps its own
// code, and -101 is bilibili_login_expired; bilibili_login_confirm_failed
// means Bilibili answered and refused.
func (c *Client) ConfirmRefresh(ctx context.Context, session Session, oldToken string) error {
	form := url.Values{"csrf": {session.Account.value("bili_jct")}, "refresh_token": {oldToken}}
	body, _, err := c.passport(ctx, request{method: http.MethodPost, host: passportHost, path: "/x/passport-login/web/confirm/refresh", form: form, session: &session})
	if err != nil {
		return err
	}
	_, _, err = passportCode(body, "bilibili_login_confirm_failed")
	return err
}

// SignOut ends the login's session at Bilibili. A login Bilibili no longer
// accepts (passport answers its sign-in page) is bilibili_login_expired.
func (c *Client) SignOut(ctx context.Context, session Session) error {
	form := url.Values{"biliCSRF": {session.Account.value("bili_jct")}}
	body, _, err := c.passport(ctx, request{method: http.MethodPost, host: passportHost, path: "/login/exit/v2", form: form, session: &session})
	if err != nil {
		return err
	}
	if !json.Valid(body) {
		if strings.Contains(strings.ToLower(string(body[:min(len(body), 512)])), "<!doctype html") {
			return &Error{"bilibili_login_expired"}
		}
		return &Error{"bilibili_sign_out_failed"}
	}
	_, _, err = passportCode(body, "bilibili_sign_out_failed")
	return err
}

// setLoginCookies keeps SESSDATA and the account cookies an answer set,
// when well formed.
func setLoginCookies(set []*http.Cookie) (string, LoginCookies) {
	login, cookies := "", LoginCookies{}
	for _, cookie := range set {
		switch {
		case cookie.Name == "SESSDATA" && sessdata.MatchString(cookie.Value):
			login = cookie.Value
		case slices.Contains(loginAccountCookies, cookie.Name) && cookieValue.MatchString(cookie.Value):
			cookies[cookie.Name] = cookie.Value
		}
	}
	return login, cookies
}

// QRCode is one web QR sign-in: the address the code encodes and the key
// its poll names. Both are secrets until the code expires, since whoever
// polls with the key after the scan receives the login.
type QRCode struct{ URL, Key string }

// QR sign-in states.
const (
	QRWaiting  = "waiting"
	QRScanned  = "scanned"
	QRSignedIn = "signed_in"
	QRExpired  = "expired"
	QRFailed   = "failed"
)

var qrKeyPattern = regexp.MustCompile(`^[0-9A-Za-z]{32}$`)

// qrHosts are the hosts a QR sign-in address may name.
var qrHosts = map[string]bool{"passport.bilibili.com": true, "account.bilibili.com": true}

// QRGenerate starts a web QR sign-in for the device identity of session.
func (c *Client) QRGenerate(ctx context.Context, session Session) (QRCode, error) {
	session.Login, session.Account = "", nil
	body, _, err := c.passport(ctx, request{method: http.MethodGet, host: passportHost, path: "/x/passport-login/web/qrcode/generate", session: &session})
	if err != nil {
		return QRCode{}, err
	}
	data, _, err := passportCode(body, "bilibili_qr_failed")
	if err != nil {
		return QRCode{}, err
	}
	var out struct {
		URL string `json:"url"`
		Key string `json:"qrcode_key"`
	}
	if json.Unmarshal(data, &out) != nil || !qrKeyPattern.MatchString(out.Key) || len(out.URL) > 512 {
		return QRCode{}, &Error{"bilibili_qr_failed"}
	}
	u, err := url.Parse(out.URL)
	if err != nil || u.Scheme != "https" || !qrHosts[u.Host] || u.User != nil {
		return QRCode{}, &Error{"bilibili_qr_failed"}
	}
	return QRCode{URL: out.URL, Key: out.Key}, nil
}

// QRPoll asks once how the sign-in of key stands. A success answers the
// login: the cookies the answer set, its refresh token and the device
// identity of session, which the sign-in used.
func (c *Client) QRPoll(ctx context.Context, session Session, key string, now time.Time) (string, StoredLogin, error) {
	if !qrKeyPattern.MatchString(key) {
		return QRFailed, StoredLogin{}, &Error{"bilibili_qr_failed"}
	}
	session.Login, session.Account = "", nil
	body, set, err := c.passport(ctx, request{method: http.MethodGet, host: passportHost, path: "/x/passport-login/web/qrcode/poll",
		rawQuery: url.Values{"qrcode_key": {key}}.Encode(), session: &session})
	if err != nil {
		return QRFailed, StoredLogin{}, err
	}
	data, _, err := passportCode(body, "bilibili_qr_failed")
	if err != nil {
		return QRFailed, StoredLogin{}, err
	}
	var out struct {
		Code         *int   `json:"code"`
		RefreshToken string `json:"refresh_token"`
	}
	if json.Unmarshal(data, &out) != nil || out.Code == nil {
		return QRFailed, StoredLogin{}, &Error{"bilibili_qr_failed"}
	}
	switch *out.Code {
	case 86101:
		return QRWaiting, StoredLogin{}, nil
	case 86090:
		return QRScanned, StoredLogin{}, nil
	case 86038:
		return QRExpired, StoredLogin{}, nil
	case 0:
	default:
		return QRFailed, StoredLogin{}, &Error{"bilibili_qr_failed"}
	}
	login, cookies := setLoginCookies(set)
	cookies["buvid3"], cookies["buvid4"] = session.Buvid3, session.Buvid4
	if session.Nut > 0 {
		cookies["b_nut"] = strconv.FormatInt(session.Nut, 10)
	}
	stored := StoredLogin{SESSDATA: login, Cookies: cookies, RefreshToken: out.RefreshToken, Via: ViaQR, StoredAt: now.UTC()}
	if !stored.valid() {
		return QRFailed, StoredLogin{}, &Error{"bilibili_qr_failed"}
	}
	return QRSignedIn, stored, nil
}

// newCorrespondPath is the correspond path of timestamp under Bilibili's
// key.
func newCorrespondPath(timestamp int64) (string, error) {
	return correspondPath(refreshKey, timestamp, rand.Reader)
}
