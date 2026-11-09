package sqlite

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/yexca/kano-navi/backend/internal/works"
)

// recentIndex is a synthetic works list checked through 2026-06-30 that
// lists LISTEDVIDEO.
var recentIndex = works.Index{UpdatedAt: "2026-06-30", Uploads: []works.Upload{
	{ID: "2026-06-01-aaaaaa", Date: "2026-06-01", Videos: []works.Video{{Platform: works.YouTube, ID: "LISTEDVIDEO"}}},
}}

var recentNow = time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)

// recentVideo stores one synthetic video; extra is a list of column=value
// assignments applied after the insert.
func recentVideo(t *testing.T, s *Store, id, source, published, kind, tag string, extra ...string) {
	t.Helper()
	url := "https://www.youtube.com/watch?v=" + id
	if source == "bilibili" {
		url = "https://www.bilibili.com/video/" + id
	}
	if _, err := s.db.Exec(`INSERT INTO videos(id,source,title,published_at,url,kind,tag,tag_source,raw_json)
		VALUES(?,?,?,?,?,?,NULLIF(?,''),CASE WHEN ?<>'' THEN 'title' END,'{"synthetic":true}')`,
		id, source, "title "+id, published, url, kind, tag, tag); err != nil {
		t.Fatal(err)
	}
	if len(extra) > 0 {
		if _, err := s.db.Exec(`UPDATE videos SET `+strings.Join(extra, ",")+` WHERE id=?`, id); err != nil {
			t.Fatal(err)
		}
	}
}

func recentIDs(t *testing.T, s *Store) map[string]string {
	t.Helper()
	recent, _, err := s.worksRecentUploads(context.Background(), recentIndex, recentNow)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, item := range recent {
		out[item.ID] = item.Origin
	}
	return out
}

