// Package keyadmin keeps the Web Bot Auth signing key for /admin: it
// generates, rotates and removes the key, stores it sealed with
// LLM_SECRETS_KEY in app_settings like the YouTube push secret, and swaps
// the running webbotauth.Holder so a change applies without a restart.
// WEB_BOT_AUTH_KEY overrides the stored key while it is set.
package keyadmin

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/yexca/kano-navi/backend/internal/httpx"
	"github.com/yexca/kano-navi/backend/internal/secretstore"
	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
	"github.com/yexca/kano-navi/backend/internal/webbotauth"
)

// Operator settings in app_settings. The keys are sealed and are never
// returned or logged; the origin and the times are plain.
const (
	settingKey           = "web_bot_auth_key"
	settingCreated       = "web_bot_auth_key_created_at"
	settingPrevious      = "web_bot_auth_previous_key"
	settingPreviousUntil = "web_bot_auth_previous_until"
	settingOrigin        = "web_bot_auth_origin"
)

var settingKeys = []string{settingKey, settingCreated, settingPrevious, settingPreviousUntil, settingOrigin}

// Grace is how long the directory keeps listing a key a rotation replaced.
const Grace = 24 * time.Hour

// Signing states.
const (
	StateActive        = "active"
	StateNoKey         = "no_key"
	StateNoOrigin      = "no_origin"
	StateKeyUnreadable = "key_unreadable"
)

// Key sources.
const (
	SourceEnvironment = "environment"
	SourceStored      = "stored"
)

// Path is the module's route below /api/admin.
const Path = "/sources/shop/web-bot-auth"

// stored is what app_settings holds, unsealed.
type stored struct {
	key        *webbotauth.Key
	unreadable bool
	created    time.Time
	previous   *webbotauth.Key
	until      time.Time
	origin     string
}

// view is the state the last load or write applied.
type view struct {
	state, source, origin string
	key                   *webbotauth.Key
	settings              stored
}

// Admin owns /api/admin/sources/shop/web-bot-auth.
type Admin struct {
	store   *sqlite.Store
	db      *sql.DB
	secrets string
	holder  *webbotauth.Holder
	// env is the WEB_BOT_AUTH_KEY override, or nil.
	env *webbotauth.Key
	// publicOrigin is the normalized PUBLIC_ORIGIN, or empty.
	publicOrigin string
	now          func() time.Time
	random       io.Reader
	// mu serializes operator writes so storage and the holder agree.
	mu sync.Mutex
	// viewMu guards current, which status reads take.
	viewMu  sync.RWMutex
	current view
}

var _ httpx.AdminModule = (*Admin)(nil)

// New binds the stored key to holder. secrets is LLM_SECRETS_KEY, env the
// parsed WEB_BOT_AUTH_KEY (nil without one) and publicOrigin the normalized
// PUBLIC_ORIGIN.
func New(store *sqlite.Store, holder *webbotauth.Holder, secrets string, env *webbotauth.Key, publicOrigin string) *Admin {
	return &Admin{store: store, db: store.DB(), secrets: secrets, holder: holder, env: env, publicOrigin: publicOrigin, now: time.Now}
}

// Load applies the stored settings at startup and only fails when they
// cannot be read. A sealed key that no longer opens (a changed
// LLM_SECRETS_KEY) and a key without an https origin leave signing off with
// their state shown, not a failed start.
func (a *Admin) Load(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	settings, err := a.read(ctx)
	if err != nil {
		return err
	}
	a.apply(settings)
	return nil
}

func (a *Admin) read(ctx context.Context) (stored, error) {
	var out stored
	rows, err := a.db.QueryContext(ctx, `SELECT key, value FROM app_settings WHERE key IN (?, ?, ?, ?, ?)`,
		settingKey, settingCreated, settingPrevious, settingPreviousUntil, settingOrigin)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	values := map[string]string{}
	for rows.Next() {
		var key, value string
		if err = rows.Scan(&key, &value); err != nil {
			return out, err
		}
		values[key] = value
	}
	if err = rows.Err(); err != nil {
		return out, err
	}
	open := func(sealed string) (*webbotauth.Key, bool) {
		if sealed == "" {
			return nil, true
		}
		plain, err := secretstore.Decrypt(sealed, a.secrets)
		if err != nil {
			return nil, false
		}
		key, err := webbotauth.ParseKey(plain)
		return key, err == nil
	}
	var ok bool
	out.key, ok = open(values[settingKey])
	out.unreadable = !ok
	if out.key != nil {
		out.created = parseTime(values[settingCreated])
	}
	// A previous key that no longer opens is simply not listed.
	if previous, opened := open(values[settingPrevious]); opened && previous != nil {
		out.previous, out.until = previous, parseTime(values[settingPreviousUntil])
	}
	if origin, valid := webbotauth.HTTPSOrigin(values[settingOrigin]); valid {
		out.origin = origin
	}
	return out, nil
}

