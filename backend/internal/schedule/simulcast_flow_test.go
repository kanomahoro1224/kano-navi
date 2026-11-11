package schedule

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	bilisource "github.com/yexca/kano-navi/backend/internal/source/bilibili"
	ytsource "github.com/yexca/kano-navi/backend/internal/source/youtube"
	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
)

// simulcastFlow writes entries the way production does: posts go through
// extraction with a synthetic model answer, reservations through the
// YouTube writer, the room through the live check's writer, and merges
// through the reconcile step.
type simulcastFlow struct {
	t       *testing.T
	f       *fixture
	ctx     context.Context
	now     time.Time
	day     string
	replies map[string]string
}

func newSimulcastFlow(t *testing.T) *simulcastFlow {
	f := newFixture(t)
	f.addProvider("p-main", protocolChat, "https://provider.example.test/v1", "model-a", `["text"]`)
	f.queue("p-main")
	now := time.Now().UTC().Truncate(time.Second)
	s := &simulcastFlow{t: t, f: f, ctx: context.Background(), now: now,
		day: now.In(jst).AddDate(0, 0, 1).Format("2006-01-02"), replies: map[string]string{}}
	f.provider.respond = func(request recordedRequest) (int, string) {
		for marker, reply := range s.replies {
			if strings.Contains(request.body, marker) {
				return http.StatusOK, chatReply(reply)
			}
		}
		return http.StatusOK, chatReply(stringify(obj("classification", "not_schedule", "events", []any{},
			"confidence", 0.9, "evidence", "e", "reason", nil, "action", "none")))
	}
	return s
}

// at is a Japan clock on the flow's day (tomorrow).
func (s *simulcastFlow) at(clock string) time.Time {
	start, err := time.Parse(time.RFC3339, s.day+"T"+clock+":00+09:00")
	if err != nil {
		s.t.Fatal(err)
	}
	return start
}

func (s *simulcastFlow) reply(marker string, entries ...any) {
	s.replies[marker] = stringify(obj("classification", "schedule", "events", entries,
		"confidence", 0.9, "evidence", "synthetic evidence", "reason", nil, "action", "add"))
}

// xEntry is an X answer: the single platform the X schema asks for.
func (s *simulcastFlow) xEntry(title, clock, eventType, platform string) *object {
	var clockValue any
	if clock != "" {
		clockValue = clock
	}
	return obj("title", title, "detail", nil, "date", s.day, "time", clockValue, "endTime", nil, "endDate", nil,
		"timePrecision", "exact", "status", "scheduled", "eventType", eventType, "tag", nil, "url", nil,
		"platform", platform, "confidence", 0.9, "evidence", title)
}

// dynamicEntry is a Bilibili answer in Beijing time: 19:00 there is 20:00 in
// Japan.
func (s *simulcastFlow) dynamicEntry(title, beijingClock, eventType string, platforms ...any) *object {
	return obj("title", title, "detail", nil, "date", s.day, "time", beijingClock, "endTime", nil, "endDate", nil,
		"timePrecision", "exact", "status", "scheduled", "eventType", eventType, "tag", nil, "url", nil,
		"platforms", platforms, "timeZone", "Asia/Shanghai", "confidence", 0.9, "evidence", title)
}

func (s *simulcastFlow) xPost(id, text string, entries ...any) {
	s.f.addPost(id, "DAILY", text+" "+id, s.now.Add(-time.Hour).Format(time.RFC3339), "")
	s.reply(" "+id, entries...)
}

func (s *simulcastFlow) dynamic(id, text string, entries ...any) {
	s.f.addBilibiliPost(id, text+" "+id, s.now.Add(-time.Hour).Format(time.RFC3339))
	s.reply(" "+id, entries...)
}

// reservation writes a YouTube video and its event through the YouTube
// writer; status is the probe's broadcast state ("" when never checked).
func (s *simulcastFlow) reservation(id string, start time.Time, status string, end time.Time) {
	s.t.Helper()
	video := ytsource.Video{ID: id, Title: "歌枠 " + id, PublishedAt: s.now.Add(-48 * time.Hour).Format(time.RFC3339),
		ScheduledAt: start.UTC().Format(time.RFC3339), URL: "https://www.youtube.com/watch?v=" + id, Kind: "UPCOMING LIVE",
		IsUpcoming: status == "" || status == ytsource.LiveStatusUpcoming, LiveStatus: status, LiveNow: status == ytsource.LiveStatusLive}
	if !end.IsZero() {
		video.EndedAt, video.Kind = end.UTC().Format(time.RFC3339), "stream"
	}
	if err := s.f.store.WriteYoutubeRecent(s.ctx, "UCsynthetic", ytsource.Result{Videos: []ytsource.Video{video}, NextCursor: id,
		NextCursorTime: video.PublishedAt}); err != nil {
		s.t.Fatal(err)
	}
}

