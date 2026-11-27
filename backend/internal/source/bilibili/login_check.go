package bilibili

import "context"

// CheckLogin verifies the read's login through nav at once, whatever the
// daily check, and stores the WBI keys nav returns: the console's QR sign-in
// calls it right after storing a login, so the account shows its name and
// state before any dynamics read. Without a login, or while the shared block
// lasts, it makes no request; a captcha or rate-limit answer starts the
// backoff as a read's does. It also reports the requests it made.
func (a *Adapter) CheckLogin(ctx context.Context, state State) (State, int, error) {
	a = a.resolved(ctx)
	now := a.Now()
	switch {
	case !a.Options.SignedIn():
		return state, 0, &Error{"bilibili_login_required"}
	case now.Before(state.BlockedUntil):
		return state, 0, &Error{"bilibili_blocked"}
	}
	requests := 0
	session, err := a.session(ctx, &state, &requests, now)
	if err == nil {
		requests++
		a.Options.signIn(&session)
		var keys WBIKeys
		var nav NavLogin
		if keys, nav, err = a.Client.NavKeys(ctx, session, now); err == nil {
			state.WBI, state.Login = keys, a.checked(now, nav)
			return state, requests, nil
		}
	}
	if ErrorCode(err) == "bilibili_login_expired" {
		state.Login = a.checked(now, NavLogin{})
	}
	return blocked(state, err, now), requests, err
}
