package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// WebPushSubscription is one browser registered for Web Push. The endpoint is
// the identity: the browser issues it, the push service keeps it, and the
// server posts to it. p256dh and auth are the browser's public key material,
// needed to encrypt every payload for that one subscription.
type WebPushSubscription struct {
	Endpoint  string
	P256DH    string
	Auth      string
	Locale    string
	UserAgent string
	// Enabled is the master switch. The three category flags narrow what this
	// browser accepts; all of them default to true.
	Enabled     bool
	NotifyLive  bool
	NotifyVideo bool
	NotifyPost  bool
	// FailureCount counts consecutive delivery failures. A push service that
	// answers 5xx or 429 repeatedly is not addressed again after the limit.
	FailureCount int
	CreatedAt    string
	UpdatedAt    string
	LastSeenAt   string
}

// RegisterWebPushSubscription upserts one browser. The endpoint stays, the key
// material and switches are refreshed, and created_at is preserved, so a
// browser that re-subscribes keeps its place in the admin list.
func (s *Store) RegisterWebPushSubscription(ctx context.Context, sub WebPushSubscription) error {
	if s == nil || s.db == nil || !s.writable {
		return ErrUnavailable
	}
	sub.Endpoint = strings.TrimSpace(sub.Endpoint)
	sub.P256DH = strings.TrimSpace(sub.P256DH)
	sub.Auth = strings.TrimSpace(sub.Auth)
	if sub.Endpoint == "" || sub.P256DH == "" || sub.Auth == "" {
		return ErrUnavailable
	}
	now := ISOTime(time.Now())
	_, err := s.db.ExecContext(ctx, `INSERT INTO web_push_subscriptions(endpoint,p256dh,auth,locale,user_agent,enabled,notify_live,notify_video,notify_post,failure_count,created_at,updated_at,last_seen_at)
VALUES(?,?,?,?,?,?,?,?,?,0,?,?,?)
ON CONFLICT(endpoint) DO UPDATE SET p256dh=excluded.p256dh,auth=excluded.auth,locale=excluded.locale,user_agent=excluded.user_agent,enabled=excluded.enabled,notify_live=excluded.notify_live,notify_video=excluded.notify_video,notify_post=excluded.notify_post,failure_count=0,updated_at=excluded.updated_at,last_seen_at=excluded.last_seen_at`,
		sub.Endpoint, sub.P256DH, sub.Auth, nullable(sub.Locale), nullable(sub.UserAgent),
		pushFlag(sub.Enabled), pushFlag(sub.NotifyLive), pushFlag(sub.NotifyVideo), pushFlag(sub.NotifyPost),
		now, now, now)
	if err != nil {
		return dbErr(err)
	}
	return nil
}

// WebPushSubscription returns one browser by endpoint.
func (s *Store) WebPushSubscription(ctx context.Context, endpoint string) (WebPushSubscription, bool, error) {
	if s == nil || s.db == nil {
		return WebPushSubscription{}, false, ErrUnavailable
	}
	row := s.db.QueryRowContext(ctx, `SELECT endpoint,p256dh,auth,COALESCE(locale,''),COALESCE(user_agent,''),enabled,notify_live,notify_video,notify_post,failure_count,created_at,updated_at,last_seen_at
FROM web_push_subscriptions WHERE endpoint=?`, strings.TrimSpace(endpoint))
	var sub WebPushSubscription
	if err := row.Scan(&sub.Endpoint, &sub.P256DH, &sub.Auth, &sub.Locale, &sub.UserAgent,
		&sub.Enabled, &sub.NotifyLive, &sub.NotifyVideo, &sub.NotifyPost, &sub.FailureCount,
		&sub.CreatedAt, &sub.UpdatedAt, &sub.LastSeenAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return WebPushSubscription{}, false, nil
		}
		return WebPushSubscription{}, false, dbErr(err)
	}
	return sub, true, nil
}