// room writes one live check of the account's room.
func (s *simulcastFlow) room(checked time.Time, live bool, started time.Time) {
	s.t.Helper()
	state := bilisource.LiveState{Room: bilisource.Room{RoomID: "1000001", Title: "synthetic room"}, CheckedAt: checked, LastAttemptAt: checked}
	if live {
		state.Room.LiveStatus, state.Room.LiveTime = bilisource.RoomLive, started.UTC().Format(time.RFC3339)
	}
	if _, err := s.f.store.WriteBilibiliLive(s.ctx, bilisource.DefaultAccount, state); err != nil {
		s.t.Fatal(err)
	}
}

func (s *simulcastFlow) settle() {
	s.t.Helper()
	s.f.scan()
	if _, err := s.f.store.ReconcileEventMerges(s.ctx, s.now); err != nil {
		s.t.Fatal(err)
	}
}

// eventOf is the stored event a post's extraction wrote.
func (s *simulcastFlow) eventOf(postID string) string {
	return s.f.text(`SELECT e.id FROM events e JOIN event_sources es ON es.event_id=e.id WHERE es.source_item_id=? AND es.deleted_at IS NULL`, postID)
}

// merge is "target|reason" of an event.
func (s *simulcastFlow) merge(id string) string {
	return s.f.text(`SELECT COALESCE(merged_into,'')||'|'||COALESCE(merge_reason,'') FROM events WHERE id=?`, id)
}

func (s *simulcastFlow) board(at time.Time) *sqlite.Dashboard {
	s.t.Helper()
	dashboard, err := s.f.store.GetDashboard(s.ctx, 3, at)
	if err != nil {
		s.t.Fatal(err)
	}
	return dashboard
}

func boardEvent(d *sqlite.Dashboard, id string) *sqlite.DashboardEvent {
	for i := range d.Events {
		if d.Events[i].ID == id {
			return &d.Events[i]
		}
	}
	return nil
}

func linkURLs(e *sqlite.DashboardEvent) map[string]string {
	out := map[string]string{}
	for _, link := range e.Links {
		if link.URL != nil {
			out[link.Platform] = *link.URL
		}
	}
	return out
}

// Item 1: a dynamic naming both platforms (stored youtube first) is the
// Bilibili side of the simulcast for Rule E only, and an X notice that
// Rule D merged into it ends on the YouTube event, never in a chain.
func TestADynamicNamingBothPlatformsMergesWithoutAChain(t *testing.T) {
	s := newSimulcastFlow(t)
	s.dynamic("900000000000000101", "明晚8点油管B站同时直播",
		s.dynamicEntry("同时直播", "19:00", "stream", "youtube", "bilibili"))
	s.xPost("1000000000000000101", "明日20時からbilibiliで配信します",
		s.xEntry("bilibili配信", "20:00", "stream", "bilibili"))
	s.reservation("SimulA00001", s.at("20:00"), ytsource.LiveStatusUpcoming, time.Time{})
	s.settle()
	dynamic, notice := s.eventOf("bilibili-900000000000000101"), s.eventOf("1000000000000000101")
	if got := s.f.text(`SELECT platform||' '||platforms||' '||platforms_inferred FROM events WHERE id=?`, dynamic); got != `youtube ["youtube","bilibili"] 0` {
		t.Fatalf("dynamic stored=%s", got)
	}
	if got := s.merge(dynamic); got != "youtube-SimulA00001|rule_e:simulcast" {
		t.Fatalf("dynamic merge=%s", got)
	}
	if got := s.merge(notice); got != "youtube-SimulA00001|rule_d:bilibili_start" {
		t.Fatalf("notice merge=%s", got)
	}
	card := boardEvent(s.board(s.now), "youtube-SimulA00001")
	if card == nil || !slices.Equal(card.Platforms, []string{"youtube", "bilibili"}) {
		t.Fatalf("card=%#v", card)
	}
}

