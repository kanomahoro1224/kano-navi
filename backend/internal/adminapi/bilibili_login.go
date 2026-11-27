package adminapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"rsc.io/qr"

	"github.com/yexca/kano-navi/backend/internal/httpx"
	"github.com/yexca/kano-navi/backend/internal/secretstore"
	bilisource "github.com/yexca/kano-navi/backend/internal/source/bilibili"
	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
)

// The Bilibili login the server owns (bilisource.Logins): an operator
// imports one or signs one in by QR code, and forgets it. No answer carries
// a cookie, a token or an account ID; the QR code is shown only while its
// sign-in waits.

// qrLifetime is how long Bilibili keeps a QR sign-in open.
const qrLifetime = 180 * time.Second

// qrPollInterval spaces the server's polls of one sign-in; tests shorten
// it.
var qrPollInterval = 3 * time.Second

// bilibiliQR is the one QR sign-in this process runs at a time. The server
// polls Bilibili itself; the console only reads the local state.
type bilibiliQR struct {
	// starting serializes starts, so a second start answers the sign-in the
	// first opened instead of sending its own requests.
	starting sync.Mutex
	mu       sync.Mutex
	// state is one of the bilisource QR states, or "" before the first
	// sign-in.
	state, code string
	expiresAt   time.Time
	// picture is the code to scan; it is dropped when the sign-in ends.
	picture *qrPicture
	cancel  context.CancelFunc
	// generation counts sign-ins, so a poller only ever changes its own.
	generation int
}

// qrPicture is a QR code as one SVG path over a square of size modules,
// the quiet zone included.
type qrPicture struct {
	Size int    `json:"size"`
	Path string `json:"path"`
}

// qrQuiet is the quiet zone around a code, in modules.
const qrQuiet = 4

func newQRPicture(text string) (*qrPicture, error) {
	code, err := qr.Encode(text, qr.M)
	if err != nil {
		return nil, err
	}
	var path strings.Builder
	for y := 0; y < code.Size; y++ {
		for x := 0; x < code.Size; {
			if !code.Black(x, y) {
				x++
				continue
			}
			start := x
			for x < code.Size && code.Black(x, y) {
				x++
			}
			run := strconv.Itoa(x - start)
			path.WriteString("M" + strconv.Itoa(start+qrQuiet) + " " + strconv.Itoa(y+qrQuiet) + "h" + run + "v1h-" + run + "z")
		}
	}
	return &qrPicture{Size: code.Size + 2*qrQuiet, Path: path.String()}, nil
}

func (q *bilibiliQR) active() bool {
	return q.state == bilisource.QRWaiting || q.state == bilisource.QRScanned
}

// payload is the sign-in as the console sees it.
func (q *bilibiliQR) payload() map[string]any {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := map[string]any{"state": nil, "error": nil, "expiresAt": nil, "qr": nil}
	if q.state != "" {
		out["state"] = q.state
	}
	if q.code != "" {
		out["error"] = q.code
	}
	if q.active() {
		out["expiresAt"], out["qr"] = sqlite.ISOTime(q.expiresAt), q.picture
	}
	return out
}

// finish ends sign-in generation at state and drops its code; a later
// sign-in is left alone.
func (q *bilibiliQR) finish(generation int, state, code string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if generation != q.generation || !q.active() {
		return
	}
	q.state, q.code, q.picture = state, code, nil
	if q.cancel != nil {
		q.cancel()
		q.cancel = nil
	}
}

// update records how sign-in generation stands while it is open.
func (q *bilibiliQR) update(generation int, state string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if generation == q.generation && q.active() {
		q.state = state
	}
}

// checkBilibiliLogin verifies the login a QR sign-in just stored, before the
// sign-in reports success, so the console shows the account's name and state
// at once. A failed check is left to the next dynamics read, which finds it
// due.
func (a *api) checkBilibiliLogin(ctx context.Context) {
	account := a.Bilibili.Options.Account
	state, err := a.Store.BilibiliState(ctx, account)
	if err != nil {
		return
	}
	if next, requests, _ := a.Bilibili.CheckLogin(ctx, state); requests > 0 {
		_ = a.Store.SaveBilibiliState(ctx, account, next)
	}
}

// bilibiliLogins answers 503 unless this process keeps a Bilibili login.
func (a *api) bilibiliLogins(w http.ResponseWriter) bool {
	if a.Bilibili == nil || a.Bilibili.Logins == nil {
		httpx.WriteCode(w, http.StatusServiceUnavailable, "bilibili_unavailable")
		return false
	}
	return true
}

