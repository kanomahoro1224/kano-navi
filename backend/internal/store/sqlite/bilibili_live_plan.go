package sqlite

import (
	"context"
	"time"

	bilisource "github.com/yexca/kano-navi/backend/internal/source/bilibili"
)

// BilibiliLiveTarget is an announced Bilibili stream the live check follows.
type BilibiliLiveTarget struct {
	EventID, Title string
	StartsAt       time.Time
	Phase          string
	DueAt          time.Time
}

// BilibiliLivePlan is when the account's room is next asked about: the
// earliest due announced stream, or the baseline interval after the last
// check, and never during the live host's backoff.
type BilibiliLivePlan struct {
	Targets []BilibiliLiveTarget
	// Phase is the phase of the earliest due target, or LivePhaseBaseline.
	Phase string
	DueAt time.Time
	Live  BilibiliLive
}

// LiveNow is a broadcast of the account's room that no announced entry
// matched, as public reads list it.
type LiveNow struct {
	Platform, URL, Title, StartedAt string
}

// LivePhaseBaseline is a check no announced stream asked for.
const LivePhaseBaseline = "baseline"

// A broadcast that starts at L is the announced stream starting at S when
// S-2h <= L <= S+3h: a stream may open a little early or start well late.
const (
	bilibiliMatchEarly = 2 * time.Hour
	bilibiliMatchLate  = 3 * time.Hour
)

// bilibiliStreamEvents selects the Bilibili streams: active, timed stream
// or members-only entries whose platform is bilibili, whatever post
// announced them.
const bilibiliStreamEvents = `e.deleted_at IS NULL AND e.merged_into IS NULL AND e.platform='bilibili' AND e.event_type IN ('stream','member') AND e.starts_at IS NOT NULL`

// BilibiliLivePlan plans the next check of the account's room. Announced
// Bilibili streams starting within liveProbeLead or in the last
// liveProbeLookback are followed through the same phases as YouTube videos,
// at the Bilibili interval; a stream whose broadcast the room already showed
// and ended, ended entries and manually confirmed cancellations are left
// out.
func (s *Store) BilibiliLivePlan(ctx context.Context, account string, now time.Time, settings LiveProbeSettings) (BilibiliLivePlan, error) {
	live, err := readBilibiliLive(ctx, s.db, account)
	if err != nil {
		return BilibiliLivePlan{}, dbErr(err)
	}
	plan := BilibiliLivePlan{Targets: []BilibiliLiveTarget{}, Live: live}
	interval := time.Duration(settings.BilibiliIntervalMinutes) * time.Minute
	last := live.State.CheckedAt
	if live.State.LastAttemptAt.After(last) {
		last = live.State.LastAttemptAt
	}
	plan.Phase, plan.DueAt = LivePhaseBaseline, now
	if !last.IsZero() {
		plan.DueAt = last.Add(interval)
	}
	n := now.UTC().Format(time.RFC3339Nano)
	rows, err := s.db.QueryContext(ctx, `SELECT e.id,e.title,e.starts_at FROM events e WHERE `+bilibiliStreamEvents+`
 AND julianday(e.starts_at)<=julianday(?1,?2) AND julianday(e.starts_at)>=julianday(?1,?3)
 AND COALESCE(e.cancellation_status,'none')<>'manual_confirmed' AND NOT (e.ends_at IS NOT NULL AND julianday(e.ends_at)<=julianday(?1))
 ORDER BY julianday(e.starts_at),e.id LIMIT 20`, n, sqliteModifier(liveProbeLead), sqliteModifier(-liveProbeLookback))
	if err != nil {
		return BilibiliLivePlan{}, dbErr(err)
	}
	defer rows.Close()
	starts := []time.Time{}
	targets := []BilibiliLiveTarget{}
	for rows.Next() {
		var target BilibiliLiveTarget
		var start string
		if err = rows.Scan(&target.EventID, &target.Title, &start); err != nil {
			return BilibiliLivePlan{}, dbErr(err)
		}
		if target.StartsAt = parseInstant(start); !target.StartsAt.IsZero() {
			targets = append(targets, target)
			starts = append(starts, target.StartsAt)
		}
	}
	if err = rows.Err(); err != nil {
		return BilibiliLivePlan{}, dbErr(err)
	}
	ended := -1
	if !live.EndedAt.IsZero() {
		ended = live.matchBroadcast(starts)
	}
	phases := LiveProbeSettings{IntervalMinutes: settings.BilibiliIntervalMinutes}
	status := ""
	if live.State.Room.LiveStatus == bilisource.RoomLive {
		status = LivePhaseLive
	}
	for i, target := range targets {
		if i == ended {
			continue
		}
		target.Phase, target.DueAt = phases.liveProbeDue(target.StartsAt, last, status, now)
		plan.Targets = append(plan.Targets, target)
		if target.DueAt.Before(plan.DueAt) {
			plan.Phase, plan.DueAt = target.Phase, target.DueAt
		}
	}
	if plan.DueAt.Before(live.State.BlockedUntil) {
		plan.DueAt = live.State.BlockedUntil
	}
	return plan, nil
}

