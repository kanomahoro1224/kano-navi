package sqlite

import (
	"context"
	"testing"
	"time"

	bilisource "github.com/yexca/kano-navi/backend/internal/source/bilibili"
)

// Rule B also moves a Bilibili dynamic's entry of a YouTube stream into the
// YouTube event, and Rule A's no-title fallback never hides a date-only
// entry of another platform behind a Bilibili stream.
func TestBilibiliEntriesInRulesAAndB(t *testing.T) {
	start := time.Date(2026, 11, 7, 11, 0, 0, 0, time.UTC)
	timed := func(id, source, platform string) mergeEvent {
		return mergeEvent{id: id, source: source, title: "雑談", startsOn: "2026-11-07", eventType: "stream", precision: "exact",
			provenance: "automatic", cancellation: "none", platform: platform, start: start, hasStart: true, startTimeSet: true}
	}
	desired, reasons, _ := planMerges([]mergeEvent{timed("bilibili-schedule-y", "bilibili", "youtube"), timed("youtube-AAAAAAAAAAA", "youtube", "youtube")})
	if desired["bilibili-schedule-y"] != "youtube-AAAAAAAAAAA" || reasons["bilibili-schedule-y"] != mergeReasonYoutube {
		t.Fatalf("rule b=%v %v", desired, reasons)
	}
	dateOnly := func(id, platform, title string) mergeEvent {
		return mergeEvent{id: id, source: "x", title: title, startsOn: "2026-11-07", eventType: "stream", precision: "unknown",
			provenance: "automatic", cancellation: "none", platform: platform}
	}
	limited := timed("bilibili-schedule-l", "bilibili", "bilibili")
	limited.title = "B站限定"
	if desired, _, _ = planMerges([]mergeEvent{limited, dateOnly("x-schedule-u", "unknown", "週間予定")}); len(desired) != 0 {
		t.Fatalf("unknown entry merged into a bilibili stream: %v", desired)
	}
	if desired, _, _ = planMerges([]mergeEvent{limited, dateOnly("x-schedule-b", "bilibili", "週間予定")}); desired["x-schedule-b"] != "bilibili-schedule-l" {
		t.Fatalf("bilibili entry was not merged: %v", desired)
	}
}

