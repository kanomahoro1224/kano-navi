package sqlite

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

// webPushTestModeKey holds the console's browser test-mode switch. It is a
// setting rather than a subscription column because one operator decides it
// for the whole site.
const webPushTestModeKey = "web_push_test_mode"

// WebPushTestMode reports whether test notifications travel at high priority.
// An absent or unreadable value means off, which is the quieter default.
func (s *Store) WebPushTestMode(ctx context.Context) (bool, error) {
	var value string
	err := s.db.QueryRowContext(ctx,
		`SELECT value FROM app_settings WHERE key=?`, webPushTestModeKey).Scan(&value)
	switch {
	case err == sql.ErrNoRows:
		return false, nil
	case err != nil:
		return false, dbErr(err)
	}
	return value == "1" || strings.EqualFold(value, "true"), nil
}

// SaveWebPushTestMode persists the switch. The next test reads it.
func (s *Store) SaveWebPushTestMode(ctx context.Context, enabled bool) error {
	if !s.Writable() {
		return ErrReadOnly
	}
	value := "0"
	if enabled {
		value = "1"
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO app_settings(key,value,updated_at) VALUES(?,?,?)
		 ON CONFLICT(key) DO UPDATE SET value=excluded.value,updated_at=excluded.updated_at`,
		webPushTestModeKey, value, workflowTime(time.Now()))
	if err != nil {
		return ErrUnavailable
	}
	return nil
}