func (a *api) writeBilibiliSource(w http.ResponseWriter, ctx context.Context) {
	status, err := a.bilibiliStatus(ctx)
	if err != nil {
		httpx.WriteCode(w, http.StatusServiceUnavailable, "database_unavailable")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, status)
}

// writeLoginStoreError reports a failed write of the stored login.
func writeLoginStoreError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, secretstore.ErrKeyRequired):
		httpx.WriteCode(w, http.StatusServiceUnavailable, "secrets_key_unavailable")
	case errors.Is(err, sqlite.ErrReadOnly):
		httpx.ReadOnly(w)
	case errors.Is(err, bilisource.ErrLoginChanged):
		httpx.WriteCode(w, http.StatusConflict, "bilibili_login_changed")
	default:
		httpx.WriteCode(w, http.StatusServiceUnavailable, "database_unavailable")
	}
}

// writeLoginRequestError reports a failed request to Bilibili: the shared
// backoff (never forced here) or the request's code.
func writeLoginRequestError(w http.ResponseWriter, err error, state bilisource.State) {
	code := bilisource.ErrorCode(err)
	if code == "bilibili_blocked" || bilisource.IsRiskControl(code) {
		httpx.WriteJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": "bilibili_blocked", "code": code, "blockedUntil": state.BlockedUntil.UTC().Format(time.RFC3339),
		})
		return
	}
	httpx.WriteJSON(w, http.StatusBadGateway, map[string]any{"error": code})
}

// importBilibiliLogin stores { cookie, refreshToken }: a private window's
// Cookie header and its localStorage ac_time_value. It sends no request;
// the login is unchecked until the next read.
func (a *api) importBilibiliLogin(w http.ResponseWriter, r *http.Request) {
	if !a.bilibiliLogins(w) {
		return
	}
	body, err := httpx.Body(w, r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	var cookie, token string
	if json.Unmarshal(body["cookie"], &cookie) != nil || json.Unmarshal(body["refreshToken"], &token) != nil || len(body) != 2 {
		httpx.WriteError(w, httpx.Invalid("send cookie and refreshToken as strings"))
		return
	}
	login, err := bilisource.ImportLogin(cookie, token, a.Now())
	if err != nil {
		if bilisource.ErrorCode(err) == "bilibili_login_token_invalid" {
			httpx.WriteError(w, httpx.Invalid("refreshToken is not a Bilibili refresh token"))
		} else {
			httpx.WriteError(w, httpx.Invalid("cookie needs SESSDATA and bili_jct"))
		}
		return
	}
	if err = a.Bilibili.Logins.Put(r.Context(), login); err != nil {
		writeLoginStoreError(w, err)
		return
	}
	a.writeBilibiliSource(w, r.Context())
}

// forgetBilibiliLogin drops the stored login. With ?signOut=1 it first ends
// the session at Bilibili, one request that respects the shared backoff; a
// login Bilibili already refuses is dropped as well, any other failure
// keeps it.
func (a *api) forgetBilibiliLogin(w http.ResponseWriter, r *http.Request) {
	if !a.bilibiliLogins(w) {
		return
	}
	ctx := r.Context()
	signOut := r.URL.Query().Get("signOut") == "1"
	login, version, err := a.Bilibili.Logins.Load(ctx)
	switch {
	case errors.Is(err, bilisource.ErrNoLogin):
		a.writeBilibiliSource(w, ctx)
		return
	case errors.Is(err, bilisource.ErrLoginUnreadable) && signOut:
		httpx.WriteCode(w, http.StatusConflict, "bilibili_login_unreadable")
		return
	case err != nil && !errors.Is(err, bilisource.ErrLoginUnreadable):
		httpx.WriteCode(w, http.StatusServiceUnavailable, "database_unavailable")
		return
	}
	if signOut {
		account := a.Bilibili.Options.Account
		state, stateErr := a.Store.BilibiliState(ctx, account)
		if stateErr != nil {
			httpx.WriteCode(w, http.StatusServiceUnavailable, "database_unavailable")
			return
		}
		if a.Now().Before(state.BlockedUntil) {
			writeLoginRequestError(w, &bilisource.Error{Code: "bilibili_blocked"}, state)
			return
		}
		next, signOutErr := a.Bilibili.SignOut(ctx, state, login)
		// A session that ended at Bilibili is forgotten even when its
		// bookkeeping could not be saved: the login no longer works.
		_ = a.Store.SaveBilibiliState(ctx, account, next)
		if signOutErr != nil {
			writeLoginRequestError(w, signOutErr, next)
			return
		}
	}
	if err = a.Bilibili.Logins.Forget(ctx, version); err != nil {
		writeLoginStoreError(w, err)
		return
	}
	a.writeBilibiliSource(w, ctx)
}

// bilibiliQRStatus is the current QR sign-in.
func (a *api) bilibiliQRStatus(w http.ResponseWriter, _ *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, a.qr.payload())
}