func TestReleasePairingIsOneToOneAndReleased(t *testing.T) {
	s := biliStore(t)
	ctx := context.Background()
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := s.DB().Exec(query, args...); err != nil {
			t.Fatal(err)
		}
	}
	text := func(query string, args ...any) string {
		t.Helper()
		var out string
		if err := s.DB().QueryRow(query, args...).Scan(&out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	release := func(id, start, opus string) {
		exec(`INSERT INTO events (id, source, source_item_id, source_key, title, starts_on, starts_at, event_type, url, platform, provenance, created_at, updated_at) VALUES (?, 'bilibili', ?, ?, '新歌', '2026-11-07', ?, 'release', ?, 'bilibili', 'automatic', ?, ?)`,
			id, "post-"+id, "k-"+id, start, opus, workflowTime(now), workflowTime(now))
		exec(`INSERT INTO event_sources (event_id, source, source_item_id, source_key, url, created_at, updated_at) VALUES (?, 'bilibili', ?, ?, ?, ?, ?)`,
			id, "post-"+id, "k-"+id, opus, workflowTime(now), workflowTime(now))
	}
	release("r1", "2026-11-07T09:00:00.000Z", "https://www.bilibili.com/opus/1")
	release("r2", "2026-11-07T09:10:00.000Z", "https://www.bilibili.com/opus/2")
	exec(`INSERT INTO videos (id, source, title, published_at, url, kind) VALUES ('BV1AAAAAAAAA', 'bilibili', '新歌', '2026-11-07T09:05:00Z', 'https://www.bilibili.com/video/BV1AAAAAAAAA', 'upload')`)
	// Two releases near one upload: neither is paired, run after run.
	for range 2 {
		if counts, err := s.ReconcileEventMerges(ctx, now); err != nil || counts.ReleasesPaired != 0 {
			t.Fatalf("counts=%#v err=%v", counts, err)
		}
	}
	if text(`SELECT COUNT(*) FROM event_sources WHERE source='bilibili-video'`) != "0" {
		t.Fatal("an ambiguous upload was paired")
	}
	// The second release moves away: the first pairs, once.
	exec(`UPDATE events SET starts_at='2026-11-07T11:00:00.000Z' WHERE id='r2'`)
	if counts, err := s.ReconcileEventMerges(ctx, now); err != nil || counts.ReleasesPaired != 1 {
		t.Fatalf("pair=%#v err=%v", counts, err)
	}
	if counts, err := s.ReconcileEventMerges(ctx, now); err != nil || counts.ReleasesPaired != 0 || counts.Changed() {
		t.Fatalf("unchanged run wrote: %#v err=%v", counts, err)
	}
	if text(`SELECT url FROM events WHERE id='r1'`) != "https://www.bilibili.com/video/BV1AAAAAAAAA" {
		t.Fatal("r1 not linked to its upload")
	}
	// The first release moves out of the window: its pairing is released.
	exec(`UPDATE events SET starts_at='2026-11-07T07:00:00.000Z' WHERE id='r1'`)
	if counts, err := s.ReconcileEventMerges(ctx, now); err != nil || counts.ReleasesPaired != 1 {
		t.Fatalf("release=%#v err=%v", counts, err)
	}
	if text(`SELECT url FROM events WHERE id='r1'`) != "https://www.bilibili.com/opus/1" ||
		text(`SELECT COUNT(*) FROM event_sources WHERE source='bilibili-video' AND deleted_at IS NULL`) != "0" {
		t.Fatal("released pairing kept its link or evidence")
	}
}

func TestLiveRoomWithoutAStartAndATitleChange(t *testing.T) {
	s := biliStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	live := bilisource.LiveState{Room: bilisource.Room{RoomID: "1000001", Title: "杂谈", LiveStatus: 1}, CheckedAt: now, LastAttemptAt: now}
	if _, err := s.WriteBilibiliLive(ctx, biliAccount, live); err != nil {
		t.Fatal(err)
	}
	// Live without a start is live from the check, never ended.
	stored, err := s.BilibiliLiveState(ctx, biliAccount)
	if err != nil || !stored.LastBroadcast.Equal(now) || !stored.EndedAt.IsZero() {
		t.Fatalf("stored=%#v err=%v", stored, err)
	}
	live.CheckedAt = now.Add(10 * time.Minute)
	if _, err = s.WriteBilibiliLive(ctx, biliAccount, live); err != nil {
		t.Fatal(err)
	}
	if stored, _ = s.BilibiliLiveState(ctx, biliAccount); !stored.LastBroadcast.Equal(now) || !stored.EndedAt.IsZero() || !stored.LastLiveAt.Equal(now.Add(10*time.Minute)) {
		t.Fatalf("second check=%#v", stored)
	}
	// A new title while live is public through liveNow, so it moves the
	// revision.
	live.Room.Title, live.CheckedAt = "歌回", now.Add(20*time.Minute)
	if changed, err := s.WriteBilibiliLive(ctx, biliAccount, live); err != nil || !changed {
		t.Fatalf("title change=%v err=%v", changed, err)
	}
	dashboard, err := s.GetDashboard(ctx, 3, now.Add(20*time.Minute))
	if err != nil || dashboard.LiveNow == nil || dashboard.LiveNow.Title != "歌回" {
		t.Fatalf("liveNow=%#v err=%v", dashboard.LiveNow, err)
	}
}

func TestDashboardNamesTheBilibiliAccounts(t *testing.T) {
	s := biliStore(t)
	ctx := context.Background()
	if _, err := s.DB().Exec(`INSERT INTO posts (id, source, account_handle, type, text, published_at, url) VALUES ('bilibili-1', 'bilibili', '316381099', 'daily', 't', '2026-11-07T00:00:00Z', 'https://www.bilibili.com/opus/1')`); err != nil {
		t.Fatal(err)
	}
	dashboard, err := s.GetDashboard(ctx, 3, time.Date(2026, 11, 7, 1, 0, 0, 0, time.UTC))
	if err != nil || len(dashboard.Meta.BilibiliAccounts) != 1 || dashboard.Meta.BilibiliAccounts[0] != biliAccount {
		t.Fatalf("accounts=%#v err=%v", dashboard.Meta.BilibiliAccounts, err)
	}
}
