package adminapi

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/yexca/kano-navi/backend/internal/httpx"
)

// biliAuthClient forwards the console's Bilibili login requests to the
// collector's loopback endpoint. The collector is the process that owns the
// Bilibili client and its cookie jar, so the console never talks to
// passport.bilibili.com itself and the qrcode_key never leaves the two
// processes.
type biliAuthClient struct {
	base   string
	client *http.Client
}

// newBiliAuthClient returns nil for an empty base: the panel then reports the
// feature as unavailable instead of failing every request.
func newBiliAuthClient(base string) *biliAuthClient {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base == "" {
		return nil
	}
	return &biliAuthClient{
		base:   base,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// biliLoginStatus is what the panel polls on open.
type biliLoginStatus struct {
	Available bool `json:"available"`
	LoggedIn  bool `json:"loggedIn"`
}

// biliLoginQR carries the code as a data URL: the console renders it with a
// plain <img>, which would not carry the session header a fetch would.
type biliLoginQR struct {
	Key   string `json:"key"`
	Image string `json:"image"`
}

// biliLoginPoll is one step of the scan check. Code follows Bilibili's own
// numbers: 0 signed in, 86090 scanned but unconfirmed, 86101 not scanned yet,
// 86038 expired. Pending covers both waiting states; Scanned only the one
// where the phone has read the code, so the panel can word them differently.
type biliLoginPoll struct {
	Code     int    `json:"code"`
	Pending  bool   `json:"pending"`
	Scanned  bool   `json:"scanned"`
	LoggedIn bool   `json:"loggedIn"`
	Message  string `json:"message"`
}

// status reads the collector's current session state.
func (c *biliAuthClient) status(r *http.Request) (biliLoginStatus, error) {
	response, err := c.get(r, "/status", nil)
	if err != nil {
		return biliLoginStatus{}, err
	}
	defer response.Body.Close()
	var payload struct {
		LoggedIn bool `json:"loggedIn"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return biliLoginStatus{}, err
	}
	return biliLoginStatus{Available: true, LoggedIn: payload.LoggedIn}, nil
}

// qr issues a fresh login code and returns it already encoded for the browser.
func (c *biliAuthClient) qr(r *http.Request) (biliLoginQR, error) {
	response, err := c.get(r, "/qr", nil)
	if err != nil {
		return biliLoginQR{}, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return biliLoginQR{}, err
	}
	key := strings.TrimSpace(response.Header.Get("X-Qrcode-Key"))
	return biliLoginQR{
		Key:   key,
		Image: "data:image/png;base64," + base64.StdEncoding.EncodeToString(body),
	}, nil
}

// poll checks whether the code has been confirmed.
func (c *biliAuthClient) poll(r *http.Request, key string) (biliLoginPoll, error) {
	response, err := c.get(r, "/poll", url.Values{"key": {key}})
	if err != nil {
		return biliLoginPoll{}, err
	}
	defer response.Body.Close()
	var payload biliLoginPoll
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return biliLoginPoll{}, err
	}
	return payload, nil
}

// get performs one loopback call, carrying the request context so a closed
// console tab does not leave the collector answering nobody.
func (c *biliAuthClient) get(r *http.Request, path string, query url.Values) (*http.Response, error) {
	target := c.base + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	request, err := http.NewRequestWithContext(r.Context(), http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	response, err := c.client.Do(request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		return nil, errBiliAuthUnavailable
	}
	return response, nil
}

var errBiliAuthUnavailable = &biliAuthError{message: "bilibili login service unavailable"}

type biliAuthError struct{ message string }

func (e *biliAuthError) Error() string { return e.message }

// biliLoginStatus answers whether the collector can drive a login right now.
func (a *api) biliLoginStatus(w http.ResponseWriter, r *http.Request) {
	if a.biliAuth == nil {
		httpx.WriteJSON(w, http.StatusOK, biliLoginStatus{})
		return
	}
	status, err := a.biliAuth.status(r)
	if err != nil {
		httpx.WriteJSON(w, http.StatusOK, biliLoginStatus{})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, status)
}

// biliLoginQR issues a code for the console to display.
func (a *api) biliLoginQR(w http.ResponseWriter, r *http.Request) {
	if a.biliAuth == nil {
		httpx.WriteCode(w, http.StatusServiceUnavailable, "bilibili_login_unavailable")
		return
	}
	qr, err := a.biliAuth.qr(r)
	if err != nil {
		httpx.WriteCode(w, http.StatusServiceUnavailable, "bilibili_login_unavailable")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, qr)
}

// biliLoginPoll reports the scan state for one code.
func (a *api) biliLoginPoll(w http.ResponseWriter, r *http.Request) {
	if a.biliAuth == nil {
		httpx.WriteCode(w, http.StatusServiceUnavailable, "bilibili_login_unavailable")
		return
	}
	key := strings.TrimSpace(r.URL.Query().Get("key"))
	if key == "" {
		httpx.WriteCode(w, http.StatusBadRequest, "invalid_request")
		return
	}
	poll, err := a.biliAuth.poll(r, key)
	if err != nil {
		httpx.WriteCode(w, http.StatusServiceUnavailable, "bilibili_login_unavailable")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, poll)
}