// matchBroadcast is the index of the announced start nearest the last
// broadcast within the matching window, or -1. One broadcast matches at most
// one entry.
func (l BilibiliLive) matchBroadcast(starts []time.Time) int {
	if l.LastBroadcast.IsZero() {
		return -1
	}
	best, bestGap := -1, time.Duration(0)
	for i, start := range starts {
		gap := l.LastBroadcast.Sub(start)
		if gap < -bilibiliMatchEarly || gap > bilibiliMatchLate {
			continue
		}
		if best < 0 || gap.Abs() < bestGap {
			best, bestGap = i, gap.Abs()
		}
	}
	return best
}

// attachBilibiliLive projects the account's room onto the Bilibili streams
// at read time. Every Bilibili stream or members-only stream carries the
// room as liveURL once a check learned it, so the stored link stays the
// announcement's. The stream nearest the room's broadcast within the
// matching window is live while the check is younger than two Bilibili
// intervals, and the one of the last broadcast is ended once a later check
// saw the room off air. Bilibili has no reservation state, so no entry is
// upcoming. A fresh broadcast that matches no entry is returned as LiveNow,
// and no entry is created for it.
func (s *Store) attachBilibiliLive(ctx context.Context, events []DashboardEvent, now time.Time) (*LiveNow, error) {
	live, err := readBilibiliLive(ctx, s.db, bilisource.OptionsFromEnv().Account)
	if err != nil {
		return nil, dbErr(err)
	}
	room := live.State.Room
	if room.RoomID == "" {
		return nil, nil
	}
	settings, err := s.LiveProbeSettings(ctx)
	if err != nil {
		return nil, err
	}
	link := bilisource.RoomURL(room.RoomID)
	fresh := live.State.CheckedAt.After(now.Add(-2 * time.Duration(settings.BilibiliIntervalMinutes) * time.Minute))
	onAir := fresh && room.LiveStatus == bilisource.RoomLive && !live.LastBroadcast.IsZero()
	indexes, starts := []int{}, []time.Time{}
	for i := range events {
		e := &events[i]
		if e.Platform == nil || *e.Platform != "bilibili" || e.EventType == nil || (*e.EventType != "stream" && *e.EventType != "member") {
			continue
		}
		e.LiveURL = &link
		if e.StartsAt == nil || e.CancellationStatus == "manual_confirmed" {
			continue
		}
		if start := parseInstant(*e.StartsAt); !start.IsZero() {
			indexes, starts = append(indexes, i), append(starts, start)
		}
	}
	match := live.matchBroadcast(starts)
	switch {
	case match >= 0 && onAir:
		status := "live"
		events[indexes[match]].LiveStatus = &status
	case match >= 0 && !live.EndedAt.IsZero():
		status := "ended"
		events[indexes[match]].LiveStatus = &status
	}
	if !onAir || match >= 0 {
		return nil, nil
	}
	matched, err := s.bilibiliBroadcastMatched(ctx, live)
	if err != nil || matched {
		return nil, err
	}
	return &LiveNow{Platform: "bilibili", URL: link, Title: room.Title, StartedAt: workflowTime(live.LastBroadcast)}, nil
}

// bilibiliBroadcastMatched reports whether any stored Bilibili stream, not
// only those in the read, is the room's current broadcast, so a page that
// reads a narrow window does not list an announced stream as unannounced.
func (s *Store) bilibiliBroadcastMatched(ctx context.Context, live BilibiliLive) (bool, error) {
	lb := live.LastBroadcast.UTC()
	var found int
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM events e WHERE `+bilibiliStreamEvents+`
 AND COALESCE(e.cancellation_status,'none')<>'manual_confirmed'
 AND julianday(e.starts_at)>=julianday(?) AND julianday(e.starts_at)<=julianday(?))`,
		lb.Add(-bilibiliMatchLate).Format(time.RFC3339Nano), lb.Add(bilibiliMatchEarly).Format(time.RFC3339Nano)).Scan(&found)
	if err != nil {
		return false, dbErr(err)
	}
	return found == 1, nil
}