// Each rule keeps its videos out, and the rest are listed by tag, form and
// format or by an operator's decision.
func TestWorksRecentUploadsFollowEachRule(t *testing.T) {
	s := worksTestStore(t)
	const after = "2026-07-01T03:00:00.000Z"
	// Listed by tag, form and format.
	recentVideo(t, s, "SONGUPLOAD1", "youtube", after, "upload", "song")
	recentVideo(t, s, "AIREDPREMIE", "youtube", after, "premiere", "song", `scheduled_at='2026-07-05T12:00:00.000Z'`)
	recentVideo(t, s, "MANUALSONG1", "youtube", after, "upload", "game", `manual_tag='song'`)
	recentVideo(t, s, "MEMBERSSONG", "youtube", after, "upload", "song", `availability='members'`)
	recentVideo(t, s, "EDGEAFTER01", "youtube", "2026-06-30T15:00:00.000Z", "upload", "song")
	recentVideo(t, s, "BV1ONLYSONG", "bilibili", after, "upload", "song")
	// Rule 1: on or before the Japan date updatedAt.
	recentVideo(t, s, "EDGEOFDAY01", "youtube", "2026-06-30T14:59:59.000Z", "upload", "song")
	// Rule 2: gone, or still upcoming.
	recentVideo(t, s, "PRIVATESONG", "youtube", after, "upload", "song", `availability='private'`)
	recentVideo(t, s, "MANUALGONE1", "youtube", after, "upload", "song", `availability='public'`, `manual_availability='removed'`)
	recentVideo(t, s, "WAITINGPREM", "youtube", after, "premiere", "song", `scheduled_at='2026-07-20T12:00:00.000Z'`, `is_upcoming=1`)
	// Rule 3: a merged copy is its YouTube video's alternate.
	recentVideo(t, s, "BV1MERGEDCP", "bilibili", after, "upload", "song", `merged_into='SONGUPLOAD1'`)
	// Rule 4: a video of the packaged list.
	recentVideo(t, s, "LISTEDVIDEO", "youtube", after, "upload", "song")
	// Rule 5: not a song, a stream, a Short, or without a form.
	recentVideo(t, s, "GAMEUPLOAD1", "youtube", after, "upload", "game")
	recentVideo(t, s, "UNTAGGED001", "youtube", after, "upload", "")
	recentVideo(t, s, "SINGINGSTRM", "youtube", after, "stream", "song")
	recentVideo(t, s, "SONGSHORT01", "youtube", after, "upload", "song", `is_short=1`)
	recentVideo(t, s, "MANUALNOTSH", "youtube", after, "upload", "song", `is_short=1`, `manual_is_short=0`)
	recentVideo(t, s, "UNKNOWNFORM", "youtube", after, "unknown", "song")
	// Operator decisions: include past rule 5, exclude, and neither past rules 1 to 4.
	recentVideo(t, s, "INCLUDEDGAM", "youtube", after, "stream", "game", `manual_work=1`)
	recentVideo(t, s, "BV1INCLUDED", "bilibili", after, "upload", "", `manual_work=1`)
	recentVideo(t, s, "EXCLUDEDSNG", "youtube", after, "upload", "song", `manual_work=0`)
	recentVideo(t, s, "OLDINCLUDED", "youtube", "2026-06-01T03:00:00.000Z", "upload", "game", `manual_work=1`)
	recentVideo(t, s, "PRIVINCLUDE", "youtube", after, "upload", "song", `availability='private'`, `manual_work=1`)
	recentVideo(t, s, "BV1MERGEINC", "bilibili", after, "upload", "song", `merged_into='SONGUPLOAD1'`, `manual_work=1`)

	got := recentIDs(t, s)
	want := map[string]string{
		"SONGUPLOAD1": WorksOriginAuto, "AIREDPREMIE": WorksOriginAuto, "MANUALSONG1": WorksOriginAuto,
		"MEMBERSSONG": WorksOriginAuto, "EDGEAFTER01": WorksOriginAuto, "BV1ONLYSONG": WorksOriginAuto,
		"MANUALNOTSH": WorksOriginAuto, "INCLUDEDGAM": WorksOriginOperator, "BV1INCLUDED": WorksOriginOperator,
	}
	if len(got) != len(want) {
		t.Errorf("recent=%v", got)
	}
	for id, origin := range want {
		if got[id] != origin {
			t.Errorf("%s origin=%q, want %q", id, got[id], origin)
		}
	}

	recent, _, err := s.worksRecentUploads(context.Background(), recentIndex, recentNow)
	if err != nil {
		t.Fatal(err)
	}
	// Newest first, by a premiere's scheduled time.
	if recent[0].ID != "AIREDPREMIE" {
		t.Errorf("first=%s", recent[0].ID)
	}
	for _, item := range recent {
		switch item.ID {
		case "SONGUPLOAD1":
			if len(item.Alternates) != 2 || item.Alternates[0].Source != "bilibili" {
				t.Errorf("alternates=%+v", item.Alternates)
			}
		case "BV1ONLYSONG":
			if item.Source != "bilibili" || item.URL != "https://www.bilibili.com/video/BV1ONLYSONG" || len(item.Alternates) != 0 {
				t.Errorf("bilibili item=%+v", item)
			}
		}
	}

	// A list without a valid date lists nothing.
	if rows, err := s.worksRecentRows(context.Background(), works.Index{}, recentNow); err != nil || len(rows) != 0 {
		t.Fatalf("rows=%v err=%v", rows, err)
	}
	// A release that moves the date drops the earlier videos without cleanup.
	moved := works.Index{UpdatedAt: "2026-07-01"}
	if recent, _, err = s.worksRecentUploads(context.Background(), moved, recentNow); err != nil || len(recent) != 1 || recent[0].ID != "AIREDPREMIE" {
		t.Fatalf("after the move recent=%+v err=%v", recent, err)
	}
}