func parseTime(value string) time.Time {
	at, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}
	}
	return at
}

// apply works out the signing state and swaps the holder. The key is the
// environment's while WEB_BOT_AUTH_KEY is set, otherwise the stored one;
// the origin is the operator's, otherwise an https PUBLIC_ORIGIN.
func (a *Admin) apply(settings stored) {
	next := view{settings: settings, source: SourceStored, key: settings.key, origin: settings.origin}
	if a.env != nil {
		next.source, next.key = SourceEnvironment, a.env
	}
	if next.origin == "" {
		if origin, ok := webbotauth.HTTPSOrigin(a.publicOrigin); ok {
			next.origin = origin
		}
	}
	switch {
	case a.env == nil && settings.unreadable:
		next.state, next.source = StateKeyUnreadable, SourceStored
	case next.key == nil:
		next.state, next.source = StateNoKey, ""
	case next.origin == "":
		next.state = StateNoOrigin
	default:
		next.state = StateActive
	}
	var signer *webbotauth.Signer
	var retired []webbotauth.Retired
	if next.state == StateActive {
		signer, _ = webbotauth.NewSigner(next.key, next.origin)
		if next.source == SourceStored && settings.previous != nil {
			retired = []webbotauth.Retired{{Key: settings.previous, Until: settings.until}}
		}
	}
	if signer == nil {
		a.holder.Set(nil, nil)
		if next.state == StateActive {
			next.state = StateNoOrigin
		}
	} else {
		a.holder.Set(signer, retired)
	}
	a.viewMu.Lock()
	a.current = next
	a.viewMu.Unlock()
}

// Status is the state the console shows: whether signing is active and why
// not, the key's keyid and age, the origin, and, while signing, the
// directory address. It never carries a private key.
func (a *Admin) Status() map[string]any {
	a.viewMu.RLock()
	current := a.current
	a.viewMu.RUnlock()
	optional := func(value string) any {
		if value == "" {
			return nil
		}
		return value
	}
	out := map[string]any{
		"state": current.state, "source": optional(current.source), "keyId": nil, "createdAt": nil,
		"origin": optional(current.origin), "originSetting": optional(current.settings.origin),
		"publicOrigin": optional(a.publicOrigin), "directoryUrl": nil, "previous": nil,
		"environmentManaged": a.env != nil,
	}
	if current.key != nil {
		out["keyId"] = current.key.Thumbprint()
	}
	if current.source == SourceStored && current.key != nil && !current.settings.created.IsZero() {
		out["createdAt"] = sqlite.ISOTime(current.settings.created)
	}
	if current.state == StateActive {
		out["directoryUrl"] = current.origin + webbotauth.DirectoryPath
	}
	if current.state == StateActive && current.source == SourceStored && current.settings.previous != nil && a.now().Before(current.settings.until) {
		out["previous"] = map[string]any{"keyId": current.settings.previous.Thumbprint(), "until": sqlite.ISOTime(current.settings.until)}
	}
	return out
}

// AdminRoutes lists the key's endpoints.
func (a *Admin) AdminRoutes() []httpx.Route {
	return []httpx.Route{
		{Method: http.MethodGet, Path: Path, Handler: func(w http.ResponseWriter, r *http.Request) {
			httpx.WriteJSON(w, http.StatusOK, a.Status())
		}},
		{Method: http.MethodPut, Path: Path, Handler: a.locked(a.updateOrigin)},
		{Method: http.MethodPost, Path: Path + "/key", Handler: a.locked(a.managed(a.generate))},
		{Method: http.MethodDelete, Path: Path + "/key", Handler: a.locked(a.managed(a.remove))},
	}
}

