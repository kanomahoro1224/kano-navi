package bilibili

import (
	"context"
	"errors"
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
	State    State
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

// FetchRecent reads the account's newest uploads at most once per
// MinInterval and never while a block lasts. A failure is returned together
// with the state to persist, which records the attempt and any backoff.
func (a *Adapter) FetchRecent(ctx context.Context, state State) (Result, error) {
	now := a.Now()
	result := Result{Videos: []Video{}, State: state}
	switch {
	case now.Before(state.BlockedUntil):
		result.Skipped = "blocked"
		return result, nil
	case !state.LastAttemptAt.IsZero() && now.Sub(state.LastAttemptAt) < a.Options.MinInterval:
		result.Skipped = "interval"
		return result, nil
	}
	result.State.LastAttemptAt = now
	session, err := a.session(ctx, &result.State, &result.Requested, now)
	if err == nil {
		result.Requested++
		result.Videos, err = a.Client.Uploads(ctx, a.Options.Account, a.Options.PageSize, session)
	}
	if err != nil {
		result.State = failed(result.State, err, now)
		result.Videos = []Video{}
		return result, err
	}
	result.State = succeeded(result.State, now)
	return result, nil
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
		check.Login = a.Options.SESSDATA
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
			session.Login = a.Options.SESSDATA
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
	result.Dynamics = page.Dynamics
	result.State = succeededDynamics(result.State, now)
	return result, nil
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
		session.Login = a.Options.SESSDATA
	}
	return session, nil
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