// Item 2: simulcast wording keeps the model's platform first, so Rule D
// still merges the X notice into the dynamic, and the reservation later
// takes both; naming both sites without 同時 is no simulcast, and a
// collaboration elsewhere stays other.
func TestAnXSimulcastNoticeFollowsRuleDThenTheReservation(t *testing.T) {
	s := newSimulcastFlow(t)
	s.dynamic("900000000000000201", "明晚8点直播", s.dynamicEntry("杂谈", "19:00", "stream"))
	s.xPost("1000000000000000201", "明日20時からYouTubeとbilibiliで同時配信！",
		s.xEntry("YouTubeとbilibiliで同時配信", "20:00", "stream", "bilibili"))
	s.xPost("1000000000000000202", "明日21時からYouTubeで歌枠、bilibiliの切り抜きもよろしく",
		s.xEntry("YouTubeで歌枠、bilibiliの切り抜きも", "21:00", "stream", "youtube"))
	s.xPost("1000000000000000203", "明日22時からコラボ配信、YouTubeとbilibiliで同時配信",
		s.xEntry("コラボ YouTubeとbilibiliで同時配信", "22:00", "stream", "other"))
	s.settle()
	dynamic, notice := s.eventOf("bilibili-900000000000000201"), s.eventOf("1000000000000000201")
	if got := s.f.text(`SELECT platform||' '||platforms FROM events WHERE id=?`, notice); got != `bilibili ["bilibili","youtube"]` {
		t.Fatalf("notice stored=%s", got)
	}
	if got := s.merge(notice); got != dynamic+"|rule_d:bilibili_start" {
		t.Fatalf("notice merge=%s", got)
	}
	if got := s.f.text(`SELECT platforms FROM events WHERE id=?`, s.eventOf("1000000000000000202")); got != `["youtube"]` {
		t.Fatalf("mention=%s", got)
	}
	if got := s.f.text(`SELECT platforms FROM events WHERE id=?`, s.eventOf("1000000000000000203")); got != `["other"]` {
		t.Fatalf("collaboration=%s", got)
	}
	s.reservation("SimulB00001", s.at("20:00"), ytsource.LiveStatusUpcoming, time.Time{})
	s.settle()
	if s.merge(dynamic) != "youtube-SimulB00001|rule_e:simulcast" || !strings.HasPrefix(s.merge(notice), "youtube-SimulB00001|") {
		t.Fatalf("dynamic=%s notice=%s", s.merge(dynamic), s.merge(notice))
	}
}

// Item 6: a dynamic's plain B站 stream is Bilibili by default (inferred)
// and merges into the simultaneous YouTube stream; 「B站限定」 states it and
// stays apart.
func TestOnlyBilibiliOnlyWordingStatesBilibili(t *testing.T) {
	s := newSimulcastFlow(t)
	s.dynamic("900000000000000301", "明晚8点B站直播", s.dynamicEntry("B站直播", "19:00", "stream", "bilibili"))
	s.dynamic("900000000000000302", "明晚8点B站限定直播", s.dynamicEntry("B站限定直播", "19:00", "stream", "bilibili"))
	s.reservation("SimulC00001", s.at("20:00"), ytsource.LiveStatusUpcoming, time.Time{})
	s.settle()
	plain, only := s.eventOf("bilibili-900000000000000301"), s.eventOf("bilibili-900000000000000302")
	if got := s.f.text(`SELECT platforms_inferred FROM events WHERE id=?`, plain); got != "1" {
		t.Fatalf("plain inferred=%s", got)
	}
	if got := s.f.text(`SELECT platforms_inferred FROM events WHERE id=?`, only); got != "0" {
		t.Fatalf("only inferred=%s", got)
	}
	if s.merge(plain) != "youtube-SimulC00001|rule_e:simulcast" || s.merge(only) != "|" {
		t.Fatalf("plain=%s only=%s", s.merge(plain), s.merge(only))
	}
}

