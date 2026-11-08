// Package bilibili reads the public uploads, dynamics and live room of one
// Bilibili account.
//
// Bilibili's anonymous web API answers a few requests and then puts the
// caller's address behind a captcha (HTTP 412 or an HTML page, sometimes the
// JSON codes -352 and -412) for hours. The client therefore makes one request
// per call, never retries, reports that state as bilibili_risk_control, and
// leaves the waiting to the adapter's persisted backoff. It requests a fixed
// set of hosts: api.bilibili.com (uploads, the device identity, the WBI keys
// and the signed-in dynamics listing), www.bilibili.com (one dynamic's opus
// page only) and api.live.bilibili.com (the live room status).
package bilibili

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// MaxResponseBytes bounds one API response; a page of 20 uploads is far
// smaller.
const MaxResponseBytes = 2 * 1024 * 1024

// The fixed destinations. The login cookie goes only to apiHost; the opus
// page gets the device identity alone and the live host no cookie at all.
const (
	apiHost  = "api.bilibili.com"
	wwwHost  = "www.bilibili.com"
	liveHost = "api.live.bilibili.com"
)

type Error struct{ Code string }

func (e *Error) Error() string { return e.Code }

// Session is the anonymous device identity Bilibili's own pages keep in
// their cookies (buvid3, buvid4, b_nut and the bili_ticket JWT), plus the
// operator's optional login cookie. Only the device identity is persisted;
// Login comes from the environment on every request and is never stored,
// logged or returned.
type Session struct {
	Buvid3 string `json:"buvid3"`
	Buvid4 string `json:"buvid4"`
	// Nut is the device's first visit (b_nut), unix seconds.
	Nut int64 `json:"bNut,omitempty"`
	// Ticket is the bili_ticket cookie; TicketAt and TicketTTL are when it
	// was issued and how long it lasts, in seconds.
	Ticket    string `json:"ticket,omitempty"`
	TicketAt  int64  `json:"ticketAt,omitempty"`
	TicketTTL int64  `json:"ticketTTL,omitempty"`
	Login     string `json:"-"`
}

// BrowserUserAgent is the desktop browser every request names, as Bilibili's
// own pages are read; an unknown client is far more often sent to the
// captcha. It is updated deliberately, together with the docs.
const BrowserUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/142.0.7444.163 Safari/537.36"

// cookie is the Cookie header for a request: the device identity, then the
// login cookie when login is set and the host is the API.
func (s Session) cookie(login bool, host string) string {
	parts := []string{"buvid3=" + s.Buvid3, "buvid4=" + s.Buvid4}
	if s.Nut > 0 {
		parts = append(parts, "b_nut="+strconv.FormatInt(s.Nut, 10))
	}
	if ticketPattern.MatchString(s.Ticket) {
		parts = append(parts, "bili_ticket="+s.Ticket, "bili_ticket_expires="+strconv.FormatInt(s.TicketAt+s.TicketTTL, 10))
	}
	if login && host == apiHost && sessdata.MatchString(s.Login) {
		parts = append(parts, "SESSDATA="+s.Login)
	}
	return strings.Join(parts, "; ")
}

func (s Session) valid() bool {
	return buvid3.MatchString(s.Buvid3) && buvid4.MatchString(s.Buvid4)
}

