// Package totpadmin binds the admin one-time code from /admin: an operator
// scans a new secret's QR code, confirms it with the password and a code, and
// the secret is sealed with LLM_SECRETS_KEY in app_settings and used by the
// next login without a restart. ADMIN_TOTP_SECRET overrides the stored
// secret while it is set, and the console cannot change it then.
package totpadmin

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/yexca/kano-navi/backend/internal/adminauth"
	"github.com/yexca/kano-navi/backend/internal/httpx"
	"github.com/yexca/kano-navi/backend/internal/qrsvg"
	"github.com/yexca/kano-navi/backend/internal/secretstore"
	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
)

// Path is the module's route below /api/admin.
const Path = "/security/totp"

// setting is the app_settings row of the sealed secret.
const setting = "admin_totp"

// EnrollmentLifetime is how long a new secret waits for its confirm.
const EnrollmentLifetime = 10 * time.Minute

// confirmAttempts is how many wrong codes end an enrollment.
const confirmAttempts = 5

// States of the code.
const (
	StateOff         = "off"
	StateConsole     = "console"
	StateEnvironment = "environment"
)

// ErrUnreadable reports a stored secret that does not open, which refuses
// startup rather than leaving login without the code the operator bound.
var ErrUnreadable = errors.New("the stored admin one-time code cannot be opened: restore LLM_SECRETS_KEY or run kano-go totp-reset")

// record is the sealed row, unsealed. It is never logged or returned.
type record struct {
	Secret  string    `json:"secret"`
	BoundAt time.Time `json:"boundAt"`
}

