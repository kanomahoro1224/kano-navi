package sqlite

import (
	"context"
	"strings"
	"time"
)

// PushDevice is one HarmonyOS device registered for Huawei Push Kit delivery.
// Token is the push token Huawei issued; the service rotates it on reinstall
// and after long inactivity, so the newest registration owns the row.
type PushDevice struct {
	Token      string
	DeviceID   string
	Platform   string
	Locale     string
	AppVersion string
	// Enabled is the master switch. The three category flags narrow what this
	// device accepts; all of them default to true.
	Enabled     bool
	NotifyLive  bool
	NotifyVideo bool
	NotifyPost  bool
}

// RegisterPushDevice upserts a device and retires the same device's previous
// token, so a rotated token cannot deliver a second copy of every update.
func (s *Store) RegisterPushDevice(ctx context.Context, device PushDevice) error {
	if s == nil || s.db == nil || !s.writable {
		return ErrUnavailable
	}
	device.Token = strings.TrimSpace(device.Token)
	device.DeviceID = strings.TrimSpace(device.DeviceID)
	if device.Token == "" || device.DeviceID == "" {
		return ErrUnavailable
	}
	platform := strings.TrimSpace(device.Platform)
	if platform == "" {
		platform = "harmonyos"
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return dbErr(err)
	}
	defer tx.Rollback()

	if _, err = tx.ExecContext(ctx, `DELETE FROM push_devices WHERE device_id=? AND token<>?`,
		device.DeviceID, device.Token); err != nil {
		return dbErr(err)
	}
	now := ISOTime(time.Now())
	if _, err = tx.ExecContext(ctx, `INSERT INTO push_devices(token,device_id,platform,locale,app_version,enabled,notify_live,notify_video,notify_post,created_at,updated_at,last_seen_at)
VALUES(?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(token) DO UPDATE SET device_id=excluded.device_id,platform=excluded.platform,locale=excluded.locale,app_version=excluded.app_version,enabled=excluded.enabled,notify_live=excluded.notify_live,notify_video=excluded.notify_video,notify_post=excluded.notify_post,updated_at=excluded.updated_at,last_seen_at=excluded.last_seen_at`,
		device.Token, device.DeviceID, platform, nullable(device.Locale), nullable(device.AppVersion),
		pushFlag(device.Enabled), pushFlag(device.NotifyLive), pushFlag(device.NotifyVideo), pushFlag(device.NotifyPost),
		now, now, now); err != nil {
		return dbErr(err)
	}
	return tx.Commit()
}

// PushDevices returns every device that accepts at least one category. The
// per-category decision stays with the caller: a device row is read once per
// send instead of once per category.
func (s *Store) PushDevices(ctx context.Context) ([]PushDevice, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	rows, err := s.db.QueryContext(ctx, `SELECT token,device_id,platform,COALESCE(locale,''),COALESCE(app_version,''),enabled,notify_live,notify_video,notify_post
FROM push_devices WHERE enabled=1 ORDER BY updated_at DESC`)
	if err != nil {
		return nil, dbErr(err)
	}
	defer rows.Close()
	devices := []PushDevice{}
	for rows.Next() {
		var device PushDevice
		if err = rows.Scan(&device.Token, &device.DeviceID, &device.Platform, &device.Locale, &device.AppVersion,
			&device.Enabled, &device.NotifyLive, &device.NotifyVideo, &device.NotifyPost); err != nil {
			return nil, dbErr(err)
		}
		devices = append(devices, device)
	}
	return devices, dbErr(rows.Err())
}

// DeletePushDevices removes tokens Huawei rejected as invalid or unregistered.
// It is the only cleanup path: a stale token otherwise costs one failed send
// per update forever.
func (s *Store) DeletePushDevices(ctx context.Context, tokens []string) error {
	if s == nil || s.db == nil || !s.writable || len(tokens) == 0 {
		return nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(tokens)), ",")
	args := make([]any, 0, len(tokens))
	for _, token := range tokens {
		args = append(args, token)
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM push_devices WHERE token IN (`+placeholders+`)`, args...)
	return dbErr(err)
}

// pushFlag stores a boolean as the integer SQLite keeps for the column.
func pushFlag(value bool) int {
	if value {
		return 1
	}
	return 0
}
