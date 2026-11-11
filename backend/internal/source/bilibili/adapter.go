package bilibili

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"time"
)

// State is the bookkeeping kept between reads of one account. Times are zero
// while unknown.
type State struct {
	Session                     Session
	SessionAt, LastAttemptAt    time.Time
	LastSuccessAt, BlockedUntil time.Time
	// Failures counts consecutive risk-control or rate-limit answers; each
	// doubles the wait before the next request.
	Failures  int
	LastError string
	// BlockedAt is when the current block was recorded and ClearedAt the
	// start of the latest read that succeeded, so a write can tell a block a
	// concurrent read stored after it from one its own success lifted.
	BlockedAt, ClearedAt time.Time
	// Dynamics is the dynamics listing's own clock and outcome; it shares the
	// captcha backoff above with the upload read.
	Dynamics DynamicsState
	// WBI caches the signature keys of the listing.
	WBI WBIKeys
	// Login is what the latest login check learned about the configured
	// cookie; it is zero until one ran.
	Login LoginState
}

// LoginState is the latest login check: when it ran and whether Bilibili
// accepted the cookie (nav isLogin).
type LoginState struct {
	CheckedAt time.Time
	SignedIn  bool
}

// loginCheckInterval is how often the configured cookie is verified.
const loginCheckInterval = 24 * time.Hour

// LoginStatus is how the console describes the login cookie: signed_in,
// expired, unchecked (configured, not verified yet) or not_configured.
func (a *Adapter) LoginStatus(state State) string {
	switch {
	case !a.Options.SignedIn():
		return "not_configured"
	case state.Login.CheckedAt.IsZero():
		return "unchecked"
	case state.Login.SignedIn:
		return "signed_in"
	}
	return "expired"
}

// DynamicsState is the bookkeeping of the dynamics listing.
type DynamicsState struct {
	LastAttemptAt, LastSuccessAt time.Time
	LastError                    string
}

// sessionTTL is how long an anonymous device identity is reused.
const sessionTTL = 7 * 24 * time.Hour

// Backoff after a captcha or rate-limit answer: 6 hours, doubled for each
// consecutive one, at most a day.
const (
	firstBlock = 6 * time.Hour
	maxBlock   = 24 * time.Hour
)

// DynamicsResult is what one dynamics read learned.
type DynamicsResult struct {
	Dynamics []Dynamic
	// Uploads are the video dynamics the listing showed, read as uploads.
	Uploads []Video
	State   State
	// Skipped is "login_required", "login_expired", "blocked" or
	// "interval" when no listing request was made; a login check may still
	// have been requested.
	Skipped   string
	Requested int
}

// Result is what one list read learned.
type Result struct {
	Videos []Video
	State  State
	// Skipped is "blocked" or "interval" when no request was made.
	Skipped   string
	Requested int
	// Listing is the method that gave the uploads. Fallback is the same
	// method when an earlier method of the read failed; ListError is the
	// code of that first failure then (bilibili_blocked when the block held
	// the web API methods back), and FallbackErrors the "<method>: <code>"
	// of each later method that failed too.
	Listing, Fallback, ListError string
	FallbackErrors               []string
	// blockCounted is set once a failure of this read started the backoff,
	// so a later fallback's captcha does not count it again.
	blockCounted bool
	// firstErr is the read's first failure, which it returns when no method
	// answers.
	firstErr error
}

type Adapter struct {
	Client  *Client
	Options Options
	Now     func() time.Time
}

func NewAdapter(c *Client, options Options) *Adapter {
	if c == nil {
		c = NewClient(nil)
	}
	return &Adapter{c, options, time.Now}
}

// Listing methods of the upload read, by the stable IDs the timed chain
// names them with.
const (
	// MethodList is the account's upload list on api.bilibili.com.
	MethodList = "list"
	// MethodDynamics is the signed-in dynamics listing on api.bilibili.com,
	// read for its video dynamics; it needs the login cookie.
	MethodDynamics = "dynamics"
	// MethodApp is the official client's space listing on app.bilibili.com;
	// it needs the client's app key pair.
	MethodApp = "app"
)

// DefaultMethods is the upload read's default order.
var DefaultMethods = []string{MethodList, MethodDynamics, MethodApp}

