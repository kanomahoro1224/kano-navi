package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	bilisource "github.com/yexca/kano-navi/backend/internal/source/bilibili"
)

// Bilibili uploads are stored as videos with source = 'bilibili' and their BV
// ID as the row ID. The account's read bookkeeping, including the anonymous
// device identity and any backoff, is kept in sync_state (source 'bilibili')
// and is never returned by an API.

type bilibiliMetadata struct {
	Session       bilisource.Session `json:"session"`
	SessionAt     string             `json:"sessionAt,omitempty"`
	LastAttemptAt string             `json:"lastAttemptAt,omitempty"`
	BlockedUntil  string             `json:"blockedUntil,omitempty"`
	Failures      int                `json:"failures,omitempty"`
	LastError     string             `json:"lastError,omitempty"`
	BlockedAt     string             `json:"blockedAt,omitempty"`
	ClearedAt     string             `json:"clearedAt,omitempty"`
	// Dynamics is the dynamics listing's clock and outcome, and WBI its
	// cached signature keys (see bilibili_dynamics.go).
	Dynamics bilibiliDynamicsMetadata `json:"dynamics"`
	WBI      bilisource.WBIKeys       `json:"wbi"`
	// Login is the latest login check of the configured cookie (never the
	// cookie itself).
	Login bilibiliLoginMetadata `json:"login"`
}

type bilibiliLoginMetadata struct {
	CheckedAt string `json:"checkedAt,omitempty"`
	SignedIn  bool   `json:"signedIn,omitempty"`
}

type bilibiliDynamicsMetadata struct {
	LastAttemptAt string `json:"lastAttemptAt,omitempty"`
	LastSuccessAt string `json:"lastSuccessAt,omitempty"`
	LastError     string `json:"lastError,omitempty"`
}

func bilibiliMetadataOf(state bilisource.State) bilibiliMetadata {
	return bilibiliMetadata{
		Session: state.Session, SessionAt: formatInstant(state.SessionAt), LastAttemptAt: formatInstant(state.LastAttemptAt),
		BlockedUntil: formatInstant(state.BlockedUntil), Failures: state.Failures, LastError: state.LastError,
		BlockedAt: formatInstant(state.BlockedAt), ClearedAt: formatInstant(state.ClearedAt),
		Dynamics: bilibiliDynamicsMetadata{
			LastAttemptAt: formatInstant(state.Dynamics.LastAttemptAt), LastSuccessAt: formatInstant(state.Dynamics.LastSuccessAt),
			LastError: state.Dynamics.LastError,
		},
		WBI:   state.WBI,
		Login: bilibiliLoginMetadata{CheckedAt: formatInstant(state.Login.CheckedAt), SignedIn: state.Login.SignedIn},
	}
}

func parseInstant(value string) time.Time {
	t, _ := time.Parse(time.RFC3339Nano, value)
	return t
}

func formatInstant(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return workflowTime(t)
}