// The console lists the listed videos with their origin and the excluded
// ones, and a decision moves the dashboard revision.
func TestWorksDecisionIncludesExcludesAndClears(t *testing.T) {
	s := worksTestStore(t)
	ctx := context.Background()
	const after = "2026-07-01T03:00:00.000Z"
	recentVideo(t, s, "SONGUPLOAD1", "youtube", after, "upload", "song")
	recentVideo(t, s, "TRAILERSONG", "youtube", after, "upload", "song")
	recentVideo(t, s, "MISSEDSONG1", "youtube", after, "upload", "")
	revision := func() int64 {
		t.Helper()
		value, err := s.GetDashboardRevision(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	set := func(id string, work *bool) AdminVideo {
		t.Helper()
		video, err := s.SetVideoOverrides(ctx, id, VideoOverrides{SetWork: true, Work: work}, recentNow)
		if err != nil {
			t.Fatal(err)
		}
		return video
	}
	start := revision()
	if video := set("TRAILERSONG", ptr(false)); video.ManualWork == nil || *video.ManualWork {
		t.Fatalf("manualWork=%v", video.ManualWork)
	}
	if video := set("MISSEDSONG1", ptr(true)); video.ManualWork == nil || !*video.ManualWork {
		t.Fatalf("manualWork=%v", video.ManualWork)
	}
	if revision() != start+2 {
		t.Fatalf("revision=%d, want %d", revision(), start+2)
	}
	// Setting the same decision again changes nothing.
	set("MISSEDSONG1", ptr(true))
	if revision() != start+2 {
		t.Fatalf("an unchanged decision moved the revision")
	}
	if got := recentIDs(t, s); len(got) != 2 || got["SONGUPLOAD1"] != WorksOriginAuto || got["MISSEDSONG1"] != WorksOriginOperator {
		t.Fatalf("recent=%v", got)
	}
	report, err := s.WorksRecentReport(ctx, recentIndex, recentNow)
	if err != nil {
		t.Fatal(err)
	}
	states := []string{}
	for _, item := range report.Items {
		state := "excluded"
		if item.Origin != nil {
			state = *item.Origin
		}
		states = append(states, item.ID+"="+state)
	}
	slices.Sort(states)
	if report.UpdatedAt != "2026-06-30" || strings.Join(states, ",") != "MISSEDSONG1=operator,SONGUPLOAD1=auto,TRAILERSONG=excluded" {
		t.Fatalf("report=%+v states=%v", report, states)
	}
	// Clearing hands both back to the rules.
	set("TRAILERSONG", nil)
	set("MISSEDSONG1", nil)
	if got := recentIDs(t, s); len(got) != 2 || got["TRAILERSONG"] != WorksOriginAuto || got["MISSEDSONG1"] != "" {
		t.Fatalf("recent after clearing=%v", got)
	}
}

// A recent upload's cover is its ready cached thumbnail; one still on its
// way has none.
func TestWorksRecentCoverIsTheCachedThumbnail(t *testing.T) {
	s := worksTestStore(t)
	content := []byte("synthetic thumbnail")
	hash := sha256Hex(string(content))
	cache := "youtube/sha256/" + hash + ".jpg"
	if err := os.MkdirAll(filepath.Join(s.dataRoot, "youtube", "sha256"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.dataRoot, filepath.FromSlash(cache)), content, 0o644); err != nil {
		t.Fatal(err)
	}
	const after = "2026-07-01T03:00:00.000Z"
	thumb := func(id string) string { return "https://i.ytimg.com/vi/" + id + "/hqdefault.jpg" }
	for _, id := range []string{"CACHEDSONG1", "PENDINGSONG"} {
		recentVideo(t, s, id, "youtube", after, "upload", "song", `thumbnail_url='`+thumb(id)+`'`)
	}
	if _, err := s.db.Exec(`INSERT INTO media_assets(id,source,source_url,status,sha256,cache_path,width,height,created_at,updated_at) VALUES(?,'youtube',?,'ready',?,?,480,360,'2026-07-01T00:00:00Z','2026-07-01T00:00:00Z')`,
		sha256Hex(thumb("CACHEDSONG1")), thumb("CACHEDSONG1"), hash, cache); err != nil {
		t.Fatal(err)
	}
	recent, _, err := s.worksRecentUploads(context.Background(), recentIndex, recentNow)
	if err != nil || len(recent) != 2 {
		t.Fatalf("recent=%+v err=%v", recent, err)
	}
	for _, item := range recent {
		switch item.ID {
		case "CACHEDSONG1":
			if item.Cover == nil || item.Cover.URL != "/media/"+sha256Hex(thumb("CACHEDSONG1"))+"?v="+hash || item.Cover.Width == nil || *item.Cover.Width != 480 {
				t.Errorf("cover=%+v", item.Cover)
			}
		case "PENDINGSONG":
			if item.Cover != nil {
				t.Errorf("pending cover=%+v", item.Cover)
			}
		}
	}
}

// The title inference asks about unmerged Bilibili uploads after the works
// list's date that nobody marked gone, and about no other Bilibili upload.
func TestVideoTagCandidatesAskAboutRecentBilibiliUploadsOnly(t *testing.T) {
	s := worksTestStore(t)
	const after = "2026-07-01T03:00:00.000Z"
	recentVideo(t, s, "YOUTUBEVID1", "youtube", "2026-06-01T03:00:00.000Z", "upload", "")
	recentVideo(t, s, "BV1RECENT01", "bilibili", after, "upload", "")
	recentVideo(t, s, "BV1OLDUPLD1", "bilibili", "2026-06-30T14:00:00.000Z", "upload", "")
	recentVideo(t, s, "BV1MERGED01", "bilibili", after, "upload", "", `merged_into='YOUTUBEVID1'`)
	recentVideo(t, s, "BV1REMOVED1", "bilibili", after, "upload", "", `manual_availability='removed'`)
	recentVideo(t, s, "BV1TAGGED01", "bilibili", after, "upload", "song")
	asked := func(updatedAt string) string {
		t.Helper()
		items, err := s.ListVideoTagCandidates(context.Background(), "2026-07-01", updatedAt, recentNow, 20)
		if err != nil {
			t.Fatal(err)
		}
		ids := []string{}
		for _, item := range items {
			ids = append(ids, item.Source+":"+item.ID)
		}
		slices.Sort(ids)
		return strings.Join(ids, ",")
	}
	if got := asked("2026-06-30"); got != "bilibili:BV1RECENT01,youtube:YOUTUBEVID1" {
		t.Fatalf("candidates=%q", got)
	}
	if got := asked(""); got != "youtube:YOUTUBEVID1" {
		t.Fatalf("without a date candidates=%q", got)
	}
}

// The Works page reads the newest WorksRecentLimit uploads and the console
// the newest WorksRecentReportLimit videos, each with how many there are.
func TestWorksRecentReadsAreBounded(t *testing.T) {
	s := worksTestStore(t)
	start := time.Date(2026, 7, 1, 3, 0, 0, 0, time.UTC)
	tx, err := s.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	for i := range WorksRecentReportLimit + 5 {
		id := fmt.Sprintf("SONG%07d", i)
		if _, err = tx.Exec(`INSERT INTO videos(id,source,title,published_at,url,kind,tag,tag_source) VALUES(?,'youtube',?,?,?,'upload','song','title')`,
			id, id, ISOTime(start.Add(time.Duration(i)*time.Hour)), "https://www.youtube.com/watch?v="+id); err != nil {
			t.Fatal(err)
		}
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	now := start.AddDate(0, 1, 0)
	recent, total, err := s.worksRecentUploads(context.Background(), recentIndex, now)
	newest := fmt.Sprintf("SONG%07d", WorksRecentReportLimit+4)
	if err != nil || len(recent) != WorksRecentLimit || total != WorksRecentReportLimit+5 || recent[0].ID != newest {
		t.Fatalf("recent=%d total=%d first=%v err=%v", len(recent), total, recent[0].ID, err)
	}
	report, err := s.WorksRecentReport(context.Background(), recentIndex, now)
	if err != nil || len(report.Items) != WorksRecentReportLimit || report.Total != WorksRecentReportLimit+5 || report.Items[0].ID != newest {
		t.Fatalf("report items=%d total=%d err=%v", len(report.Items), report.Total, err)
	}
}