// readSealed returns the sealed row, or "" when there is none. A snapshot
// without app_settings has none.
func readSealed(ctx context.Context, db *sql.DB) (string, error) {
	var tables int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='app_settings'`).Scan(&tables); err != nil {
		return "", err
	}
	if tables == 0 {
		return "", nil
	}
	var value string
	err := db.QueryRowContext(ctx, `SELECT value FROM app_settings WHERE key=?`, setting).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return value, err
}

// open unseals a stored secret.
func open(sealed, secrets string) (record, []byte, error) {
	plain, err := secretstore.Decrypt(sealed, secrets)
	if err != nil {
		return record{}, nil, ErrUnreadable
	}
	var out record
	if json.Unmarshal([]byte(plain), &out) != nil {
		return record{}, nil, ErrUnreadable
	}
	raw, err := adminauth.ParseTOTPSecret(out.Secret)
	if err != nil || len(raw) == 0 {
		return record{}, nil, ErrUnreadable
	}
	return out, raw, nil
}

// Load binds the stored secret to auth at startup, unless ADMIN_TOTP_SECRET
// provides the code. It reports when the stored secret was bound (zero
// without one or while the environment's is in use) and whether a secret is
// stored. A stored secret that does not open fails, so the process never
// signs in without the code its operator bound.
func Load(ctx context.Context, db *sql.DB, secrets string, auth *adminauth.Auth) (time.Time, bool, error) {
	sealed, err := readSealed(ctx, db)
	if err != nil || sealed == "" {
		return time.Time{}, false, err
	}
	if auth.EnvironmentCode() {
		return time.Time{}, true, nil
	}
	stored, raw, err := open(sealed, secrets)
	if err != nil {
		return time.Time{}, true, err
	}
	return stored.BoundAt, true, auth.SetStoredSecret(raw, 0)
}

// Reset deletes the stored secret and reports whether there was one. A
// running service keeps using it until it restarts.
func Reset(ctx context.Context, db *sql.DB) (bool, error) {
	result, err := db.ExecContext(ctx, `DELETE FROM app_settings WHERE key=?`, setting)
	if err != nil {
		return false, err
	}
	removed, err := result.RowsAffected()
	return removed > 0, err
}

// enrollment is a new secret waiting for its confirm, in memory only.
type enrollment struct {
	secret    string
	raw       []byte
	expiresAt time.Time
	failures  int
}

// Admin owns /api/admin/security/totp.
type Admin struct {
	store   *sqlite.Store
	secrets string
	auth    *adminauth.Auth
	now     func() time.Time
	random  io.Reader
	// mu serializes operator changes and guards the fields below.
	mu sync.Mutex
	// stored reports a stored secret, and boundAt when it was bound (zero
	// while the environment's is in use).
	stored  bool
	boundAt time.Time
	pending *enrollment
}

var _ httpx.AdminModule = (*Admin)(nil)

// New binds the console's secret to auth. secrets is LLM_SECRETS_KEY.
func New(store *sqlite.Store, secrets string, auth *adminauth.Auth) *Admin {
	return &Admin{store: store, secrets: secrets, auth: auth, now: time.Now}
}

// Load applies the stored secret at startup (see Load).
func (a *Admin) Load(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	boundAt, stored, err := Load(ctx, a.store.DB(), a.secrets, a.auth)
	if err != nil {
		return err
	}
	a.stored, a.boundAt = stored, boundAt
	return nil
}

// Status is what the console shows. It never carries the secret.
func (a *Admin) Status() map[string]any {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.statusLocked()
}

func (a *Admin) statusLocked() map[string]any {
	state := StateOff
	switch {
	case a.auth.EnvironmentCode():
		state = StateEnvironment
	case a.stored:
		state = StateConsole
	}
	out := map[string]any{
		"state": state, "asked": a.auth.RequiresCode(), "boundAt": nil,
		"storedUnused": state == StateEnvironment && a.stored, "enrollment": nil,
	}
	if state == StateConsole && !a.boundAt.IsZero() {
		out["boundAt"] = sqlite.ISOTime(a.boundAt)
	}
	if a.pendingLocked() != nil {
		out["enrollment"] = map[string]any{"expiresAt": sqlite.ISOTime(a.pending.expiresAt)}
	}
	return out
}

// pendingLocked is the enrollment still open, dropping an expired one.
func (a *Admin) pendingLocked() *enrollment {
	if a.pending != nil && !a.now().Before(a.pending.expiresAt) {
		a.pending = nil
	}
	return a.pending
}

// AdminRoutes lists the code's endpoints.
func (a *Admin) AdminRoutes() []httpx.Route {
	return []httpx.Route{
		{Method: http.MethodGet, Path: Path, Handler: func(w http.ResponseWriter, _ *http.Request) {
			httpx.WriteJSON(w, http.StatusOK, a.Status())
		}},
		{Method: http.MethodPost, Path: Path + "/enrollment", Handler: a.locked(a.managed(a.start))},
		{Method: http.MethodDelete, Path: Path + "/enrollment", Handler: a.locked(a.cancel)},
		{Method: http.MethodPut, Path: Path, Handler: a.locked(a.managed(a.confirm))},
		{Method: http.MethodDelete, Path: Path, Handler: a.locked(a.managed(a.turnOff))},
	}
}

// locked serializes changes against a writable store.
func (a *Admin) locked(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !a.store.Writable() {
			httpx.ReadOnly(w)
			return
		}
		a.mu.Lock()
		defer a.mu.Unlock()
		next(w, r)
	}
}

// managed refuses changes while ADMIN_TOTP_SECRET is set.
func (a *Admin) managed(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.auth.EnvironmentCode() {
			httpx.WriteCode(w, http.StatusConflict, "environment_managed")
			return
		}
		next(w, r)
	}
}

// start opens an enrollment with a new secret, replacing any open one. Its
// answer is the only one that carries the secret: as text for manual entry
// and as the QR code of its otpauth:// address.
func (a *Admin) start(w http.ResponseWriter, _ *http.Request) {
	secret, err := adminauth.GenerateTOTPSecret(a.random)
	if err != nil {
		httpx.WriteCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	raw, err := adminauth.ParseTOTPSecret(secret)
	if err != nil {
		httpx.WriteCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	picture, err := qrsvg.Encode(adminauth.TOTPURI(secret))
	if err != nil {
		httpx.WriteCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	a.pending = &enrollment{secret: secret, raw: raw, expiresAt: a.now().Add(EnrollmentLifetime)}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"secret": secret, "qr": picture, "expiresAt": sqlite.ISOTime(a.pending.expiresAt),
	})
}

// cancel ends the open enrollment.
func (a *Admin) cancel(w http.ResponseWriter, _ *http.Request) {
	a.pending = nil
	httpx.WriteJSON(w, http.StatusOK, a.statusLocked())
}

// fields reads the named string fields of a body; a missing one is "".
func fields(w http.ResponseWriter, r *http.Request, names ...string) (map[string]string, bool) {
	body, err := httpx.Body(w, r)
	if err != nil {
		httpx.WriteError(w, err)
		return nil, false
	}
	allowed := map[string]bool{}
	for _, name := range names {
		allowed[name] = true
	}
	out := map[string]string{}
	for name, raw := range body {
		var value string
		if !allowed[name] || json.Unmarshal(raw, &value) != nil {
			httpx.WriteError(w, httpx.Invalid("%s is not a string field of this request", name))
			return nil, false
		}
		out[name] = value
	}
	return out, true
}

// writeVerifyError answers a refused password or current code. It is never
// 401, which the console reads as a signed-out session.
func writeVerifyError(w http.ResponseWriter, err error) {
	if errors.Is(err, adminauth.ErrTooManyAttempts) {
		httpx.WriteCode(w, http.StatusTooManyRequests, "too_many_attempts")
		return
	}
	httpx.WriteCode(w, http.StatusForbidden, "invalid_credentials")
}

// confirm reads { password, code, currentCode }: code is from the new
// secret, and currentCode, needed while a stored secret is in use, from the
// old one. The new code is checked first, so a failed scan spends no
// password attempt; five wrong codes end the enrollment. A confirmed secret
// is sealed and stored, applies to the next login at once, and ends every
// other admin session.
func (a *Admin) confirm(w http.ResponseWriter, r *http.Request) {
	body, ok := fields(w, r, "password", "code", "currentCode")
	if !ok {
		return
	}
	pending := a.pendingLocked()
	if pending == nil {
		httpx.WriteCode(w, http.StatusConflict, "totp_enrollment_missing")
		return
	}
	now := a.now()
	step, matched := adminauth.MatchCode(pending.raw, body["code"], now)
	if !matched {
		if pending.failures++; pending.failures >= confirmAttempts {
			a.pending = nil
		}
		httpx.WriteCode(w, http.StatusBadRequest, "totp_code_invalid")
		return
	}
	if err := a.auth.VerifyChange(r, body["password"], body["currentCode"], a.stored); err != nil {
		writeVerifyError(w, err)
		return
	}
	plain, err := json.Marshal(record{Secret: pending.secret, BoundAt: now.UTC()})
	if err != nil {
		httpx.WriteCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	sealed, err := secretstore.Encrypt(string(plain), a.secrets)
	if err != nil {
		writeSealError(w, err)
		return
	}
	if _, err = a.store.DB().ExecContext(r.Context(), `INSERT INTO app_settings(key,value,updated_at) VALUES(?,?,?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value, updated_at=excluded.updated_at`, setting, sealed, sqlite.ISOTime(now)); err != nil {
		httpx.WriteCode(w, http.StatusServiceUnavailable, "database_unavailable")
		return
	}
	if err = a.auth.SetStoredSecret(pending.raw, step); err != nil {
		httpx.WriteCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	a.stored, a.boundAt, a.pending = true, now.UTC(), nil
	a.auth.EndOtherSessions(r)
	httpx.WriteJSON(w, http.StatusOK, a.statusLocked())
}

// turnOff reads { password, code } and removes the stored secret: login asks
// for the password alone again, and every other admin session ends.
func (a *Admin) turnOff(w http.ResponseWriter, r *http.Request) {
	body, ok := fields(w, r, "password", "code")
	if !ok {
		return
	}
	if !a.stored {
		httpx.WriteJSON(w, http.StatusOK, a.statusLocked())
		return
	}
	if err := a.auth.VerifyChange(r, body["password"], body["code"], true); err != nil {
		writeVerifyError(w, err)
		return
	}
	if _, err := Reset(r.Context(), a.store.DB()); err != nil {
		httpx.WriteCode(w, http.StatusServiceUnavailable, "database_unavailable")
		return
	}
	_ = a.auth.SetStoredSecret(nil, 0)
	a.stored, a.boundAt, a.pending = false, time.Time{}, nil
	a.auth.EndOtherSessions(r)
	httpx.WriteJSON(w, http.StatusOK, a.statusLocked())
}

func writeSealError(w http.ResponseWriter, err error) {
	if errors.Is(err, secretstore.ErrKeyRequired) {
		httpx.WriteCode(w, http.StatusServiceUnavailable, "secrets_key_unavailable")
		return
	}
	httpx.WriteCode(w, http.StatusInternalServerError, "internal_error")
}