var (
	buvid3 = regexp.MustCompile(`^[A-Fa-f0-9-]{20,64}infoc$`)
	buvid4 = regexp.MustCompile(`^[A-Za-z0-9+/=_-]{10,200}$`)
	mid    = regexp.MustCompile(`^[1-9][0-9]{0,15}$`)
	// sessdata is the login cookie as browsers store it (URL-encoded, so no
	// separators that could add another cookie or header).
	sessdata = regexp.MustCompile(`^[A-Za-z0-9%*,._-]{16,512}$`)
	// ticketPattern is a JWT: three base64url parts.
	ticketPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{8,512}\.[A-Za-z0-9_-]{8,1000}\.[A-Za-z0-9_-]{8,512}$`)
)

// IsRiskControl reports a code that starts the captcha backoff: one of the
// risk-control answers, split by how Bilibili gave it, or a rate limit.
func IsRiskControl(code string) bool {
	return strings.HasPrefix(code, "bilibili_risk_control") || code == "bilibili_rate_limited"
}

type Client struct{ http *http.Client }

func NewClient(client *http.Client) *Client {
	if client == nil {
		client = &http.Client{}
	}
	copy := *client
	copy.Timeout = 10 * time.Second
	copy.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Client{http: &copy}
}

// get requests one api.bilibili.com path and returns the envelope's data.
func (c *Client) get(ctx context.Context, path string, query url.Values, referer string, session *Session) (json.RawMessage, error) {
	body, err := c.fetch(ctx, apiHost, path, query.Encode(), referer, "application/json", session, true)
	if err != nil {
		return nil, err
	}
	return envelope(body)
}

// fetch requests one path on a fixed host with GET and returns the bounded
// body.
func (c *Client) fetch(ctx context.Context, host, path, rawQuery, referer, accept string, session *Session, login bool) ([]byte, error) {
	return c.do(ctx, http.MethodGet, host, path, rawQuery, referer, accept, session, login)
}

// do requests one path on a fixed host as a browser would: its user agent
// and language, the page it came from as Referer and, for the API hosts,
// that page's Origin. The device identity is sent when session is set, and
// the login cookie only when login is true as well and the host is the API.
func (c *Client) do(ctx context.Context, method, host, path, rawQuery, referer, accept string, session *Session, login bool) ([]byte, error) {
	u := url.URL{Scheme: "https", Host: host, Path: path, RawQuery: rawQuery}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), nil)
	if err != nil {
		return nil, &Error{"bilibili_invalid_request"}
	}
	req.Header.Set("User-Agent", BrowserUserAgent)
	req.Header.Set("Accept", accept)
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
	if referer != "" {
		req.Header.Set("Referer", referer)
		if host != wwwHost {
			if from, parseErr := url.Parse(referer); parseErr == nil && from.Host != "" {
				req.Header.Set("Origin", from.Scheme+"://"+from.Host)
			}
		}
	}
	if session != nil {
		req.Header.Set("Cookie", session.cookie(login, host))
	}
	resp, err := c.http.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, &Error{"bilibili_timeout"}
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, &Error{"bilibili_unavailable"}
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode >= 300 && resp.StatusCode < 400:
		return nil, &Error{"bilibili_redirect_rejected"}
	case resp.StatusCode == http.StatusPreconditionFailed:
		return nil, &Error{"bilibili_risk_control_http"}
	case resp.StatusCode == http.StatusTooManyRequests:
		return nil, &Error{"bilibili_rate_limited"}
	case resp.StatusCode == http.StatusNotFound && host == wwwHost:
		return nil, &Error{"bilibili_not_found"}
	case resp.StatusCode >= 500:
		return nil, &Error{"bilibili_upstream_5xx"}
	case resp.StatusCode != http.StatusOK:
		return nil, &Error{"bilibili_http_error"}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxResponseBytes+1))
	if err != nil {
		return nil, &Error{"bilibili_body_unavailable"}
	}
	if len(body) > MaxResponseBytes {
		return nil, &Error{"bilibili_response_oversized"}
	}
	return body, nil
}

// envelope reads Bilibili's {code, message, data}. An HTML answer is the
// captcha page. The risk-control codes name how Bilibili answered, for
// diagnosis; IsRiskControl treats them alike.
func envelope(body []byte) (json.RawMessage, error) {
	var out struct {
		Code *int            `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	if json.Unmarshal(body, &out) != nil || out.Code == nil {
		if strings.Contains(strings.ToLower(string(body[:min(len(body), 512)])), "<!doctype html") {
			return nil, &Error{"bilibili_risk_control_page"}
		}
		return nil, &Error{"bilibili_invalid_response"}
	}
	switch *out.Code {
	case 0:
		return out.Data, nil
	case -352:
		return nil, &Error{"bilibili_risk_control_352"}
	case -412:
		return nil, &Error{"bilibili_risk_control_412"}
	case -799, -509:
		return nil, &Error{"bilibili_rate_limited"}
	case -404, 4101131, 4101105, 62002, 62004, 62012:
		return nil, &Error{"bilibili_not_found"}
	case -101:
		return nil, &Error{"bilibili_login_expired"}
	}
	return nil, &Error{"bilibili_api_error"}
}

