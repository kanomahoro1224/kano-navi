package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	bilisource "github.com/yexca/kano-navi/backend/internal/source/bilibili"
)

// Bilibili dynamics are posts with source = 'bilibili', their ID prefixed
// with bilibili- (bilisource.PostPrefix) so that they never meet an X status
// ID in posts.id, account_handle the account's mid and url the opus page.
// The listing's bookkeeping (its own attempt clock, outcome and the WBI keys)
// is kept in the metadata of the account's 'bilibili' sync_state row, beside
// the upload read's, because both share the captcha backoff stored there.

// bilibiliPostLabel is the label every dynamic gets, as an X post does.
const bilibiliPostLabel = "DAILY / 近况"

// WriteBilibiliDynamics stores what one listing read returned together with
// the account state, in one transaction. A failed or skipped read stores only
// the state, so known posts survive. It reports whether a public post
// changed, and advances the dashboard revision only then.
func (s *Store) WriteBilibiliDynamics(ctx context.Context, account string, result bilisource.DynamicsResult) (bool, error) {
	return s.writeBilibiliPosts(ctx, account, result.Dynamics, result.State)
}

// WriteBilibiliDynamic stores one dynamic an operator asked for and the state.
func (s *Store) WriteBilibiliDynamic(ctx context.Context, account string, d bilisource.Dynamic, state bilisource.State) (bool, error) {
	return s.writeBilibiliPosts(ctx, account, []bilisource.Dynamic{d}, state)
}

func (s *Store) writeBilibiliPosts(ctx context.Context, account string, dynamics []bilisource.Dynamic, state bilisource.State) (bool, error) {
	if s == nil || !s.workflowWritable {
		return false, ErrSourceWrite
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, ErrSourceWrite
	}
	defer tx.Rollback()
	if err = fenceSourceWrite(ctx, tx); err != nil {
		return false, err
	}
	now := workflowTime(time.Now())
	changed := false
	for _, d := range dynamics {
		visible, err := writeBilibiliPost(ctx, tx, account, d, now)
		if err != nil {
			return false, ErrSourceWrite
		}
		changed = changed || visible
	}
	if err = writeBilibiliState(ctx, tx, account, state, "", ""); err != nil {
		return false, err
	}
	if changed {
		if err = bumpRevision(ctx, tx, now); err != nil {
			return false, ErrSourceWrite
		}
	}
	if tx.Commit() != nil {
		return false, ErrSourceWrite
	}
	return changed, nil
}

// bilibiliPostState is what the board shows of a dynamic: its fields and its
// images in order.
const bilibiliPostState = `SELECT p.text||char(31)||p.published_at||char(31)||p.url||char(31)||COALESCE(p.media_url,'')||char(31)||COALESCE(p.hidden_at,'')||char(31)||
 COALESCE((SELECT group_concat(ma.source_url,char(30)) FROM (SELECT media_id FROM media_links WHERE owner_type='post' AND owner_id=p.id AND role='post-image' ORDER BY position) ml JOIN media_assets ma ON ma.id=ml.media_id),'')
 FROM posts p WHERE p.id=?`

// writeBilibiliPost upserts one dynamic and its image links. A row of
// another source with the same ID is left alone. Operator hides survive. It
// reports whether what the board shows changed.
func writeBilibiliPost(ctx context.Context, tx *sql.Tx, account string, d bilisource.Dynamic, now string) (bool, error) {
	id := d.PostID()
	var before string
	if err := tx.QueryRowContext(ctx, bilibiliPostState, id).Scan(&before); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	raw, _ := json.Marshal(d.Raw)
	media := ""
	if len(d.Images) > 0 {
		media = d.Images[0]
	}
	written, err := tx.ExecContext(ctx, `INSERT INTO posts(id,source,account_handle,type,label,text,published_at,url,likes,reposts,replies,media_url,media_alt,raw_json,quote_json) VALUES(?,'bilibili',?,'daily',?,?,?,?,0,0,0,?,NULL,?,NULL)
 ON CONFLICT(id) DO UPDATE SET account_handle=excluded.account_handle,text=excluded.text,published_at=excluded.published_at,url=excluded.url,media_url=excluded.media_url,raw_json=excluded.raw_json
 WHERE posts.source='bilibili'`, id, account, bilibiliPostLabel, d.Text, d.PublishedAt, d.URL(), nullable(media), string(raw))
	if err != nil {
		return false, err
	}
	if count, e := written.RowsAffected(); e != nil {
		return false, e
	} else if count != 1 {
		return false, nil
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM media_links WHERE owner_type='post' AND owner_id=? AND role='post-image'`, id); err != nil {
		return false, err
	}
	for i, u := range d.Images {
		if err = replaceMediaLink(ctx, tx, "bilibili", u, "post", id, "post-image", i, now); err != nil {
			return false, err
		}
	}
	var after string
	if err = tx.QueryRowContext(ctx, bilibiliPostState, id).Scan(&after); err != nil {
		return false, err
	}
	return after != before, nil
}

// writeBilibiliState stores the account state in its 'bilibili' sync_state
// row. newest and newestAt move the cursor of the upload read forward.
func writeBilibiliState(ctx context.Context, tx *sql.Tx, account string, state bilisource.State, newest, newestAt string) error {
	// Operator reads run outside the workflow queue, so another read may have
	// stored a captcha block since this one read the state.
	var storedMeta string
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(metadata_json,'{}') FROM sync_state WHERE source='bilibili' AND account_id=?`, account).Scan(&storedMeta); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return ErrSourceWrite
	}
	var stored bilibiliMetadata
	_ = json.Unmarshal([]byte(storedMeta), &stored)
	meta, _ := json.Marshal(mergeBilibiliMetadata(stored, bilibiliMetadataOf(state), time.Now()))
	// The cursor only names the newest upload seen; reads are not incremental.
	_, err := tx.ExecContext(ctx, `INSERT INTO sync_state(source,account_id,cursor_id,cursor_time,last_success_at,metadata_json) VALUES('bilibili',?,NULLIF(?,''),NULLIF(?,''),NULLIF(?,''),?)
 ON CONFLICT(source,account_id) DO UPDATE SET
  cursor_id=CASE WHEN julianday(excluded.cursor_time)>COALESCE(julianday(sync_state.cursor_time),0) THEN excluded.cursor_id ELSE sync_state.cursor_id END,
  cursor_time=CASE WHEN julianday(excluded.cursor_time)>COALESCE(julianday(sync_state.cursor_time),0) THEN excluded.cursor_time ELSE sync_state.cursor_time END,
  last_success_at=COALESCE(excluded.last_success_at,sync_state.last_success_at),metadata_json=excluded.metadata_json`,
		account, newest, newestAt, formatInstant(state.LastSuccessAt), string(meta))
	if err != nil {
		return ErrSourceWrite
	}
	return nil
}