// FetchRecent reads the newest uploads through DefaultMethods.
func (a *Adapter) FetchRecent(ctx context.Context, state State) (Result, error) {
	return a.FetchRecentWith(ctx, state, DefaultMethods)
}

// FetchRecentWith reads the account's newest uploads at most once per
// MinInterval through methods, the timed chain's enabled methods in order:
// the first that answers gives the uploads. A method that lacks its
// credential (the login cookie, or the app key pair) is passed over without
// a request. A failure is returned together with the state to persist, which
// records the attempt and any backoff. The shared block holds back every
// api.bilibili.com request, so while it lasts only the official client's
// listing, another service, is read when the chain enables it; a success
// there leaves the block in force. Only an api.bilibili.com success, the
// list's or the dynamics', clears it.
func (a *Adapter) FetchRecentWith(ctx context.Context, state State, methods []string) (Result, error) {
	if len(methods) == 0 {
		methods = DefaultMethods
	}
	now := a.Now()
	result := Result{Videos: []Video{}, State: state}
	due := state.LastAttemptAt.IsZero() || now.Sub(state.LastAttemptAt) >= a.Options.MinInterval
	held := now.Before(state.BlockedUntil)
	app := slices.Contains(methods, MethodApp) && a.Options.App.Valid()
	switch {
	case held && (!due || !app):
		result.Skipped = "blocked"
		return result, nil
	case !due:
		result.Skipped = "interval"
		return result, nil
	}
	result.State.LastAttemptAt = now
	if held {
		err := &Error{"bilibili_blocked"}
		result.firstErr, result.ListError, result.blockCounted = err, err.Code, true
		if videos, ok := a.uploadsFromApp(ctx, &result, now); ok {
			result.answered(MethodApp, videos)
			result.State.LastSuccessAt = now
			return result, nil
		}
		return result, err
	}
	var session Session
	var sessionErr error
	opened := false
	// The web API methods share one device identity, made at most once.
	open := func(method string) bool {
		if !opened {
			opened = true
			session, sessionErr = a.session(ctx, &result.State, &result.Requested, now)
			if sessionErr != nil {
				result.methodFailed(method, sessionErr, now)
			}
		}
		return sessionErr == nil
	}
	for _, method := range methods {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		switch method {
		case MethodList:
			if !open(method) {
				continue
			}
			result.Requested++
			videos, err := a.Client.Uploads(ctx, a.Options.Account, a.Options.PageSize, session)
			if err != nil {
				result.methodFailed(method, err, now)
				continue
			}
			result.answered(method, videos)
			result.State = succeeded(result.State, now)
			return result, nil
		case MethodDynamics:
			if !a.loginUsable(result.State, now) || !open(method) {
				continue
			}
			if videos, ok := a.uploadsFromDynamics(ctx, &result, session, now); ok {
				result.answered(method, videos)
				result.State = succeeded(result.State, now)
				return result, nil
			}
		case MethodApp:
			if videos, ok := a.uploadsFromApp(ctx, &result, now); ok {
				result.answered(method, videos)
				result.State.LastSuccessAt = now
				return result, nil
			}
		}
	}
	if ctx.Err() != nil {
		return result, ctx.Err()
	}
	if result.firstErr == nil {
		// No enabled method could make a request: the chain's methods all
		// lack their credentials.
		result.methodFailed("", &Error{"bilibili_no_method"}, now)
	}
	return result, result.firstErr
}

// answered records the method that gave the uploads; it is a fallback when
// an earlier method of the read failed.
func (r *Result) answered(method string, videos []Video) {
	r.Videos, r.Listing = videos, method
	if r.ListError != "" {
		r.Fallback = method
	}
}

// methodFailed records one method's failure. The read's first failure sets
// the account's last error and starts the backoff for a captcha; a later one
// is a fallback error and starts the backoff only when no earlier failure
// of the read did.
func (r *Result) methodFailed(method string, err error, now time.Time) {
	if r.firstErr == nil {
		code := ErrorCode(err)
		r.firstErr, r.ListError = err, code
		r.State, r.blockCounted = failed(r.State, err, now), IsRiskControl(code)
		return
	}
	r.fallbackFailed(method, err, now)
}

