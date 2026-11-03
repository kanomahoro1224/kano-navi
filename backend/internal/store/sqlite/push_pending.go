package sqlite

import (
	"context"
	"strings"
	"time"
)

// PendingPush is one post whose notification waits for the translation step.
// The devices reading Japanese already have it; every other language follows
// once the machine translation settles, or after a grace period when the
// translate queue is off or keeps failing.
type PendingPush struct {
	PostID   string
	Platform string
	Kind     string
	Author   string
	Title    string
	Text     string
	URL      string
	// CreatedAt is when the post first waited; a re-ingested post restarts it.
	CreatedAt string
	// Ready is true when the translation settled or the grace period elapsed.
	Ready bool
}

// SavePendingPushes remembers posts for their remaining languages. A post
// ingested again replaces its row, so the newest text and URL are delivered.
func (s *Store) SavePendingPushes(ctx context.Context, items []PendingPush) error {
	if s == nil || s.db == nil || !s.writable || len(items) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return dbErr(err)
	}
	defer tx.Rollback()
	for _, item := range items {
		if _, err = tx.ExecContext(ctx, `INSERT INTO push_pending(post_id,platform,kind,author,title,text,url,created_at)
VALUES(?,?,?,?,?,?,?,?)
ON CONFLICT(post_id) DO UPDATE SET platform=excluded.platform, kind=excluded.kind, author=excluded.author,
  title=excluded.title, text=excluded.text, url=excluded.url, created_at=excluded.created_at`,
			item.PostID, item.Platform, item.Kind, item.Author, item.Title, item.Text, item.URL, item.CreatedAt); err != nil {
			return dbErr(err)
		}
	}
	return dbErr(tx.Commit())
}

// PendingPushes returns the newest pending posts that are ready to deliver,
// with the rest of the batch left for later runs. A row is ready when the
// translation of its current wording succeeded or was skipped, when its post
// no longer exists, or when it has waited longer than grace.
func (s *Store) PendingPushes(ctx context.Context, now time.Time, grace time.Duration, limit int) ([]PendingPush, error) {
	if s == nil || s.db == nil || limit < 1 {
		return []PendingPush{}, nil
	}
	cutoff := ISOTime(now.Add(-grace))
	rows, err := s.db.QueryContext(ctx, `SELECT p.post_id,p.platform,p.kind,COALESCE(p.author,''),COALESCE(p.title,''),p.text,p.url,p.created_at,
CASE WHEN p.created_at <= ? THEN 1
     WHEN po.id IS NULL THEN 1
     WHEN EXISTS (SELECT 1 FROM translation_states ts WHERE ts.owner_type='post' AND ts.owner_id=p.post_id AND ts.field='text'
       AND ts.source_hash=translation_hash(po.text) AND ts.status IN ('success','skipped')) THEN 1
     ELSE 0 END
FROM push_pending p LEFT JOIN posts po ON po.id=p.post_id
ORDER BY p.created_at DESC, p.post_id DESC LIMIT ?`, cutoff, limit)
	if err != nil {
		return nil, dbErr(err)
	}
	defer rows.Close()
	items := []PendingPush{}
	for rows.Next() {
		item := PendingPush{}
		var ready int
		if err = rows.Scan(&item.PostID, &item.Platform, &item.Kind, &item.Author, &item.Title, &item.Text, &item.URL, &item.CreatedAt, &ready); err != nil {
			return nil, dbErr(err)
		}
		item.Ready = ready == 1
		items = append(items, item)
	}
	return items, dbErr(rows.Err())
}

// DeletePendingPushes forgets rows whose notification was attempted, so a
// failed send is not retried and cannot duplicate a delivered one.
func (s *Store) DeletePendingPushes(ctx context.Context, ids []string) error {
	if s == nil || s.db == nil || !s.writable || len(ids) == 0 {
		return nil
	}
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = strings.TrimSpace(id)
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM push_pending WHERE post_id IN (`+placeholders(len(ids))+`)`, args...)
	return dbErr(err)
}