// BilibiliState reads the bookkeeping of one account; an account never read
// has the zero state.
func (s *Store) BilibiliState(ctx context.Context, account string) (bilisource.State, error) {
	var lastSuccess, meta string
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(last_success_at,''),COALESCE(metadata_json,'{}') FROM sync_state WHERE source='bilibili' AND account_id=?`, account).Scan(&lastSuccess, &meta)
	if errors.Is(err, sql.ErrNoRows) {
		return bilisource.State{}, nil
	}
	if err != nil {
		return bilisource.State{}, ErrUnavailable
	}
	var data bilibiliMetadata
	_ = json.Unmarshal([]byte(meta), &data)
	return bilisource.State{
		Session: data.Session, SessionAt: parseInstant(data.SessionAt),
		LastAttemptAt: parseInstant(data.LastAttemptAt), LastSuccessAt: parseInstant(lastSuccess),
		BlockedUntil: parseInstant(data.BlockedUntil), Failures: data.Failures, LastError: data.LastError,
		BlockedAt: parseInstant(data.BlockedAt), ClearedAt: parseInstant(data.ClearedAt),
		Dynamics: bilisource.DynamicsState{
			LastAttemptAt: parseInstant(data.Dynamics.LastAttemptAt), LastSuccessAt: parseInstant(data.Dynamics.LastSuccessAt),
			LastError: data.Dynamics.LastError,
		},
		WBI:   data.WBI,
		Login: bilisource.LoginState{CheckedAt: parseInstant(data.Login.CheckedAt), SignedIn: data.Login.SignedIn},
	}, nil
}

// BilibiliStatus is the console's view of one account: when it was read, any
// backoff and the stored uploads. It never includes the device identity.
type BilibiliStatus struct {
	Account       string  `json:"account"`
	Videos        int     `json:"videos"`
	LastAttemptAt *string `json:"lastAttemptAt"`
	LastSuccessAt *string `json:"lastSuccessAt"`
	BlockedUntil  *string `json:"blockedUntil"`
	LastError     *string `json:"lastError"`
	// Dynamics is the dynamics listing; it never includes the WBI keys.
	Dynamics BilibiliDynamicsStatus `json:"dynamics"`
	// LoginCheckedAt is when the login cookie was last verified.
	LoginCheckedAt *string `json:"loginCheckedAt"`
	// Live is the live room as the last check saw it.
	Live BilibiliLiveStatus `json:"live"`
}

type BilibiliDynamicsStatus struct {
	Posts         int     `json:"posts"`
	LastAttemptAt *string `json:"lastAttemptAt"`
	LastSuccessAt *string `json:"lastSuccessAt"`
	LastError     *string `json:"lastError"`
}

type BilibiliLiveStatus struct {
	RoomID     *string `json:"roomId"`
	LiveStatus *int    `json:"liveStatus"`
	Title      *string `json:"title"`
	LiveTime   *string `json:"liveTime"`
	CheckedAt  *string `json:"checkedAt"`
	// LastLiveAt is the last check that saw the room live: the approximate
	// end of the last broadcast once the room is off air.
	LastLiveAt   *string `json:"lastLiveAt"`
	BlockedUntil *string `json:"blockedUntil"`
	LastError    *string `json:"lastError"`
}

func (s *Store) BilibiliStatus(ctx context.Context, account string) (BilibiliStatus, error) {
	state, err := s.BilibiliState(ctx, account)
	if err != nil {
		return BilibiliStatus{}, err
	}
	out := BilibiliStatus{Account: account}
	if err = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM videos WHERE source='bilibili'`).Scan(&out.Videos); err != nil {
		return BilibiliStatus{}, ErrUnavailable
	}
	optional := func(value string) *string {
		if value == "" {
			return nil
		}
		return &value
	}
	out.LastAttemptAt, out.LastSuccessAt = optional(formatInstant(state.LastAttemptAt)), optional(formatInstant(state.LastSuccessAt))
	out.BlockedUntil, out.LastError = optional(formatInstant(state.BlockedUntil)), optional(state.LastError)
	if err = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM posts WHERE source='bilibili'`).Scan(&out.Dynamics.Posts); err != nil {
		return BilibiliStatus{}, ErrUnavailable
	}
	out.Dynamics.LastAttemptAt, out.Dynamics.LastSuccessAt = optional(formatInstant(state.Dynamics.LastAttemptAt)), optional(formatInstant(state.Dynamics.LastSuccessAt))
	out.Dynamics.LastError = optional(state.Dynamics.LastError)
	out.LoginCheckedAt = optional(formatInstant(state.Login.CheckedAt))
	live, err := s.BilibiliLiveState(ctx, account)
	if err != nil {
		return BilibiliStatus{}, err
	}
	room := live.State.Room
	out.Live = BilibiliLiveStatus{
		RoomID: optional(room.RoomID), Title: optional(room.Title), LiveTime: optional(room.LiveTime),
		CheckedAt: optional(formatInstant(live.State.CheckedAt)), LastLiveAt: optional(formatInstant(live.LastLiveAt)),
		BlockedUntil: optional(formatInstant(live.State.BlockedUntil)),
		LastError:    optional(live.State.LastError),
	}
	if room.RoomID != "" {
		status := room.LiveStatus
		out.Live.LiveStatus = &status
	}
	return out, nil
}

// WriteBilibiliRecent stores what one list read returned together with the
// account state, in one transaction. A failed or skipped read stores only the
// state, so known uploads survive. It reports whether a video changed.
func (s *Store) WriteBilibiliRecent(ctx context.Context, account string, result bilisource.Result) (bool, error) {
	return s.writeBilibili(ctx, account, result.Videos, result.State)
}

// WriteBilibiliVideo stores one upload an operator asked for and the state.
func (s *Store) WriteBilibiliVideo(ctx context.Context, account string, video bilisource.Video, state bilisource.State) (bool, error) {
	return s.writeBilibili(ctx, account, []bilisource.Video{video}, state)
}

// SaveBilibiliState stores the bookkeeping of a read that stored nothing,
// such as a failed operator request.
func (s *Store) SaveBilibiliState(ctx context.Context, account string, state bilisource.State) error {
	_, err := s.writeBilibili(ctx, account, nil, state)
	return err
}

func (s *Store) writeBilibili(ctx context.Context, account string, videos []bilisource.Video, state bilisource.State) (bool, error) {
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
	changed, err := writeBilibiliVideos(ctx, tx, videos, now)
	if err != nil {
		return false, err
	}
	newest, newestAt := "", ""
	for _, v := range videos {
		if newestAt == "" || parseInstant(v.PublishedAt).After(parseInstant(newestAt)) {
			newest, newestAt = v.BVID, v.PublishedAt
		}
	}
	if err = writeBilibiliState(ctx, tx, account, state, newest, newestAt); err != nil {
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

// mergeBilibiliMetadata keeps a block that is still active and later than
// the incoming one, with its failure bookkeeping, unless the incoming read
// started after that block was recorded and succeeded (an operator's forced
// read): no request starts while a block lasts, so any other such block was
// stored by a concurrent read that this write must not erase. The latest
// attempt of each read is kept as well.
func mergeBilibiliMetadata(stored, incoming bilibiliMetadata, now time.Time) bilibiliMetadata {
	storedBlock := parseInstant(stored.BlockedUntil)
	lifted := !parseInstant(stored.BlockedAt).IsZero() && parseInstant(incoming.ClearedAt).After(parseInstant(stored.BlockedAt))
	if storedBlock.After(now) && storedBlock.After(parseInstant(incoming.BlockedUntil)) && !lifted {
		incoming.BlockedUntil, incoming.Failures, incoming.LastError = stored.BlockedUntil, stored.Failures, stored.LastError
		incoming.BlockedAt = stored.BlockedAt
	}
	if parseInstant(stored.ClearedAt).After(parseInstant(incoming.ClearedAt)) {
		incoming.ClearedAt = stored.ClearedAt
	}
	if parseInstant(stored.LastAttemptAt).After(parseInstant(incoming.LastAttemptAt)) {
		incoming.LastAttemptAt = stored.LastAttemptAt
	}
	if parseInstant(stored.Dynamics.LastAttemptAt).After(parseInstant(incoming.Dynamics.LastAttemptAt)) {
		incoming.Dynamics = stored.Dynamics
	}
	if parseInstant(stored.Login.CheckedAt).After(parseInstant(incoming.Login.CheckedAt)) {
		incoming.Login = stored.Login
	}
	return incoming
}

// writeBilibiliVideos upserts uploads. A list read is a complete description
// of an upload, so it replaces the stored platform fields and keeps the
// replaced title and cover as history. Operator corrections survive. A
// listing without a description (the official client's) keeps the stored
// one. The list carries no live-replay flag, so it writes the form only for
// a new row (upload, unsettled); a stored form and who settled it
// (bilibili_forms.go) are kept.
func writeBilibiliVideos(ctx context.Context, tx *sql.Tx, videos []bilisource.Video, now string) (bool, error) {
	changed := false
	for _, v := range videos {
		var before string
		if err := tx.QueryRowContext(ctx, youtubeVideoState, v.BVID).Scan(&before); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return false, ErrSourceWrite
		}
		prior, existed, err := readVideoSource(ctx, tx, v.BVID)
		if err != nil {
			return false, ErrSourceWrite
		}
		var duration any
		if v.DurationSeconds > 0 {
			duration = v.DurationSeconds
		}
		raw, _ := json.Marshal(map[string]any{"aid": v.AID, "description": v.Description})
		written, err := tx.ExecContext(ctx, `INSERT INTO videos(id,source,title,published_at,scheduled_at,url,thumbnail_url,kind,is_upcoming,live_checked_at,duration_seconds,raw_json) VALUES(?,'bilibili',?,?,NULL,?,?,?,0,?,?,?)
 ON CONFLICT(id) DO UPDATE SET title=excluded.title,published_at=excluded.published_at,url=excluded.url,thumbnail_url=COALESCE(excluded.thumbnail_url,videos.thumbnail_url),live_checked_at=excluded.live_checked_at,duration_seconds=COALESCE(excluded.duration_seconds,videos.duration_seconds),raw_json=CASE WHEN COALESCE(json_extract(excluded.raw_json,'$.description'),'')='' AND json_valid(videos.raw_json) THEN json_set(videos.raw_json,'$.aid',json_extract(excluded.raw_json,'$.aid')) ELSE excluded.raw_json END,refresh_requested_at=NULL
 WHERE videos.source='bilibili'`,
			v.BVID, v.Title, v.PublishedAt, v.URL, nullable(v.ThumbnailURL), v.Kind, now, duration, string(raw))
		if err != nil {
			return false, ErrSourceWrite
		}
		if count, e := written.RowsAffected(); e != nil {
			return false, ErrSourceWrite
		} else if count == 0 {
			continue
		}
		if v.ThumbnailURL != "" {
			if err = replaceMediaLink(ctx, tx, "bilibili", v.ThumbnailURL, "video", v.BVID, "thumbnail", 0, now); err != nil {
				return false, ErrSourceWrite
			}
		}
		if existed && (prior.Title != v.Title || v.ThumbnailURL != "" && prior.ThumbnailURL != v.ThumbnailURL) {
			if err = recordVideoRevision(ctx, tx, v.BVID, prior, revisionSync, now); err != nil {
				return false, ErrSourceWrite
			}
		}
		// A merged upload's title is its YouTube video's Bilibili translation.
		translated, err := refreshMergeTranslations(ctx, tx, v.BVID, now)
		if err != nil {
			return false, ErrSourceWrite
		}
		var after string
		if err = tx.QueryRowContext(ctx, youtubeVideoState, v.BVID).Scan(&after); err != nil {
			return false, ErrSourceWrite
		}
		changed = changed || translated || after != before
	}
	return changed, nil
}