// WebPushSubscriptions returns every enabled browser. The per-category
// decision stays with the caller, which reads the list once per dispatch
// instead of once per update.
func (s *Store) WebPushSubscriptions(ctx context.Context) ([]WebPushSubscription, error) {
	return s.webPushSubscriptions(ctx, false, 0)
}

// AdminWebPushSubscriptions returns the most recently seen rows, disabled ones
// included, for the console.
func (s *Store) AdminWebPushSubscriptions(ctx context.Context, limit int) ([]WebPushSubscription, error) {
	return s.webPushSubscriptions(ctx, true, limit)
}

func (s *Store) webPushSubscriptions(ctx context.Context, all bool, limit int) ([]WebPushSubscription, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	query := `SELECT endpoint,p256dh,auth,COALESCE(locale,''),COALESCE(user_agent,''),enabled,notify_live,notify_video,notify_post,failure_count,created_at,updated_at,last_seen_at
FROM web_push_subscriptions`
	if !all {
		query += ` WHERE enabled=1`
	}
	query += ` ORDER BY updated_at DESC`
	args := []any{}
	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, dbErr(err)
	}
	defer rows.Close()
	subs := []WebPushSubscription{}
	for rows.Next() {
		var sub WebPushSubscription
		if err = rows.Scan(&sub.Endpoint, &sub.P256DH, &sub.Auth, &sub.Locale, &sub.UserAgent,
			&sub.Enabled, &sub.NotifyLive, &sub.NotifyVideo, &sub.NotifyPost, &sub.FailureCount,
			&sub.CreatedAt, &sub.UpdatedAt, &sub.LastSeenAt); err != nil {
			return nil, dbErr(err)
		}
		subs = append(subs, sub)
	}
	if err = rows.Err(); err != nil {
		return nil, dbErr(err)
	}
	return subs, nil
}

// DeleteWebPushSubscription retires one browser, whether it unsubscribed or
// its push service reported the endpoint gone.
func (s *Store) DeleteWebPushSubscription(ctx context.Context, endpoint string) error {
	if s == nil || s.db == nil || !s.writable {
		return ErrUnavailable
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM web_push_subscriptions WHERE endpoint=?`,
		strings.TrimSpace(endpoint)); err != nil {
		return dbErr(err)
	}
	return nil
}

// RecordWebPushFailure counts one failed delivery and disables the row once it
// reaches limit, so a broken push service stops consuming every run.
func (s *Store) RecordWebPushFailure(ctx context.Context, endpoint string, limit int) error {
	if s == nil || s.db == nil || !s.writable {
		return ErrUnavailable
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE web_push_subscriptions
SET failure_count=failure_count+1, updated_at=?, enabled=CASE WHEN failure_count+1>=? THEN 0 ELSE enabled END
WHERE endpoint=?`, ISOTime(time.Now()), limit, strings.TrimSpace(endpoint)); err != nil {
		return dbErr(err)
	}
	return nil
}

// ClearWebPushFailures resets the counter after a successful delivery.
func (s *Store) ClearWebPushFailures(ctx context.Context, endpoint string) error {
	if s == nil || s.db == nil || !s.writable {
		return ErrUnavailable
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE web_push_subscriptions SET failure_count=0 WHERE endpoint=?`,
		strings.TrimSpace(endpoint)); err != nil {
		return dbErr(err)
	}
	return nil
}

// WebPushSubscriptionStats reports the rows the console shows: how many
// browsers registered and how many still accept updates.
func (s *Store) WebPushSubscriptionStats(ctx context.Context) (total, enabled int, err error) {
	if s == nil || s.db == nil {
		return 0, 0, ErrUnavailable
	}
	row := s.db.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(SUM(enabled),0) FROM web_push_subscriptions`)
	if err = row.Scan(&total, &enabled); err != nil {
		return 0, 0, dbErr(err)
	}
	return total, enabled, nil
}