// Item 5: a Bilibili-only entry of the day never joins the day's YouTube
// stream through Rule A, and an entry Rule B merges lends no platform.
func TestBilibiliOnlyDateOnlyEntriesAndRuleBLendNothing(t *testing.T) {
	s := newSimulcastFlow(t)
	s.xPost("1000000000000000501", "明日はbilibili限定で歌枠", s.xEntry("歌枠", "", "stream", "unknown"))
	s.xPost("1000000000000000502", "明日20時からYouTubeとbilibiliで同時プレミア公開",
		s.xEntry("同時プレミア公開", "20:00", "release", "youtube"))
	s.reservation("SimulD00001", s.at("19:00"), ytsource.LiveStatusUpcoming, time.Time{})
	video := ytsource.Video{ID: "SimulD00002", Title: "MV", PublishedAt: s.now.Add(-48 * time.Hour).Format(time.RFC3339),
		ScheduledAt: s.at("20:00").UTC().Format(time.RFC3339), URL: "https://www.youtube.com/watch?v=SimulD00002",
		Kind: "UPCOMING LIVE", IsUpcoming: true, Premiere: true, PremiereKnown: true}
	if err := s.f.store.WriteYoutubeRecent(s.ctx, "UCsynthetic", ytsource.Result{Videos: []ytsource.Video{video}, NextCursor: "x", NextCursorTime: video.PublishedAt}); err != nil {
		t.Fatal(err)
	}
	s.settle()
	only, premiere := s.eventOf("1000000000000000501"), s.eventOf("1000000000000000502")
	if got := s.f.text(`SELECT platforms||' '||platforms_inferred FROM events WHERE id=?`, only); got != `["bilibili"] 0` {
		t.Fatalf("only stored=%s", got)
	}
	if s.merge(only) != "|" {
		t.Fatalf("bilibili-only merged by Rule A: %s", s.merge(only))
	}
	if !strings.HasPrefix(s.merge(premiere), "youtube-SimulD00002|rule_b") {
		t.Fatalf("premiere notice merge=%s", s.merge(premiere))
	}
	if card := boardEvent(s.board(s.now), "youtube-SimulD00002"); card == nil || !slices.Equal(card.Platforms, []string{"youtube"}) {
		t.Fatalf("premiere card=%#v", card)
	}
}

// Item 3: a YouTube reservation that has not started never takes the room's
// broadcast; once the probe sees it live, the broadcast is its simulcast
// (item 10: the card is live while either platform is).
func TestOnlyAStartedYoutubeBroadcastTakesTheRoom(t *testing.T) {
	s := newSimulcastFlow(t)
	start := s.now.Add(-10 * time.Minute)
	s.reservation("SimulE00001", start, ytsource.LiveStatusUpcoming, time.Time{})
	s.room(s.now, true, s.now.Add(-5*time.Minute))
	board := s.board(s.now)
	if card := boardEvent(board, "youtube-SimulE00001"); board.LiveNow == nil || card == nil || !slices.Equal(card.Platforms, []string{"youtube"}) {
		t.Fatalf("upcoming took the room: liveNow=%#v card=%#v", board.LiveNow, card)
	}
	s.reservation("SimulE00001", start, ytsource.LiveStatusLive, time.Time{})
	board = s.board(s.now)
	card := boardEvent(board, "youtube-SimulE00001")
	if board.LiveNow != nil || card == nil || !slices.Equal(card.Platforms, []string{"youtube", "bilibili"}) || card.LiveStatus == nil || *card.LiveStatus != "live" {
		t.Fatalf("live: liveNow=%#v card=%#v", board.LiveNow, card)
	}
	if links := linkURLs(card); links["youtube"] != "https://www.youtube.com/watch?v=SimulE00001" || links["bilibili"] != bilisource.RoomURL("1000001") {
		t.Fatalf("links=%v", links)
	}
}