// loginUsable reports whether the signed-in dynamics listing may be tried:
// a login cookie is configured and the last check did not find it expired,
// or a check is due.
func (a *Adapter) loginUsable(state State, now time.Time) bool {
	loginDue := state.Login.CheckedAt.IsZero() || now.Sub(state.Login.CheckedAt) >= loginCheckInterval
	return a.Options.SignedIn() && (loginDue || state.Login.SignedIn)
}

// uploadsFromDynamics reads the uploads from the account's signed-in
// dynamics listing after the upload list failed: one listing request, plus
// the login check and WBI keys when they are due, made at once whatever the
// list's failure, since Bilibili accepts a signed-in listing more readily
// than the anonymous list. It needs the login cookie and a login the last
// check did not find expired, and leaves the dynamics read's own clock
// alone. A captcha it meets starts the backoff unless the list's failure
// already did.
func (a *Adapter) uploadsFromDynamics(ctx context.Context, result *Result, session Session, now time.Time) ([]Video, bool) {
	state := &result.State
	loginDue := state.Login.CheckedAt.IsZero() || now.Sub(state.Login.CheckedAt) >= loginCheckInterval
	if !a.loginUsable(*state, now) {
		return nil, false
	}
	fail := func(err error) ([]Video, bool) {
		code := ErrorCode(err)
		switch code {
		case "bilibili_risk_control_voucher", "bilibili_risk_control_352":
			state.WBI = WBIKeys{}
		case "bilibili_login_expired":
			state.Login = LoginState{CheckedAt: now}
		}
		result.methodFailed(MethodDynamics, err, now)
		return nil, false
	}
	if loginDue || !state.WBI.fresh(now) {
		result.Requested++
		check := session
		a.Options.signIn(&check)
		keys, signedIn, err := a.Client.NavKeys(ctx, check, now)
		if err != nil {
			return fail(err)
		}
		state.WBI, state.Login = keys, LoginState{CheckedAt: now, SignedIn: signedIn}
		if !signedIn {
			result.methodFailed(MethodDynamics, &Error{"bilibili_login_expired"}, now)
			return nil, false
		}
	}
	a.Options.signIn(&session)
	result.Requested++
	page, err := a.Client.SpaceDynamics(ctx, a.Options.Account, session, state.WBI, now)
	if err != nil {
		return fail(err)
	}
	return page.Uploads, true
}

// FetchVideo reads one upload for an operator, whatever the list interval.
// While a block lasts it answers bilibili_blocked without a request, unless
// the operator forces this one request: a success then clears the backoff
// and a failure extends it as usual. Only uploads of the configured account
// are accepted.
func (a *Adapter) FetchVideo(ctx context.Context, state State, bvid string, force bool) (Video, State, error) {
	now := a.Now()
	if now.Before(state.BlockedUntil) && !force {
		return Video{}, state, &Error{"bilibili_blocked"}
	}
	requests := 0
	session, err := a.session(ctx, &state, &requests, now)
	var v Video
	if err == nil {
		v, err = a.Client.Video(ctx, bvid, session)
	}
	if err != nil {
		return Video{}, failed(state, err, now), err
	}
	if strconv.FormatInt(v.OwnerMid, 10) != a.Options.Account {
		return Video{}, succeeded(state, now), &Error{"bilibili_other_account"}
	}
	return v, succeeded(state, now), nil
}

