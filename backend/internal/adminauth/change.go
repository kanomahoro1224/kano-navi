package adminauth

import (
	"crypto/subtle"
	"errors"
	"net/http"
	"time"
)

// Failures of an operator change to the one-time code. Like a failed login,
// neither names the factor that was wrong.
var (
	ErrInvalidCredentials = errors.New("invalid_credentials")
	ErrTooManyAttempts    = errors.New("too_many_attempts")
)

// limitedLocked reports an address that has used up its failed logins.
func (a *Auth) limitedLocked(address string, now time.Time) bool {
	current := a.attempts[address]
	return current.count >= 5 && current.resetAt.After(now)
}

// verifyLocked compares the password and, when withCode is set and a code is
// in use, a code of the secret in use. A failure counts toward the address's
// limit, and a right password with a wrong code toward the shared code
// limit, which once reached fails every check as a wrong password does. A
// success clears the address's failures and spends the code's step, which it
// returns (0 when no code was checked).
func (a *Auth) verifyLocked(address, password, code string, withCode bool, now time.Time) (int64, error) {
	actual := []byte(password)
	expected := []byte(a.cfg.Password)
	matches := len(actual) == len(expected) && subtle.ConstantTimeCompare(actual, expected) == 1
	step, codeMatches := int64(0), true
	locked := false
	if withCode && a.requiresCodeLocked() {
		step, codeMatches = matchTOTP(a.secretLocked(), code, now)
		// A code is accepted once: a later check needs a later step.
		codeMatches = codeMatches && step > a.lastStep
		// The shared limit fails every check as a wrong password does, so
		// the lockout never tells which password is right.
		locked = a.codeFailures.count >= codeFailureLimit
	}
	if !matches || !codeMatches || locked {
		current := a.attempts[address]
		if current.resetAt.IsZero() || !current.resetAt.After(now) {
			current = attempt{resetAt: now.Add(5 * time.Minute)}
		}
		current.count++
		a.attempts[address] = current
		// Only the right password with a wrong code spends the shared
		// allowance; a wrong password is limited per address alone.
		if matches && !locked && withCode && a.requiresCodeLocked() {
			if a.codeFailures.resetAt.IsZero() {
				a.codeFailures.resetAt = now.Add(5 * time.Minute)
			}
			a.codeFailures.count++
		}
		return 0, ErrInvalidCredentials
	}
	if step > 0 {
		a.lastStep = step
	}
	delete(a.attempts, address)
	return step, nil
}

// VerifyChange checks an operator change to the one-time code: the admin
// password and, when withCode is set and a code is in use, a code of the
// secret in use, which is then spent. Failures count as failed logins do.
// Development mode, which asks for neither at login, checks nothing.
func (a *Auth) VerifyChange(r *http.Request, password, code string, withCode bool) error {
	if !a.RequiresPassword() {
		return nil
	}
	now := a.cfg.Now()
	address := remoteAddress(r)
	a.mu.Lock()
	defer a.mu.Unlock()
	a.purgeLocked(now)
	if a.limitedLocked(address, now) {
		return ErrTooManyAttempts
	}
	_, err := a.verifyLocked(address, password, code, withCode, now)
	return err
}

// EnvironmentCode reports whether ADMIN_TOTP_SECRET provides the code, which
// the console then cannot change.
func (a *Auth) EnvironmentCode() bool { return len(a.cfg.TOTPSecret) > 0 }

// SetStoredSecret binds secret, the console's, or clears it with nil. Unless
// ADMIN_TOTP_SECRET is in use, the change applies to the next login at once:
// the spent step becomes spent (the step of the code that confirmed the new
// secret, or 0) and the shared wrong-code count starts again.
func (a *Auth) SetStoredSecret(secret []byte, spent int64) error {
	if len(secret) > 0 && len(secret) < totpMinBytes {
		return ErrTOTPSecret
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.stored = append([]byte(nil), secret...)
	if len(a.cfg.TOTPSecret) == 0 {
		a.lastStep, a.codeFailures = spent, attempt{}
	}
	return nil
}

// MatchCode returns the step of code when it is a code of secret within the
// clock-skew window.
func MatchCode(secret []byte, code string, now time.Time) (int64, bool) {
	return matchTOTP(secret, code, now)
}

// EndOtherSessions signs out every admin session but the request's.
func (a *Auth) EndOtherSessions(r *http.Request) {
	keep := cookieValue(r, CookieName)
	a.mu.Lock()
	defer a.mu.Unlock()
	for token := range a.sessions {
		if token != keep {
			delete(a.sessions, token)
		}
	}
}