// Item 4: a broadcast after the entry's own ended is not that entry's: a
// finished simulcast keeps its end, and a Bilibili-only entry whose
// broadcast the room saw end does not take the next one.
func TestALaterBroadcastIsNotTheFinishedEntrys(t *testing.T) {
	s := newSimulcastFlow(t)
	start := s.now.Add(-3 * time.Hour)
	s.dynamic("900000000000000401", "直播", obj("title", "杂谈", "detail", nil, "date", start.In(cst).Format("2006-01-02"),
		"time", start.In(cst).Format("15:04"), "endTime", nil, "endDate", nil, "timePrecision", "exact", "status", "scheduled",
		"eventType", "stream", "tag", nil, "url", nil, "platforms", []any{}, "timeZone", "Asia/Shanghai", "confidence", 0.9, "evidence", "杂谈"))
	s.reservation("SimulF00001", start, ytsource.LiveStatusEnded, s.now.Add(-2*time.Hour))
	s.settle()
	if got := s.merge(s.eventOf("bilibili-900000000000000401")); got != "youtube-SimulF00001|rule_e:simulcast" {
		t.Fatalf("simulcast merge=%s", got)
	}
	s.room(s.now, true, s.now.Add(-30*time.Minute))
	if board := s.board(s.now); board.LiveNow == nil {
		t.Fatalf("the finished simulcast took a later broadcast: %#v", boardEvent(board, "youtube-SimulF00001"))
	}

	o := newSimulcastFlow(t)
	first := o.now.Add(-3 * time.Hour)
	o.dynamic("900000000000000402", "B站限定直播", obj("title", "B站限定", "detail", nil, "date", first.In(cst).Format("2006-01-02"),
		"time", first.In(cst).Format("15:04"), "endTime", nil, "endDate", nil, "timePrecision", "exact", "status", "scheduled",
		"eventType", "stream", "tag", nil, "url", nil, "platforms", []any{"bilibili"}, "timeZone", "Asia/Shanghai", "confidence", 0.9, "evidence", "B站限定"))
	o.settle()
	id := o.eventOf("bilibili-900000000000000402")
	o.room(first.Add(10*time.Minute), true, first)
	if card := boardEvent(o.board(first.Add(10*time.Minute)), id); card == nil || card.LiveStatus == nil || *card.LiveStatus != "live" {
		t.Fatalf("first broadcast=%#v", card)
	}
	o.room(first.Add(2*time.Hour), false, time.Time{})
	o.room(o.now, true, o.now.Add(-20*time.Minute))
	board := o.board(o.now)
	if card := boardEvent(board, id); board.LiveNow == nil || card != nil && card.LiveStatus != nil && *card.LiveStatus == "live" {
		t.Fatalf("second broadcast: liveNow=%#v card=%#v", board.LiveNow, card)
	}
}

// Item 7: a platform link is a page on that platform. An X notice of a
// simulcast without a reservation has no YouTube link, and its post stays
// the entry's own url.
func TestPlatformLinksNeverNameTheAnnouncement(t *testing.T) {
	s := newSimulcastFlow(t)
	s.xPost("1000000000000000701", "明日20時からYouTubeとbilibiliで同時配信",
		s.xEntry("同時配信", "20:00", "stream", "youtube"))
	s.room(s.now, false, time.Time{})
	s.settle()
	card := boardEvent(s.board(s.now), s.eventOf("1000000000000000701"))
	if card == nil || card.URL == nil || !strings.Contains(*card.URL, "x.com") || !slices.Equal(card.Platforms, []string{"youtube", "bilibili"}) {
		t.Fatalf("card=%#v", card)
	}
	if links := linkURLs(card); len(links) != 1 || links["bilibili"] != bilisource.RoomURL("1000001") {
		t.Fatalf("links=%v", links)
	}
}

// Item 10: a manual entry an operator merged a live YouTube reservation into
// is live, with the live YouTube link.
func TestAMergedLiveReservationMakesTheEntryLive(t *testing.T) {
	s := newSimulcastFlow(t)
	start := s.now.Add(-10 * time.Minute)
	created, err := s.f.store.CreateManualEvent(s.ctx, sqlite.EventInput{Title: "雑談", StartsOn: start.In(jst).Format("2006-01-02"),
		StartsAt: sqlite.ISOTime(start), EventType: "stream", Platforms: []string{"bilibili"}, PlatformsSet: true})
	if err != nil {
		t.Fatal(err)
	}
	s.reservation("SimulG00001", start, ytsource.LiveStatusLive, time.Time{})
	if _, err = s.f.store.MergeEvent(s.ctx, "youtube-SimulG00001", created.ID); err != nil {
		t.Fatal(err)
	}
	card := boardEvent(s.board(s.now), created.ID)
	if card == nil || card.LiveStatus == nil || *card.LiveStatus != "live" || !slices.Equal(card.Platforms, []string{"bilibili", "youtube"}) {
		t.Fatalf("card=%#v", card)
	}
	for _, link := range card.Links {
		if link.Platform == "youtube" && (link.LiveStatus == nil || *link.LiveStatus != "live") {
			t.Fatalf("youtube link=%#v", link)
		}
	}
}