// FetchDynamics reads the first page of the account's dynamics at most once
// per MinInterval on its own clock, never while the shared block lasts, and
// only with a login cookie Bilibili accepts: Bilibili answers the anonymous
// listing only with fingerprint parameters, so none is attempted. The login
// is verified through nav isLogin at most once a day, and an expired cookie
// holds the listing back until the next check. It makes at most one listing
// request, plus the device identity, the ticket and nav when they are due.
func (a *Adapter) FetchDynamics(ctx context.Context, state State) (DynamicsResult, error) {
	now := a.Now()
	result := DynamicsResult{Dynamics: []Dynamic{}, State: state}
	loginDue := state.Login.CheckedAt.IsZero() || now.Sub(state.Login.CheckedAt) >= loginCheckInterval
	switch {
	case !a.Options.SignedIn():
		result.Skipped = "login_required"
		return result, nil
	case now.Before(state.BlockedUntil):
		result.Skipped = "blocked"
		return result, nil
	case !state.Dynamics.LastAttemptAt.IsZero() && now.Sub(state.Dynamics.LastAttemptAt) < a.Options.MinInterval:
		result.Skipped = "interval"
		return result, nil
	case !loginDue && !state.Login.SignedIn:
		result.Skipped = "login_expired"
		return result, nil
	}
	result.State.Dynamics.LastAttemptAt = now
	session, err := a.session(ctx, &result.State, &result.Requested, now)
	if err == nil && (loginDue || !result.State.WBI.fresh(now)) {
		// The check sends the cookie even when the last one found it
		// expired, so a renewed cookie is noticed.
		result.Requested++
		check := session
		a.Options.signIn(&check)
		var keys WBIKeys
		var signedIn bool
		if keys, signedIn, err = a.Client.NavKeys(ctx, check, now); err == nil {
			result.State.WBI = keys
			result.State.Login = LoginState{CheckedAt: now, SignedIn: signedIn}
			if !signedIn {
				result.State.Dynamics.LastError = "bilibili_login_expired"
				result.Skipped = "login_expired"
				return result, nil
			}
			a.Options.signIn(&session)
		}
	}
	var page SpacePage
	if err == nil {
		result.Requested++
		page, err = a.Client.SpaceDynamics(ctx, a.Options.Account, session, result.State.WBI, now)
	}
	if err != nil {
		result.State = failedDynamics(result.State, err, now)
		return result, err
	}
	result.Dynamics, result.Uploads = page.Dynamics, page.Uploads
	result.State = succeededDynamics(result.State, now)
	return result, nil
}

// FetchUploadsPage reads one page of the account's uploads for an operator's
// fetch, whatever the list interval; page 1 is the newest. While a block
// lasts it answers bilibili_blocked without a request, and a fetch is never
// forced. A success clears the backoff and records the read's success, a
// failure its code and any backoff; the list read's attempt clock stays with
// the timer. It also reports the requests it made, the device identity and
// ticket included.
func (a *Adapter) FetchUploadsPage(ctx context.Context, state State, page int) (UploadsPage, State, int, error) {
	now := a.Now()
	if now.Before(state.BlockedUntil) {
		return UploadsPage{}, state, 0, &Error{"bilibili_blocked"}
	}
	requests := 0
	session, err := a.session(ctx, &state, &requests, now)
	var out UploadsPage
	if err == nil {
		requests++
		out, err = a.Client.UploadsPage(ctx, a.Options.Account, a.Options.PageSize, page, session)
	}
	if err != nil {
		return UploadsPage{}, failed(state, err, now), requests, err
	}
	return out, succeeded(state, now), requests, nil
}

// FetchDynamicsPage reads the page of the account's dynamics that follows
// offset (empty for the newest) for an operator's fetch, whatever the
// listing interval. Like FetchDynamics it needs a login cookie Bilibili
// accepts, checks the login and WBI keys when they are due, and never asks
// while the shared block lasts; each refusal is a code rather than a skip
// (bilibili_login_required, bilibili_login_expired, bilibili_blocked), and a
// fetch is never forced. A success clears the backoff and records the
// listing's success, a failure its code and any backoff; the listing's
// attempt clock stays with the timer.
func (a *Adapter) FetchDynamicsPage(ctx context.Context, state State, offset string) (SpacePage, State, int, error) {
	now := a.Now()
	loginDue := state.Login.CheckedAt.IsZero() || now.Sub(state.Login.CheckedAt) >= loginCheckInterval
	switch {
	case !a.Options.SignedIn():
		return SpacePage{}, state, 0, &Error{"bilibili_login_required"}
	case now.Before(state.BlockedUntil):
		return SpacePage{}, state, 0, &Error{"bilibili_blocked"}
	case !loginDue && !state.Login.SignedIn:
		return SpacePage{}, state, 0, &Error{"bilibili_login_expired"}
	}
	requests := 0
	session, err := a.session(ctx, &state, &requests, now)
	if err == nil && (loginDue || !state.WBI.fresh(now)) {
		requests++
		check := session
		a.Options.signIn(&check)
		var keys WBIKeys
		var signedIn bool
		if keys, signedIn, err = a.Client.NavKeys(ctx, check, now); err == nil {
			state.WBI = keys
			state.Login = LoginState{CheckedAt: now, SignedIn: signedIn}
			if !signedIn {
				state.Dynamics.LastError = "bilibili_login_expired"
				return SpacePage{}, state, requests, &Error{"bilibili_login_expired"}
			}
			a.Options.signIn(&session)
		}
	}
	var page SpacePage
	if err == nil {
		requests++
		page, err = a.Client.SpaceDynamicsPage(ctx, a.Options.Account, offset, session, state.WBI, now)
	}
	if err != nil {
		return SpacePage{}, failedDynamics(state, err, now), requests, err
	}
	return page, succeededDynamics(state, now), requests, nil
}