// locked serializes settings writes against a writable store.
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

// managed refuses key changes while WEB_BOT_AUTH_KEY is set.
func (a *Admin) managed(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.env != nil {
			httpx.WriteCode(w, http.StatusConflict, "environment_managed")
			return
		}
		next(w, r)
	}
}

// updateOrigin reads { origin } (a bare https origin; empty or null clears
// it so PUBLIC_ORIGIN applies).
func (a *Admin) updateOrigin(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Body(w, r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	raw, ok := body["origin"]
	if !ok || len(body) != 1 {
		httpx.WriteError(w, httpx.Invalid("only origin can be changed"))
		return
	}
	var value *string
	if json.Unmarshal(raw, &value) != nil {
		httpx.WriteError(w, httpx.Invalid("origin must be a string"))
		return
	}
	origin := ""
	if value != nil && strings.TrimSpace(*value) != "" {
		normalized, valid := webbotauth.HTTPSOrigin(*value)
		if !valid {
			httpx.WriteError(w, httpx.Invalid("origin must be a bare https origin"))
			return
		}
		origin = normalized
	}
	a.write(w, r.Context(), map[string]string{settingOrigin: origin})
}

// generate makes a new key. A readable key it replaces stays listed in the
// directory for Grace, while requests sign with the new key at once; any
// older previous key is dropped.
func (a *Admin) generate(w http.ResponseWriter, r *http.Request) {
	settings, err := a.read(r.Context())
	if err != nil {
		httpx.WriteCode(w, http.StatusServiceUnavailable, "database_unavailable")
		return
	}
	key, err := webbotauth.GenerateKey(a.random)
	if err != nil {
		httpx.WriteCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	now := a.now()
	sealed, err := secretstore.Encrypt(key.Seed(), a.secrets)
	if err != nil {
		writeSealError(w, err)
		return
	}
	changes := map[string]string{settingKey: sealed, settingCreated: sqlite.ISOTime(now), settingPrevious: "", settingPreviousUntil: ""}
	if settings.key != nil {
		previous, err := secretstore.Encrypt(settings.key.Seed(), a.secrets)
		if err != nil {
			writeSealError(w, err)
			return
		}
		changes[settingPrevious], changes[settingPreviousUntil] = previous, sqlite.ISOTime(now.Add(Grace))
	}
	a.write(w, r.Context(), changes)
}

// remove deletes the stored key and any previous one: signing stops and the
// directory answers 404.
func (a *Admin) remove(w http.ResponseWriter, r *http.Request) {
	a.write(w, r.Context(), map[string]string{settingKey: "", settingCreated: "", settingPrevious: "", settingPreviousUntil: ""})
}

func writeSealError(w http.ResponseWriter, err error) {
	if errors.Is(err, secretstore.ErrKeyRequired) {
		httpx.WriteCode(w, http.StatusServiceUnavailable, "secrets_key_unavailable")
		return
	}
	httpx.WriteCode(w, http.StatusInternalServerError, "internal_error")
}

// write stores the changes in one transaction (an empty value deletes the
// row), then reloads and applies the settings and answers the status.
func (a *Admin) write(w http.ResponseWriter, ctx context.Context, changes map[string]string) {
	if err := a.save(ctx, changes); err != nil {
		httpx.WriteCode(w, http.StatusServiceUnavailable, "database_unavailable")
		return
	}
	settings, err := a.read(ctx)
	if err != nil {
		httpx.WriteCode(w, http.StatusServiceUnavailable, "database_unavailable")
		return
	}
	a.apply(settings)
	httpx.WriteJSON(w, http.StatusOK, a.Status())
}

func (a *Admin) save(ctx context.Context, changes map[string]string) error {
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := sqlite.ISOTime(a.now())
	for _, key := range settingKeys {
		value, ok := changes[key]
		switch {
		case !ok:
			continue
		case value == "":
			_, err = tx.ExecContext(ctx, `DELETE FROM app_settings WHERE key=?`, key)
		default:
			_, err = tx.ExecContext(ctx, `INSERT INTO app_settings(key,value,updated_at) VALUES(?,?,?)
			ON CONFLICT(key) DO UPDATE SET value=excluded.value, updated_at=excluded.updated_at`, key, value, now)
		}
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}
