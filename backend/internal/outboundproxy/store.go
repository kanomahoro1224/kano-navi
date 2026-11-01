package outboundproxy

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/yexca/kano-navi/backend/internal/secretstore"
	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
)

// settingKey stores the configuration as JSON in app_settings.
const settingKey = "outbound_proxy"

// ErrUnavailable fails requests closed when the stored configuration cannot
// be read, rather than silently connecting directly.
var ErrUnavailable = errors.New("outbound proxy setting is unavailable")

type storedConfig struct {
	Proxies        []storedProxy  `json:"proxies"`
	Routes         map[Area]Route `json:"routes"`
	DirectFallback bool           `json:"directFallback"`
}

// storedProxy keeps the password sealed with LLM_SECRETS_KEY.
type storedProxy struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Scheme         string `json:"scheme"`
	Host           string `json:"host"`
	Port           int    `json:"port"`
	Username       string `json:"username"`
	SealedPassword string `json:"password,omitempty"`
	Enabled        bool   `json:"enabled"`
}

type queryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// load reads the stored configuration. No row is an empty configuration;
// a password that no longer decrypts marks its proxy unreadable.
func load(ctx context.Context, db queryer, key string) (Config, error) {
	var raw string
	err := db.QueryRowContext(ctx, `SELECT value FROM app_settings WHERE key = ?`, settingKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return Normalize(Config{})
	}
	if err != nil {
		return Config{}, err
	}
	var stored storedConfig
	if json.Unmarshal([]byte(raw), &stored) != nil {
		return Config{}, ErrUnavailable
	}
	config := Config{Routes: stored.Routes, DirectFallback: stored.DirectFallback, Proxies: make([]Proxy, 0, len(stored.Proxies))}
	for _, item := range stored.Proxies {
		proxy := Proxy{ID: item.ID, Name: item.Name, Scheme: item.Scheme, Host: item.Host, Port: item.Port, Username: item.Username, Enabled: item.Enabled}
		if item.SealedPassword != "" {
			plain, decryptErr := secretstore.Decrypt(item.SealedPassword, key)
			proxy.Password, proxy.Unreadable = plain, decryptErr != nil
		}
		config.Proxies = append(config.Proxies, proxy)
	}
	normalized, err := Normalize(config)
	if err != nil {
		return Config{}, ErrUnavailable
	}
	return normalized, nil
}

// save writes a normalized configuration in tx. An unreadable password is
// kept sealed as it was so a later key change back can recover it.
func save(ctx context.Context, tx *sql.Tx, config Config, key string, previous map[string]string, now time.Time) error {
	stored := storedConfig{Routes: config.Routes, DirectFallback: config.DirectFallback, Proxies: make([]storedProxy, 0, len(config.Proxies))}
	for _, proxy := range config.Proxies {
		item := storedProxy{ID: proxy.ID, Name: proxy.Name, Scheme: proxy.Scheme, Host: proxy.Host, Port: proxy.Port, Username: proxy.Username, Enabled: proxy.Enabled}
		switch {
		case proxy.Unreadable:
			item.SealedPassword = previous[proxy.ID]
		case proxy.Password != "":
			sealed, err := secretstore.Encrypt(proxy.Password, key)
			if err != nil {
				return err
			}
			item.SealedPassword = sealed
		}
		stored.Proxies = append(stored.Proxies, item)
	}
	encoded, err := json.Marshal(stored)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO app_settings(key,value,updated_at) VALUES(?,?,?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value, updated_at=excluded.updated_at`, settingKey, string(encoded), sqlite.ISOTime(now))
	return err
}

// sealedPasswords returns the stored sealed passwords by proxy id.
func sealedPasswords(ctx context.Context, db queryer) (map[string]string, error) {
	var raw string
	err := db.QueryRowContext(ctx, `SELECT value FROM app_settings WHERE key = ?`, settingKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	var stored storedConfig
	if json.Unmarshal([]byte(raw), &stored) != nil {
		return map[string]string{}, nil
	}
	sealed := make(map[string]string, len(stored.Proxies))
	for _, proxy := range stored.Proxies {
		sealed[proxy.ID] = proxy.SealedPassword
	}
	return sealed, nil
}