// FetchDynamic reads one dynamic for an operator from its opus page, which
// needs no login, whatever the listing interval. A block and force work as
// in FetchVideo. Only post-type dynamics of the configured account are
// accepted.
func (a *Adapter) FetchDynamic(ctx context.Context, state State, id string, force bool) (Dynamic, State, error) {
	now := a.Now()
	if now.Before(state.BlockedUntil) && !force {
		return Dynamic{}, state, &Error{"bilibili_blocked"}
	}
	requests := 0
	session, err := a.session(ctx, &state, &requests, now)
	var d Dynamic
	if err == nil {
		d, err = a.Client.OpusPage(ctx, id, session)
	}
	if err != nil {
		return Dynamic{}, blocked(state, err, now), err
	}
	state.Failures, state.BlockedUntil, state.ClearedAt = 0, time.Time{}, now
	if strconv.FormatInt(d.OwnerMid, 10) != a.Options.Account {
		return Dynamic{}, state, &Error{"bilibili_other_account"}
	}
	return d, state, nil
}

// FetchCover reads the cover address of any upload for the Works page,
// whatever its account, never while a block lasts. As in FetchDynamic, a
// captcha or rate-limit answer starts the shared backoff and a success
// clears it; the list read's interval, last success and last error stay as
// they were.
func (a *Adapter) FetchCover(ctx context.Context, state State, bvid string) (string, State, error) {
	now := a.Now()
	if now.Before(state.BlockedUntil) {
		return "", state, &Error{"bilibili_blocked"}
	}
	requests := 0
	session, err := a.session(ctx, &state, &requests, now)
	var v Video
	if err == nil {
		v, err = a.Client.Video(ctx, bvid, session)
	}
	if err == nil && v.ThumbnailURL == "" {
		err = &Error{"bilibili_invalid_response"}
	}
	if err != nil {
		return "", blocked(state, err, now), err
	}
	state.Failures, state.BlockedUntil, state.ClearedAt = 0, time.Time{}, now
	return v.ThumbnailURL, state, nil
}

// session reuses a recent device identity or requests a new one (its b_nut
// is that moment), renews its bili_ticket when it is missing or about to
// expire, and adds the configured login cookie for this request only, unless
// the latest login check found it expired. A failed ticket request is not a
// failure: the read goes on without a ticket. A ticket also carries the
// current WBI keys.
func (a *Adapter) session(ctx context.Context, state *State, requests *int, now time.Time) (Session, error) {
	if !state.Session.valid() || now.Sub(state.SessionAt) >= sessionTTL {
		*requests++
		session, err := a.Client.NewSession(ctx)
		if err != nil {
			return Session{}, err
		}
		session.Nut = now.Unix()
		state.Session, state.SessionAt = session, now
	}
	if state.Session.ticketDue(now) {
		*requests++
		if session, keys, err := a.Client.WebTicket(ctx, state.Session, now); err == nil {
			state.Session = session
			if keys.valid() {
				state.WBI = keys
			}
		} else if ctx.Err() != nil {
			return Session{}, ctx.Err()
		}
	}
	session := state.Session
	if a.Options.SignedIn() && (state.Login.CheckedAt.IsZero() || state.Login.SignedIn) {
		a.Options.signIn(&session)
	}
	return session, nil
}

