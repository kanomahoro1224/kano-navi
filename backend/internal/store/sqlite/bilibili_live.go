package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	bilisource "github.com/yexca/kano-navi/backend/internal/source/bilibili"
)

// bilibiliLiveSource names the sync_state row (account_id = the mid) holding
// the account's live room state and the live host's backoff.
const bilibiliLiveSource = "bilibili-live"

type bilibiliLiveMetadata struct {
	RoomID        string `json:"roomId,omitempty"`
	Title         string `json:"title,omitempty"`
	LiveStatus    *int   `json:"liveStatus,omitempty"`
	LiveTime      string `json:"liveTime,omitempty"`
	CheckedAt     string `json:"checkedAt,omitempty"`
	LastAttemptAt string `json:"lastAttemptAt,omitempty"`
	BlockedUntil  string `json:"blockedUntil,omitempty"`
	Failures      int    `json:"failures,omitempty"`
	LastError     string `json:"lastError,omitempty"`
	// LastBroadcast is the start of the newest broadcast a check saw live,
	// LastLiveAt the last check that saw it live (its approximate end) and
	// EndedAt the first later check that saw the room off air.
	LastBroadcast string `json:"lastBroadcast,omitempty"`
	LastLiveAt    string `json:"lastLiveAt,omitempty"`
	EndedAt       string `json:"endedAt,omitempty"`
}

// BilibiliLive is the stored live room state.
type BilibiliLive struct {
	State bilisource.LiveState
	// LastBroadcast, LastLiveAt and EndedAt are zero until a broadcast was
	// seen. LastLiveAt is only an approximate end: the room may have gone off
	// air any time before EndedAt.
	LastBroadcast, LastLiveAt, EndedAt time.Time
}

func readBilibiliLive(ctx context.Context, q rowQuerier, account string) (BilibiliLive, error) {
	var meta string
	err := q.QueryRowContext(ctx, `SELECT COALESCE(metadata_json,'{}') FROM sync_state WHERE source=? AND account_id=?`, bilibiliLiveSource, account).Scan(&meta)
	if errors.Is(err, sql.ErrNoRows) {
		return BilibiliLive{}, nil
	}
	if err != nil {
		return BilibiliLive{}, err
	}
	var data bilibiliLiveMetadata
	_ = json.Unmarshal([]byte(meta), &data)
	out := BilibiliLive{LastBroadcast: parseInstant(data.LastBroadcast), LastLiveAt: parseInstant(data.LastLiveAt), EndedAt: parseInstant(data.EndedAt)}
	out.State = bilisource.LiveState{
		Room:      bilisource.Room{RoomID: data.RoomID, Title: data.Title, LiveTime: data.LiveTime},
		CheckedAt: parseInstant(data.CheckedAt), LastAttemptAt: parseInstant(data.LastAttemptAt),
		BlockedUntil: parseInstant(data.BlockedUntil), Failures: data.Failures, LastError: data.LastError,
	}
	if data.LiveStatus != nil {
		out.State.Room.LiveStatus = *data.LiveStatus
	}
	return out, nil
}

// BilibiliLiveState reads the stored live room state of one account.
func (s *Store) BilibiliLiveState(ctx context.Context, account string) (BilibiliLive, error) {
	out, err := readBilibiliLive(ctx, s.db, account)
	if err != nil {
		return BilibiliLive{}, dbErr(err)
	}
	return out, nil
}

// WriteBilibiliLive stores one live check. A failed check keeps the known
// room state and records the attempt and any backoff. It advances the
// dashboard revision only when what the board projects changed: the room,
// whether it is live and the broadcast's start or end.
func (s *Store) WriteBilibiliLive(ctx context.Context, account string, state bilisource.LiveState) (bool, error) {
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
	prior, err := readBilibiliLive(ctx, tx, account)
	if err != nil {
		return false, ErrSourceWrite
	}
	data := bilibiliLiveMetadata{
		RoomID: state.Room.RoomID, Title: state.Room.Title, LiveTime: state.Room.LiveTime,
		CheckedAt: formatInstant(state.CheckedAt), LastAttemptAt: formatInstant(state.LastAttemptAt),
		BlockedUntil: formatInstant(state.BlockedUntil), Failures: state.Failures, LastError: state.LastError,
		LastBroadcast: formatInstant(prior.LastBroadcast), LastLiveAt: formatInstant(prior.LastLiveAt), EndedAt: formatInstant(prior.EndedAt),
	}
	if state.Room.RoomID != "" {
		status := state.Room.LiveStatus
		data.LiveStatus = &status
	}
	// A fresh check (not a failed one) moves the broadcast bookkeeping.
	// A live room without a start time is live with an unknown start: the
	// broadcast already seen, or one that started by this check.
	if state.CheckedAt.After(prior.State.CheckedAt) {
		start := parseInstant(state.Room.LiveTime)
		switch {
		case state.Room.LiveStatus == bilisource.RoomLive:
			if start.IsZero() && (prior.LastBroadcast.IsZero() || !prior.EndedAt.IsZero()) {
				start = state.CheckedAt
			}
			if !start.IsZero() && !start.Equal(prior.LastBroadcast) {
				data.LastBroadcast, data.EndedAt = formatInstant(start), ""
			}
			data.LastLiveAt = formatInstant(state.CheckedAt)
		case !prior.LastBroadcast.IsZero() && prior.EndedAt.IsZero():
			data.EndedAt = formatInstant(state.CheckedAt)
		}
	}
	encoded, _ := json.Marshal(data)
	if _, err = tx.ExecContext(ctx, `INSERT INTO sync_state(source,account_id,last_success_at,metadata_json) VALUES(?,?,NULLIF(?,''),?)
 ON CONFLICT(source,account_id) DO UPDATE SET last_success_at=COALESCE(excluded.last_success_at,sync_state.last_success_at),metadata_json=excluded.metadata_json`,
		bilibiliLiveSource, account, formatInstant(state.CheckedAt), string(encoded)); err != nil {
		return false, ErrSourceWrite
	}
	live := func(room bilisource.Room) bool { return room.LiveStatus == bilisource.RoomLive }
	// The title is public through liveNow while the room is live.
	changed := prior.State.Room.RoomID != state.Room.RoomID || live(prior.State.Room) != live(state.Room) ||
		prior.State.Room.LiveTime != state.Room.LiveTime || formatInstant(prior.EndedAt) != data.EndedAt ||
		formatInstant(prior.LastBroadcast) != data.LastBroadcast || live(state.Room) && prior.State.Room.Title != state.Room.Title
	if changed {
		if err = bumpRevision(ctx, tx, workflowTime(time.Now())); err != nil {
			return false, ErrSourceWrite
		}
	}
	if tx.Commit() != nil {
		return false, ErrSourceWrite
	}
	return changed, nil
}
