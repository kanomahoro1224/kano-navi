package sqlite

import (
	"context"
	"time"

	bilisource "github.com/yexca/kano-navi/backend/internal/source/bilibili"
	"github.com/yexca/kano-navi/backend/internal/source/history"
)

// BilibiliFetchPage is one page of an operator's Bilibili fetch: the uploads
// or the dynamics it listed, the account state after the request, and the
// continuation of the fetch.
type BilibiliFetchPage struct {
	Account  string
	Videos   []bilisource.Video
	Dynamics []bilisource.Dynamic
	State    bilisource.State
	// Source is the fetch's source (history.SourceBilibili or
	// history.SourceBilibiliDynamics); its checkpoint row is keyed by
	// CheckpointKey under <source>-window. Next is the next page's cursor, a
	// page number or a listing offset, and empty once the fetch is complete;
	// Discovered counts the records the selection has taken so far.
	Source, CheckpointKey, Next string
	Discovered                  int
	Window                      history.Window
}

// WriteBilibiliFetchPage commits one Bilibili fetch page together with the
// account state and the fetch's checkpoint, so a repeated fetch can neither
// skip nor repeat a page. Every listed record is written as the timed read
// writes it (a listing describes the whole record), and the upload read's
// newest cursor only moves forward. It counts the page's records that are new
// and the ones already stored; a dynamic the post rules leave unstored, such
// as a forward without wording of its own whose dynamic is unavailable, is
// neither. It never touches the timed reads' interval clocks.
func (s *Store) WriteBilibiliFetchPage(ctx context.Context, page BilibiliFetchPage) (history.Written, error) {
	if s == nil || !s.workflowWritable {
		return history.Written{}, ErrSourceWrite
	}
	if (page.Source != history.SourceBilibili && page.Source != history.SourceBilibiliDynamics) || page.CheckpointKey == "" ||
		(page.Source == history.SourceBilibili && len(page.Dynamics) > 0) || (page.Source == history.SourceBilibiliDynamics && len(page.Videos) > 0) {
		return history.Written{}, ErrSourceWrite
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return history.Written{}, ErrSourceWrite
	}
	defer tx.Rollback()
	if err = fenceSourceWrite(ctx, tx); err != nil {
		return history.Written{}, err
	}
	now := workflowTime(time.Now())
	out := history.Written{NewIDs: []string{}}
	changed := false
	newest, newestAt := "", ""
	for _, v := range page.Videos {
		_, existed, lookupErr := readVideoSource(ctx, tx, v.BVID)
		if lookupErr != nil {
			return history.Written{}, ErrSourceWrite
		}
		written, writeErr := writeBilibiliVideos(ctx, tx, []bilisource.Video{v}, now)
		if writeErr != nil {
			return history.Written{}, writeErr
		}
		changed = changed || written
		if existed {
			out.Stored++
		} else {
			out.New++
			out.NewIDs = append(out.NewIDs, v.BVID)
		}
		if newestAt == "" || parseInstant(v.PublishedAt).After(parseInstant(newestAt)) {
			newest, newestAt = v.BVID, v.PublishedAt
		}
	}
	for _, d := range page.Dynamics {
		existed, lookupErr := rowExists(ctx, tx, `SELECT 1 FROM posts WHERE id=?`, d.PostID())
		if lookupErr != nil {
			return history.Written{}, ErrSourceWrite
		}
		visible, writeErr := writeBilibiliPost(ctx, tx, page.Account, d, now)
		if writeErr != nil {
			return history.Written{}, ErrSourceWrite
		}
		changed = changed || visible
		if existed {
			out.Stored++
			continue
		}
		stored, lookupErr := rowExists(ctx, tx, `SELECT 1 FROM posts WHERE id=? AND source='bilibili'`, d.PostID())
		if lookupErr != nil {
			return history.Written{}, ErrSourceWrite
		}
		if stored {
			out.New++
			out.NewIDs = append(out.NewIDs, d.PostID())
		}
	}
	if err = writeBilibiliState(ctx, tx, page.Account, page.State, newest, newestAt); err != nil {
		return history.Written{}, err
	}
	metadata := map[string]any{"window": page.Window, "discovered": page.Discovered}
	if err = upsertSyncState(ctx, tx, page.Source+"-window", page.CheckpointKey, page.Next, "", metadata, now); err != nil {
		return history.Written{}, ErrSourceWrite
	}
	if changed {
		if err = bumpRevision(ctx, tx, now); err != nil {
			return history.Written{}, ErrSourceWrite
		}
	}
	if tx.Commit() != nil {
		return history.Written{}, ErrSourceWrite
	}
	return out, nil
}

// ClearBilibiliFetchCheckpoint drops the continuation of one Bilibili fetch
// selection, so the next fetch begins at the newest page.
func (s *Store) ClearBilibiliFetchCheckpoint(ctx context.Context, source, key string) error {
	if s == nil || !s.workflowWritable || (source != history.SourceBilibili && source != history.SourceBilibiliDynamics) || key == "" {
		return ErrSourceWrite
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ErrSourceWrite
	}
	defer tx.Rollback()
	if err = fenceSourceWrite(ctx, tx); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE sync_state SET cursor_id=NULL WHERE source=? AND account_id=?`, source+"-window", key); err != nil || tx.Commit() != nil {
		return ErrSourceWrite
	}
	return nil
}
