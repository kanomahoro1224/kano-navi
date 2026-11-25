package bilibili

import (
	"context"
	"errors"
	"time"
)

// RefreshState is the stored login's latest refresh check. Of names the
// login it checked (StoredLogin.key), so a new login starts unchecked.
type RefreshState struct {
	Of        string
	CheckedAt time.Time
	// LastError is the check's failure code, empty after a check that
	// succeeded.
	LastError string
}

// refreshCheckInterval is how often the stored login is checked for a
// refresh.
const refreshCheckInterval = 24 * time.Hour

// Maintenance outcomes.
const (
	// RefreshCurrent: Bilibili asked for no refresh.
	RefreshCurrent = "current"
	// RefreshRefreshed: the login was refreshed, stored and confirmed.
	RefreshRefreshed = "refreshed"
	// RefreshConfirmed: a confirm a failed check left pending went through,
	// and no further refresh was asked.
	RefreshConfirmed = "confirmed"
	// RefreshSuperseded: another write (an import, a forget or another
	// process's refresh) replaced the login during the check, which then
	// left it alone.
	RefreshSuperseded = "superseded"
	// RefreshFailed: the check stopped at LastError.
	RefreshFailed = "failed"
)

// RefreshResult is what one maintenance of the stored login did.
type RefreshResult struct {
	State State
	// Outcome is empty when nothing was due, otherwise one of the Refresh
	// outcomes.
	Outcome   string
	Requested int
}

// MaintainLogin keeps the stored login alive, once a day and never while
// the shared backoff holds. A confirm that an earlier check left pending
// goes first; refused again, it is dropped. Then cookie/info says whether Bilibili wants a refresh; when
// it does, the correspond page and cookie/refresh trade the refresh token
// for new cookies. The new login is stored, with the replaced token as the
// pending confirm, before the confirm invalidates the old cookie: until the
// confirm the old cookie keeps working, so a failed write never loses the
// login, and a failed confirm is sent again by the next check. The
// environment's login is never refreshed.
func (a *Adapter) MaintainLogin(ctx context.Context, state State) (RefreshResult, error) {
	result := RefreshResult{State: state}
	login, version, err := a.Logins.Load(ctx)
	if err != nil {
		return result, nil
	}
	now := a.Now()
	key := login.key()
	last := state.Refresh
	if last.Of != key {
		last = RefreshState{}
	}
	if (!last.CheckedAt.IsZero() && now.Sub(last.CheckedAt) < refreshCheckInterval) || now.Before(state.BlockedUntil) {
		return result, nil
	}
	result.State.Refresh = RefreshState{Of: key, CheckedAt: now}
	fail := func(err error) (RefreshResult, error) {
		code := ErrorCode(err)
		result.Outcome, result.State.Refresh.LastError = RefreshFailed, code
		if code == "bilibili_login_expired" {
			view := a.resolved(ctx)
			result.State.Login = view.checked(now, NavLogin{})
		}
		result.State = blocked(result.State, err, now)
		return result, err
	}
	superseded := func() (RefreshResult, error) {
		result.Outcome, result.State.Refresh = RefreshSuperseded, state.Refresh
		return result, nil
	}
	session, err := a.session(ctx, &result.State, &result.Requested, now)
	if err != nil {
		return fail(err)
	}
	session.Login, session.Account = "", nil
	if login.PendingConfirm != "" {
		result.Requested++
		confirmErr := a.Client.ConfirmRefresh(ctx, login.signed(session), login.PendingConfirm)
		if confirmErr != nil && ErrorCode(confirmErr) != "bilibili_login_confirm_failed" {
			return fail(confirmErr)
		}
		// A confirm Bilibili refuses again is dropped: a token it already
		// took (its answer lost, or the write after it failed) is refused
		// every time, and the new login works without the confirm.
		login.PendingConfirm = ""
		if version, err = a.Logins.Replace(ctx, version, login); err != nil {
			if errors.Is(err, ErrLoginChanged) {
				return superseded()
			}
			return fail(&Error{"bilibili_login_save_failed"})
		}
		if confirmErr == nil {
			result.Outcome = RefreshConfirmed
		}
	}
	result.Requested++
	due, timestamp, err := a.Client.CookieInfo(ctx, login.signed(session))
	if err != nil {
		return fail(err)
	}
	if !due {
		if result.Outcome == "" {
			result.Outcome = RefreshCurrent
		}
		return result, nil
	}
	path, err := newCorrespondPath(timestamp)
	if err != nil {
		return fail(err)
	}
	result.Requested++
	csrf, err := a.Client.RefreshCSRF(ctx, login.signed(session), path)
	if err != nil {
		return fail(err)
	}
	result.Requested++
	sessdata, cookies, token, err := a.Client.RefreshCookie(ctx, login.signed(session), csrf, login.RefreshToken)
	if err != nil {
		return fail(err)
	}
	next := login
	next.SESSDATA, next.Cookies = sessdata, LoginCookies{}
	for name, value := range login.Cookies {
		next.Cookies[name] = value
	}
	for name, value := range cookies {
		next.Cookies[name] = value
	}
	next.RefreshToken, next.PendingConfirm, next.RefreshedAt = token, login.RefreshToken, now.UTC()
	if version, err = a.Logins.Replace(ctx, version, next); err != nil {
		if errors.Is(err, ErrLoginChanged) {
			return superseded()
		}
		return fail(&Error{"bilibili_login_save_failed"})
	}
	result.Outcome = RefreshRefreshed
	result.Requested++
	if err = a.Client.ConfirmRefresh(ctx, next.signed(session), next.PendingConfirm); err != nil {
		return fail(err)
	}
	next.PendingConfirm = ""
	if _, err = a.Logins.Replace(ctx, version, next); err != nil && !errors.Is(err, ErrLoginChanged) {
		// The confirm went through; the next check sends it once more.
		return fail(&Error{"bilibili_login_save_failed"})
	}
	return result, nil
}

// Blocked starts or extends the shared backoff when err is a captcha or a
// rate limit, as a read's failure does; any other error leaves state alone.
func Blocked(state State, err error, now time.Time) State {
	if err == nil {
		return state
	}
	return blocked(state, err, now)
}

// DeviceSession is the anonymous device identity a QR sign-in uses, made or
// renewed as a read would. It reports the state to store and the requests
// it made.
func (a *Adapter) DeviceSession(ctx context.Context, state State) (Session, State, int, error) {
	requests := 0
	view := *a
	view.Options.SESSDATA, view.Options.Cookies = "", nil
	session, err := view.session(ctx, &state, &requests, a.Now())
	if err != nil {
		return Session{}, blocked(state, err, a.Now()), requests, err
	}
	session.Login, session.Account = "", nil
	return session, state, requests, nil
}

// SignOut ends the stored login's session at Bilibili with the device
// identity of state. A login Bilibili already refuses counts as signed out.
func (a *Adapter) SignOut(ctx context.Context, state State, login StoredLogin) (State, error) {
	session, next, _, err := a.DeviceSession(ctx, state)
	if err != nil {
		return next, err
	}
	err = a.Client.SignOut(ctx, login.signed(session))
	if ErrorCode(err) == "bilibili_login_expired" {
		err = nil
	}
	return Blocked(next, err, a.Now()), err
}