// NewSession requests the anonymous device identity.
func (c *Client) NewSession(ctx context.Context) (Session, error) {
	data, err := c.get(ctx, "/x/frontend/finger/spi", nil, "https://www.bilibili.com/", nil)
	if err != nil {
		return Session{}, err
	}
	var out struct {
		B3 string `json:"b_3"`
		B4 string `json:"b_4"`
	}
	session := Session{}
	if json.Unmarshal(data, &out) == nil {
		session = Session{Buvid3: out.B3, Buvid4: out.B4}
	}
	if !session.valid() {
		return Session{}, &Error{"bilibili_invalid_response"}
	}
	return session, nil
}

// Uploads lists an account's newest uploads, newest first.
func (c *Client) Uploads(ctx context.Context, account string, pageSize int, session Session) ([]Video, error) {
	page, err := c.UploadsPage(ctx, account, pageSize, 1, session)
	return page.Videos, err
}

// UploadsPage lists one page of an account's uploads, newest first; page 1
// is the newest.
func (c *Client) UploadsPage(ctx context.Context, account string, pageSize, number int, session Session) (UploadsPage, error) {
	if !mid.MatchString(account) {
		return UploadsPage{}, &Error{"bilibili_invalid_account"}
	}
	if !session.valid() {
		return UploadsPage{}, &Error{"bilibili_invalid_session"}
	}
	if pageSize < 1 || pageSize > 50 || number < 1 {
		return UploadsPage{}, &Error{"bilibili_invalid_page"}
	}
	query := url.Values{"mid": {account}, "keywords": {""}, "ps": {strconv.Itoa(pageSize)}, "pn": {strconv.Itoa(number)}, "orderby": {"pubdate"}}
	data, err := c.get(ctx, "/x/series/recArchivesByKeywords", query, "https://space.bilibili.com/"+account, &session)
	if err != nil {
		return UploadsPage{}, err
	}
	return ParseUploadsPage(data, account)
}

// Video reads one upload by its BV ID.
func (c *Client) Video(ctx context.Context, bvid string, session Session) (Video, error) {
	if !bvidPattern.MatchString(bvid) {
		return Video{}, &Error{"bilibili_invalid_video"}
	}
	if !session.valid() {
		return Video{}, &Error{"bilibili_invalid_session"}
	}
	data, err := c.get(ctx, "/x/web-interface/view", url.Values{"bvid": {bvid}}, "https://www.bilibili.com/video/"+bvid, &session)
	if err != nil {
		return Video{}, err
	}
	return ParseView(data, bvid)
}

type Options struct {
	// Account is the uploader's numeric mid.
	Account string
	// MinInterval spaces list reads, whatever the source schedule.
	MinInterval time.Duration
	PageSize    int
	// SESSDATA is an operator's Bilibili login cookie from BILIBILI_SESSDATA.
	// Signed-in reads meet the captcha far less often. It is a credential of
	// that account: it stays in memory and goes only to api.bilibili.com.
	SESSDATA string
}

// SignedIn reports whether a valid login cookie is configured.
func (o Options) SignedIn() bool { return sessdata.MatchString(o.SESSDATA) }

// DefaultAccount is Kano's public space, linked from the resource directory.
const DefaultAccount = "316381099"

func OptionsFromEnv() Options {
	account := strings.TrimSpace(os.Getenv("BILIBILI_UID"))
	if !mid.MatchString(account) {
		account = DefaultAccount
	}
	minutes, err := strconv.Atoi(os.Getenv("BILIBILI_MIN_INTERVAL_MINUTES"))
	if err != nil || minutes < 60 || minutes > 1440 {
		minutes = 180
	}
	login := strings.TrimSpace(os.Getenv("BILIBILI_SESSDATA"))
	if !sessdata.MatchString(login) {
		login = ""
	}
	return Options{Account: account, MinInterval: time.Duration(minutes) * time.Minute, PageSize: 20, SESSDATA: login}
}
