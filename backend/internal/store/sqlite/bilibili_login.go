package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// bilibiliLoginSetting is the app_settings row of the Bilibili login the
// server owns, sealed with LLM_SECRETS_KEY by bilisource.Logins. The store
// never opens it.
const bilibiliLoginSetting = "bilibili_login"

// BilibiliLoginRecord is the sealed Bilibili login, or "" when none is
// stored.
func (s *Store) BilibiliLoginRecord(ctx context.Context) (string, error) {
	if s == nil || s.db == nil {
		return "", ErrUnavailable
	}
	var value string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM app_settings WHERE key=?`, bilibiliLoginSetting).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", ErrUnavailable
	}
	return value, nil
}

// SwapBilibiliLoginRecord replaces the sealed login with to when the row
// still holds from ("" for no row); an empty to deletes it. It reports
// whether it replaced, so a write that lost to another one since its read
// changes nothing. Operator imports and the workflow's refresh both write
// it, so either write permission is enough.
func (s *Store) SwapBilibiliLoginRecord(ctx context.Context, from, to string) (bool, error) {
	if s == nil || s.db == nil || (!s.writable && !s.workflowWritable) {
		return false, ErrReadOnly
	}
	now := workflowTime(time.Now())
	var result sql.Result
	var err error
	switch {
	case from == "" && to == "":
		return false, nil
	case from == "":
		result, err = s.db.ExecContext(ctx, `INSERT INTO app_settings(key,value,updated_at) VALUES(?,?,?) ON CONFLICT(key) DO NOTHING`, bilibiliLoginSetting, to, now)
	case to == "":
		result, err = s.db.ExecContext(ctx, `DELETE FROM app_settings WHERE key=? AND value=?`, bilibiliLoginSetting, from)
	default:
		result, err = s.db.ExecContext(ctx, `UPDATE app_settings SET value=?, updated_at=? WHERE key=? AND value=?`, to, now, bilibiliLoginSetting, from)
	}
	if err != nil {
		return false, ErrUnavailable
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return false, ErrUnavailable
	}
	return changed == 1, nil
}