// startBilibiliQR opens a QR sign-in with the server's device identity, or
// answers the one still open. The server polls it every few seconds until
// it is signed in, expired or failed; a success stores the login like an
// import.
func (a *api) startBilibiliQR(w http.ResponseWriter, r *http.Request) {
	if !a.bilibiliLogins(w) {
		return
	}
	ctx := r.Context()
	a.qr.starting.Lock()
	defer a.qr.starting.Unlock()
	a.qr.mu.Lock()
	active := a.qr.active()
	a.qr.mu.Unlock()
	if active {
		httpx.WriteJSON(w, http.StatusOK, a.qr.payload())
		return
	}
	account := a.Bilibili.Options.Account
	state, err := a.Store.BilibiliState(ctx, account)
	if err != nil {
		httpx.WriteCode(w, http.StatusServiceUnavailable, "database_unavailable")
		return
	}
	if a.Now().Before(state.BlockedUntil) {
		writeLoginRequestError(w, &bilisource.Error{Code: "bilibili_blocked"}, state)
		return
	}
	session, next, requests, err := a.Bilibili.DeviceSession(ctx, state)
	var code bilisource.QRCode
	if err == nil {
		code, err = a.Bilibili.Client.QRGenerate(ctx, session)
		next = bilisource.Blocked(next, err, a.Now())
		requests++
	}
	if requests > 0 {
		if saveErr := a.Store.SaveBilibiliState(ctx, account, next); saveErr != nil && err == nil {
			httpx.WriteCode(w, http.StatusServiceUnavailable, "database_unavailable")
			return
		}
	}
	if err != nil {
		writeLoginRequestError(w, err, next)
		return
	}
	picture, err := newQRPicture(code.URL)
	if err != nil {
		httpx.WriteCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	pollCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), qrLifetime)
	a.qr.mu.Lock()
	if a.qr.cancel != nil {
		a.qr.cancel()
	}
	a.qr.generation++
	generation := a.qr.generation
	a.qr.state, a.qr.code, a.qr.picture, a.qr.cancel = bilisource.QRWaiting, "", picture, cancel
	a.qr.expiresAt = a.Now().Add(qrLifetime)
	a.qr.mu.Unlock()
	go a.pollBilibiliQR(pollCtx, generation, session, code.Key, qrPollInterval)
	httpx.WriteJSON(w, http.StatusOK, a.qr.payload())
}

// cancelBilibiliQR ends the open sign-in; its code stops working here.
func (a *api) cancelBilibiliQR(w http.ResponseWriter, _ *http.Request) {
	a.qr.mu.Lock()
	generation := a.qr.generation
	a.qr.mu.Unlock()
	a.qr.finish(generation, bilisource.QRExpired, "")
	httpx.WriteJSON(w, http.StatusOK, a.qr.payload())
}

// pollBilibiliQR polls one sign-in until it ends. A captcha answer starts
// the shared backoff.
func (a *api) pollBilibiliQR(ctx context.Context, generation int, session bilisource.Session, key string, every time.Duration) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			a.qr.finish(generation, bilisource.QRExpired, "")
			return
		case <-ticker.C:
		}
		status, login, err := a.Bilibili.Client.QRPoll(ctx, session, key, a.Now())
		if ctx.Err() != nil {
			continue
		}
		if err != nil {
			code := bilisource.ErrorCode(err)
			if bilisource.IsRiskControl(code) {
				account := a.Bilibili.Options.Account
				if state, stateErr := a.Store.BilibiliState(ctx, account); stateErr == nil {
					_ = a.Store.SaveBilibiliState(ctx, account, bilisource.Blocked(state, err, a.Now()))
				}
			}
			a.qr.finish(generation, bilisource.QRFailed, code)
			return
		}
		switch status {
		case bilisource.QRWaiting, bilisource.QRScanned:
			a.qr.update(generation, status)
		case bilisource.QRSignedIn:
			if err = a.Bilibili.Logins.Put(context.WithoutCancel(ctx), login); err != nil {
				a.qr.finish(generation, bilisource.QRFailed, "bilibili_login_save_failed")
				return
			}
			a.checkBilibiliLogin(context.WithoutCancel(ctx))
			a.qr.finish(generation, bilisource.QRSignedIn, "")
			return
		default:
			a.qr.finish(generation, status, "")
			return
		}
	}
}