// uploadsFromApp reads the uploads from the official client's space listing
// on app.bilibili.com after the upload list and any dynamics fallback
// failed, or while the block holds them back: one anonymous request signed
// with the configured app key pair, a different service from the web API
// that meets the captcha. Without a pair it is not tried. Like the other
// fallback it starts the backoff only when no earlier failure of the read
// did, and never extends a block in force.
func (a *Adapter) uploadsFromApp(ctx context.Context, result *Result, now time.Time) ([]Video, bool) {
	if !a.Options.App.Valid() {
		return nil, false
	}
	result.Requested++
	videos, err := a.Client.AppUploads(ctx, a.Options.Account, a.Options.PageSize, a.Options.App, now)
	if err != nil {
		result.methodFailed(MethodApp, err, now)
		return nil, false
	}
	return videos, true
}

// fallbackFailed records a fallback's failure and starts the backoff for a
// captcha unless an earlier failure of the read already did.
func (r *Result) fallbackFailed(name string, err error, now time.Time) {
	code := ErrorCode(err)
	r.FallbackErrors = append(r.FallbackErrors, name+": "+code)
	if IsRiskControl(code) && !r.blockCounted {
		r.State, r.blockCounted = blocked(r.State, err, now), true
	}
}

// ErrorCode is the fixed code of a source error.
func ErrorCode(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return "bilibili_unavailable"
}

func failed(state State, err error, now time.Time) State {
	state.LastError = ErrorCode(err)
	return blocked(state, err, now)
}

// blocked starts or extends the shared backoff after a captcha or a rate
// limit; any other failure leaves it alone.
func blocked(state State, err error, now time.Time) State {
	if IsRiskControl(ErrorCode(err)) {
		state.BlockedAt = now
	}
	state.Failures, state.BlockedUntil = backoff(state.Failures, state.BlockedUntil, ErrorCode(err), now)
	return state
}

// backoff is the wait after a captcha or rate-limit answer: 6 hours, doubled
// for each consecutive one, at most a day.
func backoff(failures int, until time.Time, code string, now time.Time) (int, time.Time) {
	if !IsRiskControl(code) {
		return failures, until
	}
	failures++
	wait := firstBlock << min(failures-1, 4)
	return failures, now.Add(min(wait, maxBlock))
}

func failedDynamics(state State, err error, now time.Time) State {
	state.Dynamics.LastError = ErrorCode(err)
	// A signature answer may mean the keys rotated: read them again next
	// time rather than reuse them after the block.
	if code := state.Dynamics.LastError; code == "bilibili_risk_control_voucher" || code == "bilibili_risk_control_352" {
		state.WBI = WBIKeys{}
	}
	if state.Dynamics.LastError == "bilibili_login_expired" {
		state.Login = LoginState{CheckedAt: now}
	}
	return blocked(state, err, now)
}

func succeededDynamics(state State, now time.Time) State {
	state.Failures, state.BlockedUntil, state.ClearedAt = 0, time.Time{}, now
	state.Dynamics.LastError, state.Dynamics.LastSuccessAt = "", now
	return state
}

func succeeded(state State, now time.Time) State {
	state.Failures, state.LastError, state.ClearedAt = 0, "", now
	state.LastSuccessAt, state.BlockedUntil = now, time.Time{}
	return state
}

// LiveState is the bookkeeping of the live host, with its own backoff: the
// live API is a different service from the one that shows the captcha.
type LiveState struct {
	Room                     Room
	CheckedAt, LastAttemptAt time.Time
	BlockedUntil             time.Time
	Failures                 int
	LastError                string
}

// CheckLive asks about the account's live room once, never while the live
// host's block lasts. The caller decides when a check is due.
func (a *Adapter) CheckLive(ctx context.Context, state LiveState) (LiveState, bool, error) {
	now := a.Now()
	if now.Before(state.BlockedUntil) {
		return state, false, nil
	}
	state.LastAttemptAt = now
	room, err := a.Client.LiveStatus(ctx, a.Options.Account)
	if err != nil {
		state.LastError = ErrorCode(err)
		state.Failures, state.BlockedUntil = backoff(state.Failures, state.BlockedUntil, state.LastError, now)
		return state, true, err
	}
	state.Room, state.CheckedAt = room, now
	state.Failures, state.BlockedUntil, state.LastError = 0, time.Time{}, ""
	return state, true, nil
}
