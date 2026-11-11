package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
	"github.com/yexca/kano-navi/backend/internal/testutil"
)

func archiveHandler(t *testing.T, setup string) http.Handler {
	t.Helper()
	path := testutil.NewSnapshot(t)
	testutil.Exec(t, path, setup)
	store, err := sqlite.OpenReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return NewHandlerWithClock(store, func() time.Time { return time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC) })
}

func archiveIDs(t *testing.T, payload map[string]any) []string {
	t.Helper()
	ids := []string{}
	for _, item := range payload["items"].([]any) {
		ids = append(ids, item.(map[string]any)["id"].(string))
	}
	return ids
}

func TestPostArchiveReadsAJapanMonthNewestFirst(t *testing.T) {
	handler := archiveHandler(t, `
		INSERT INTO posts (id,source,account_handle,type,text,published_at,url,raw_json) VALUES
		 ('post-before','x','kano_2525','daily','September','2026-09-30T14:59:00Z','https://x.com/kano_2525/status/1','{"private":"synthetic"}'),
		 ('post-first','x','kano_2525','daily','First','2026-09-30T15:00:00Z','https://x.com/kano_2525/status/2','{"private":"synthetic"}'),
		 ('post-second','x','_Kanotic','daily','Second','2026-10-12T03:00:00+09:00','https://x.com/_Kanotic/status/3',NULL),
		 ('post-third','x','Kano_2525','daily','Third','2026-10-31T14:59:59Z','https://x.com/kano_2525/status/4',NULL),
		 ('post-after','x','_Kanotic','daily','November','2026-10-31T15:00:00Z','https://x.com/_Kanotic/status/5',NULL);
		INSERT INTO app_settings (key,value,updated_at) VALUES ('dashboard_revision','4','');`)

	recorder := request(t, handler, http.MethodGet, "/api/posts?from=2026-10-01&to=2026-10-31&pageSize=2")
	if recorder.Code != http.StatusOK || recorder.Header().Get("Cache-Control") != "no-cache" || !strings.HasPrefix(recorder.Header().Get("ETag"), `W/"`) {
		t.Fatalf("status=%d headers=%v body=%s", recorder.Code, recorder.Header(), recorder.Body)
	}
	if strings.Contains(recorder.Body.String(), "private") || strings.Contains(recorder.Body.String(), "raw") {
		t.Fatalf("archive leaked raw post fields: %s", recorder.Body)
	}
	payload := decodeObject(t, recorder)
	if got := strings.Join(archiveIDs(t, payload), ","); got != "post-third,post-second" {
		t.Fatalf("first page=%s", got)
	}
	if payload["total"] != float64(3) || payload["totalPages"] != float64(2) || payload["hasNext"] != true || payload["revision"] != float64(4) {
		t.Fatalf("paging=%#v", payload)
	}
	adjacent := payload["adjacent"].(map[string]any)
	if adjacent["previous"] != "2026-09-30" || adjacent["next"] != "2026-11-01" {
		t.Fatalf("adjacent=%#v", adjacent)
	}
	counts := payload["accountCounts"].(map[string]any)
	if counts["kano_2525"] != float64(2) || counts["_kanotic"] != float64(1) {
		t.Fatalf("account counts=%#v", counts)
	}
	first := payload["items"].([]any)[0].(map[string]any)
	if first["accountUrl"] != "https://x.com/Kano_2525" || first["text"] != "Third" {
		t.Fatalf("post projection=%#v", first)
	}

	second := decodeObject(t, request(t, handler, http.MethodGet, "/api/posts?from=2026-10-01&to=2026-10-31&pageSize=2&page=2"))
	if got := strings.Join(archiveIDs(t, second), ","); got != "post-first" || second["hasNext"] != false {
		t.Fatalf("second page=%s %#v", got, second)
	}

	// The account filter ignores case; counts and adjacent dates follow it.
	filtered := decodeObject(t, request(t, handler, http.MethodGet, "/api/posts?from=2026-10-01&to=2026-10-31&account=KANO_2525"))
	if got := strings.Join(archiveIDs(t, filtered), ","); got != "post-third,post-first" || filtered["total"] != float64(2) {
		t.Fatalf("filtered=%s %#v", got, filtered)
	}
	if adjacent := filtered["adjacent"].(map[string]any); adjacent["previous"] != "2026-09-30" || adjacent["next"] != nil {
		t.Fatalf("filtered adjacent=%#v", adjacent)
	}
	if filtered["accountCounts"].(map[string]any)["_kanotic"] != float64(1) {
		t.Fatalf("filtered counts=%#v", filtered["accountCounts"])
	}

	for path, code := range map[string]string{
		"/api/posts?from=2026-10-31&to=2026-10-01":                       "invalid_range",
		"/api/posts?from=2026-10-01":                                     "invalid_range",
		"/api/posts?from=2026-09-01&to=2026-11-30":                       "invalid_range",
		"/api/posts?from=2026-10-01&to=2026-10-31&account=kano%25":       "invalid_account",
		"/api/posts?from=2026-10-01&to=2026-10-31&account=a'%20OR%201=1": "invalid_account",
	} {
		bad := request(t, handler, http.MethodGet, path)
		if bad.Code != http.StatusBadRequest || bad.Body.String() != `{"error":"`+code+`"}` {
			t.Fatalf("%s: status=%d body=%s", path, bad.Code, bad.Body)
		}
	}
	if method := request(t, handler, http.MethodPost, "/api/posts"); method.Code != http.StatusMethodNotAllowed || method.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("method status=%d headers=%v", method.Code, method.Header())
	}
}

func TestPostMonthsCountJapanMonthsPerAccount(t *testing.T) {
	handler := archiveHandler(t, `
		INSERT INTO posts (id,source,account_handle,type,text,published_at,url,raw_json,hidden_at) VALUES
		 ('post-old','x','kano_2525','daily','Old','2024-01-10T03:00:00Z','https://x.com/kano_2525/status/1','{"private":"synthetic"}',NULL),
		 ('post-edge','x','Kano_2525','daily','Edge','2026-09-30T15:00:00Z','https://x.com/kano_2525/status/2',NULL,NULL),
		 ('post-other','x','_Kanotic','daily','Other','2026-10-12T03:00:00+09:00','https://x.com/_Kanotic/status/3',NULL,NULL),
		 ('post-september','x','_Kanotic','daily','September','2026-09-30T14:59:00Z','https://x.com/_Kanotic/status/4',NULL,NULL),
		 ('post-hidden','x','kano_2525','daily','Hidden','2025-05-05T03:00:00Z','https://x.com/kano_2525/status/5',NULL,'2026-10-01T00:00:00Z'),
		 ('post-undated','x','kano_2525','daily','Undated','not a time','https://x.com/kano_2525/status/6',NULL,NULL);
		INSERT INTO app_settings (key,value,updated_at) VALUES ('dashboard_revision','7','');`)

	recorder := request(t, handler, http.MethodGet, "/api/posts/months")
	if recorder.Code != http.StatusOK || recorder.Header().Get("Cache-Control") != "no-cache" || !strings.HasPrefix(recorder.Header().Get("ETag"), `W/"`) {
		t.Fatalf("status=%d headers=%v body=%s", recorder.Code, recorder.Header(), recorder.Body)
	}
	if strings.Contains(recorder.Body.String(), "private") || strings.Contains(recorder.Body.String(), "post-") {
		t.Fatalf("month index leaked post fields: %s", recorder.Body)
	}
	payload := decodeObject(t, recorder)
	if payload["revision"] != float64(7) {
		t.Fatalf("revision=%#v", payload["revision"])
	}
	// Japan months, oldest first; hidden and unparseable posts are not counted.
	months := payload["months"].([]any)
	got := []string{}
	for _, raw := range months {
		month := raw.(map[string]any)
		got = append(got, month["month"].(string))
	}
	if strings.Join(got, ",") != "2024-01,2026-09,2026-10" {
		t.Fatalf("months=%v", got)
	}
	october := months[2].(map[string]any)
	counts := october["accountCounts"].(map[string]any)
	if october["total"] != float64(2) || counts["kano_2525"] != float64(1) || counts["_kanotic"] != float64(1) {
		t.Fatalf("october=%#v", october)
	}
	if september := months[1].(map[string]any); september["total"] != float64(1) {
		t.Fatalf("september=%#v", september)
	}
	if method := request(t, handler, http.MethodPost, "/api/posts/months"); method.Code != http.StatusMethodNotAllowed || method.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("method status=%d headers=%v", method.Code, method.Header())
	}
}

func TestVideoArchiveUsesTheScheduledOrPublishedTime(t *testing.T) {
	handler := archiveHandler(t, `
		INSERT INTO videos (id,source,title,published_at,scheduled_at,url,kind,is_upcoming,raw_json,duration_seconds,is_short) VALUES
		 ('video-old','youtube','September','2026-09-20T12:00:00Z',NULL,'https://www.youtube.com/watch?v=old','video',0,'{"private":"synthetic"}',NULL,NULL),
		 ('video-upload','youtube','Upload','2026-10-05T12:00:00Z',NULL,'https://www.youtube.com/watch?v=upload','video',0,NULL,NULL,1),
		 ('video-stream','youtube','Stream','2026-09-28T12:00:00Z','2026-10-10T12:00:00Z','https://www.youtube.com/watch?v=stream','liveArchive',0,NULL,7265,0),
		 ('video-reserved','youtube','Reserved','2026-10-18T12:00:00Z','2026-10-25T12:00:00Z','https://www.youtube.com/watch?v=reserved','upcoming',1,NULL,NULL,NULL),
		 ('video-undated','youtube','Undated',NULL,NULL,'https://www.youtube.com/watch?v=undated','video',0,NULL,NULL,NULL);`)

	recorder := request(t, handler, http.MethodGet, "/api/videos?from=2026-10-01&to=2026-10-31")
	if recorder.Code != http.StatusOK || recorder.Header().Get("Cache-Control") != "no-cache" || !strings.HasPrefix(recorder.Header().Get("ETag"), `W/"`) {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body)
	}
	if strings.Contains(recorder.Body.String(), "private") {
		t.Fatalf("archive leaked raw video fields: %s", recorder.Body)
	}
	payload := decodeObject(t, recorder)
	if got := strings.Join(archiveIDs(t, payload), ","); got != "video-reserved,video-stream,video-upload" || payload["total"] != float64(3) || payload["hasNext"] != false {
		t.Fatalf("videos=%s %#v", got, payload)
	}
	items := payload["items"].([]any)
	if items[0].(map[string]any)["isUpcoming"] != true || items[1].(map[string]any)["isUpcoming"] != false {
		t.Fatalf("upcoming flags=%#v", items)
	}
	// A known length is public; an unknown one stays null.
	if length, ok := items[2].(map[string]any)["durationSeconds"]; items[1].(map[string]any)["durationSeconds"] != float64(7265) || !ok || length != nil {
		t.Fatalf("lengths=%#v", items)
	}
	// The feed's format is public; a row it never named stays null.
	if format, ok := items[0].(map[string]any)["isShort"]; !ok || format != nil || items[1].(map[string]any)["isShort"] != false || items[2].(map[string]any)["isShort"] != true {
		t.Fatalf("formats=%#v", items)
	}
	if adjacent := payload["adjacent"].(map[string]any); adjacent["previous"] != "2026-09-20" || adjacent["next"] != nil {
		t.Fatalf("adjacent=%#v", adjacent)
	}
	if _, ok := payload["accountCounts"]; ok {
		t.Fatalf("video archive carries post counts: %#v", payload)
	}
	if bad := request(t, handler, http.MethodGet, "/api/videos?from=2026-02-30&to=2026-03-01"); bad.Code != http.StatusBadRequest {
		t.Fatalf("invalid range status=%d", bad.Code)
	}
}

func TestVideoTimelinePagesThroughEveryMonth(t *testing.T) {
	handler := archiveHandler(t, `
		INSERT INTO videos (id,source,title,published_at,scheduled_at,url,kind,is_upcoming,raw_json,manual_tag) VALUES
		 ('video-2024','youtube','Old',     '2024-03-02T12:00:00Z',NULL,'https://www.youtube.com/watch?v=v2024','video',0,'{"private":"synthetic"}','song'),
		 ('video-edge','youtube','Edge',    '2026-09-30T15:00:00Z',NULL,'https://www.youtube.com/watch?v=edge','video',0,NULL,NULL),
		 ('video-sep', 'youtube','September','2026-09-30T14:59:00Z',NULL,'https://www.youtube.com/watch?v=sep','video',0,NULL,'game'),
		 ('video-oct', 'youtube','October', '2026-10-05T12:00:00Z',NULL,'https://www.youtube.com/watch?v=oct','video',0,NULL,'game'),
		 ('video-next','youtube','Reserved','2026-10-18T12:00:00Z','2026-12-01T12:00:00Z','https://www.youtube.com/watch?v=next','upcoming',1,NULL,NULL),
		 ('video-undated','youtube','Undated',NULL,NULL,'https://www.youtube.com/watch?v=undated','video',0,NULL,NULL),
		 ('video-bili','bilibili','Elsewhere','2026-10-06T12:00:00Z',NULL,'https://www.bilibili.com/video/BV1example','video',0,NULL,NULL);
		INSERT INTO app_settings (key,value,updated_at) VALUES ('dashboard_revision','9','');`)

	// Without a range the pages run newest first through every month, from
	// reservations ahead to the oldest video; undated and other-platform rows
	// are not listed.
	first := request(t, handler, http.MethodGet, "/api/videos?pageSize=3")
	if first.Code != http.StatusOK || first.Header().Get("Cache-Control") != "no-cache" || !strings.HasPrefix(first.Header().Get("ETag"), `W/"`) {
		t.Fatalf("status=%d headers=%v body=%s", first.Code, first.Header(), first.Body)
	}
	if strings.Contains(first.Body.String(), "private") {
		t.Fatalf("timeline leaked raw video fields: %s", first.Body)
	}
	payload := decodeObject(t, first)
	if got := strings.Join(archiveIDs(t, payload), ","); got != "video-next,video-oct,video-edge" {
		t.Fatalf("first page=%s", got)
	}
	if payload["total"] != float64(5) || payload["hasNext"] != true || payload["revision"] != float64(9) {
		t.Fatalf("paging=%#v", payload)
	}
	if adjacent := payload["adjacent"].(map[string]any); adjacent["previous"] != nil || adjacent["next"] != nil {
		t.Fatalf("open adjacent=%#v", adjacent)
	}
	counts := payload["categoryCounts"].(map[string]any)
	if counts["game"] != float64(2) || counts["song"] != float64(1) || counts["other"] != float64(2) {
		t.Fatalf("counts=%#v", counts)
	}
	second := decodeObject(t, request(t, handler, http.MethodGet, "/api/videos?pageSize=3&page=2"))
	if got := strings.Join(archiveIDs(t, second), ","); got != "video-sep,video-2024" || second["hasNext"] != false {
		t.Fatalf("second page=%s %#v", got, second)
	}

	// until ends the timeline on an inclusive Japan date, and the nearest
	// later video tells the board that newer months exist.
	september := decodeObject(t, request(t, handler, http.MethodGet, "/api/videos?until=2026-09-30"))
	if got := strings.Join(archiveIDs(t, september), ","); got != "video-sep,video-2024" {
		t.Fatalf("until=%s", got)
	}
	if adjacent := september["adjacent"].(map[string]any); adjacent["previous"] != nil || adjacent["next"] != "2026-10-01" {
		t.Fatalf("until adjacent=%#v", adjacent)
	}
	game := decodeObject(t, request(t, handler, http.MethodGet, "/api/videos?until=2026-10-31&category=game"))
	if got := strings.Join(archiveIDs(t, game), ","); got != "video-oct,video-sep" || game["categoryCounts"].(map[string]any)["other"] != float64(1) {
		t.Fatalf("game=%s %#v", got, game)
	}

	for path, code := range map[string]string{
		"/api/videos?until=2026-02-30":                               "invalid_range",
		"/api/videos?until=yesterday":                                "invalid_range",
		"/api/videos?from=2026-10-01&to=2026-10-31&until=2026-10-31": "invalid_range",
		"/api/videos?to=2026-10-31":                                  "invalid_range",
		"/api/videos?category=live":                                  "invalid_category",
	} {
		bad := request(t, handler, http.MethodGet, path)
		if bad.Code != http.StatusBadRequest || bad.Body.String() != `{"error":"`+code+`"}` {
			t.Fatalf("%s: status=%d body=%s", path, bad.Code, bad.Body)
		}
	}
}

func TestVideoMonthsCountJapanMonthsPerCategory(t *testing.T) {
	handler := archiveHandler(t, `
		INSERT INTO videos (id,source,title,published_at,scheduled_at,url,kind,raw_json,manual_tag) VALUES
		 ('video-old', 'youtube','Old',      '2024-03-02T12:00:00Z',NULL,'https://www.youtube.com/watch?v=old','video','{"private":"synthetic"}','song'),
		 ('video-edge','youtube','Edge',     '2026-09-30T15:00:00Z',NULL,'https://www.youtube.com/watch?v=edge','video',NULL,'game'),
		 ('video-oct', 'youtube','October',  '2026-10-05T12:00:00Z',NULL,'https://www.youtube.com/watch?v=oct','video',NULL,NULL),
		 ('video-moved','youtube','Scheduled','2026-08-01T12:00:00Z','2026-10-20T12:00:00Z','https://www.youtube.com/watch?v=moved','stream',NULL,'game'),
		 ('video-undated','youtube','Undated',NULL,NULL,'https://www.youtube.com/watch?v=undated','video',NULL,NULL),
		 ('video-bili','bilibili','Elsewhere','2025-01-06T12:00:00Z',NULL,'https://www.bilibili.com/video/BV1example','video',NULL,NULL);
		INSERT INTO app_settings (key,value,updated_at) VALUES ('dashboard_revision','3','');`)

	recorder := request(t, handler, http.MethodGet, "/api/videos/months")
	if recorder.Code != http.StatusOK || recorder.Header().Get("Cache-Control") != "no-cache" || !strings.HasPrefix(recorder.Header().Get("ETag"), `W/"`) {
		t.Fatalf("status=%d headers=%v body=%s", recorder.Code, recorder.Header(), recorder.Body)
	}
	if strings.Contains(recorder.Body.String(), "private") || strings.Contains(recorder.Body.String(), "video-") {
		t.Fatalf("month index leaked video fields: %s", recorder.Body)
	}
	payload := decodeObject(t, recorder)
	if payload["revision"] != float64(3) {
		t.Fatalf("revision=%#v", payload["revision"])
	}
	// The scheduled time decides the month; undated and other-platform rows
	// are not counted.
	months := payload["months"].([]any)
	got := []string{}
	for _, raw := range months {
		got = append(got, raw.(map[string]any)["month"].(string))
	}
	if strings.Join(got, ",") != "2024-03,2026-10" {
		t.Fatalf("months=%v", got)
	}
	october := months[1].(map[string]any)
	counts := october["categoryCounts"].(map[string]any)
	if october["total"] != float64(3) || counts["game"] != float64(2) || counts["other"] != float64(1) {
		t.Fatalf("october=%#v", october)
	}
	if _, ok := october["accountCounts"]; ok {
		t.Fatalf("video months carry post counts: %#v", october)
	}
	if method := request(t, handler, http.MethodPost, "/api/videos/months"); method.Code != http.StatusMethodNotAllowed || method.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("method status=%d headers=%v", method.Code, method.Header())
	}
}

func TestVideoArchiveFiltersByTag(t *testing.T) {
	// A video's manual tag comes first, then its schedule entry's label (a
	// merge target's first; live and video name a form), then its own
	// inferred tag; anything else is other.
	handler := archiveHandler(t, `
		INSERT INTO videos (id,source,title,published_at,url,kind,manual_tag,tag) VALUES
		 ('v-aug','youtube','August','2026-08-10T12:00:00Z','https://www.youtube.com/watch?v=aug','upload',NULL,'game'),
		 ('v-sep','youtube','September','2026-09-25T12:00:00Z','https://www.youtube.com/watch?v=sep','upload',NULL,NULL),
		 ('v-game','youtube','Game','2026-10-01T12:00:00Z','https://www.youtube.com/watch?v=game','stream',NULL,NULL),
		 ('v-linked','youtube','Linked','2026-10-02T12:00:00Z','https://www.youtube.com/watch?v=linked','stream',NULL,NULL),
		 ('v-manual','youtube','Manual tag','2026-10-03T12:00:00Z','https://www.youtube.com/watch?v=manual','stream','game',NULL),
		 ('v-talk','youtube','Inferred talk','2026-10-04T12:00:00Z','https://www.youtube.com/watch?v=talk','upload',NULL,'talk'),
		 ('v-label','youtube','Label over inference','2026-10-05T12:00:00Z','https://www.youtube.com/watch?v=label','premiere',NULL,'song'),
		 ('v-merged','youtube','Merged','2026-10-06T12:00:00Z','https://www.youtube.com/watch?v=merged','stream',NULL,NULL),
		 ('v-music','youtube','Form label','2026-10-07T12:00:00Z','https://www.youtube.com/watch?v=music','premiere',NULL,'song'),
		 ('v-live','youtube','Live label','2026-10-08T12:00:00Z','https://www.youtube.com/watch?v=live','stream',NULL,NULL),
		 ('v-deleted','youtube','Deleted entry','2026-10-09T12:00:00Z','https://www.youtube.com/watch?v=deleted','stream',NULL,NULL);
		INSERT INTO events (id,source,source_item_id,title,starts_on,tag,merged_into,deleted_at,created_at,updated_at) VALUES
		 ('youtube-v-game','youtube','v-game','Game','2026-10-01','game',NULL,NULL,'',''),
		 ('x-linked','x','post-1','Linked','2026-10-02','game',NULL,NULL,'',''),
		 ('youtube-v-manual','youtube','v-manual','Manual','2026-10-03','talk',NULL,NULL,'',''),
		 ('youtube-v-label','youtube','v-label','Label','2026-10-05','talk',NULL,NULL,'',''),
		 ('manual-song','manual',NULL,'Song','2026-10-06','song',NULL,NULL,'',''),
		 ('youtube-v-merged','youtube','v-merged','Merged','2026-10-06',NULL,'manual-song',NULL,'',''),
		 ('youtube-v-music','youtube','v-music','Music','2026-10-07','video',NULL,NULL,'',''),
		 ('youtube-v-live','youtube','v-live','Live','2026-10-08','live',NULL,NULL,'',''),
		 ('youtube-v-deleted','youtube','v-deleted','Deleted','2026-10-09','game',NULL,'2026-10-10T00:00:00Z','','');
		INSERT INTO event_sources (event_id,source,source_item_id,source_key,created_at,updated_at) VALUES
		 ('x-linked','youtube','v-linked','reservation','','');`)

	all := decodeObject(t, request(t, handler, http.MethodGet, "/api/videos?from=2026-10-01&to=2026-10-31"))
	counts := all["categoryCounts"].(map[string]any)
	if all["total"] != float64(9) || counts["game"] != float64(3) || counts["talk"] != float64(2) || counts["song"] != float64(2) || counts["other"] != float64(2) {
		t.Fatalf("total=%v counts=%#v", all["total"], counts)
	}
	if adjacent := all["adjacent"].(map[string]any); adjacent["previous"] != "2026-09-25" {
		t.Fatalf("adjacent=%#v", adjacent)
	}

	for category, want := range map[string]string{
		"game":  "v-manual,v-linked,v-game",
		"talk":  "v-label,v-talk",
		"song":  "v-music,v-merged",
		"other": "v-deleted,v-live",
	} {
		page := decodeObject(t, request(t, handler, http.MethodGet, "/api/videos?from=2026-10-01&to=2026-10-31&category="+category))
		if got := strings.Join(archiveIDs(t, page), ","); got != want {
			t.Fatalf("%s=%s want %s", category, got, want)
		}
		// The counts ignore the filter so every button keeps its total.
		if page["categoryCounts"].(map[string]any)["game"] != float64(3) {
			t.Fatalf("%s counts=%#v", category, page["categoryCounts"])
		}
	}
	// Adjacent dates follow the filter, so an empty month jumps to a match.
	game := decodeObject(t, request(t, handler, http.MethodGet, "/api/videos?from=2026-10-01&to=2026-10-31&category=game"))
	if adjacent := game["adjacent"].(map[string]any); adjacent["previous"] != "2026-08-10" || game["total"] != float64(3) {
		t.Fatalf("game adjacent=%#v total=%v", adjacent, game["total"])
	}
	// Each item carries its shown tag.
	if first := game["items"].([]any)[0].(map[string]any); first["tag"] != "game" {
		t.Fatalf("game item=%#v", first)
	}

	bad := request(t, handler, http.MethodGet, "/api/videos?from=2026-10-01&to=2026-10-31&category=live")
	if bad.Code != http.StatusBadRequest || !strings.Contains(bad.Body.String(), "invalid_category") {
		t.Fatalf("invalid category status=%d body=%s", bad.Code, bad.Body)
	}
}

func TestVideoArchiveValidatorFollowsTimeDerivedFields(t *testing.T) {
	path := testutil.NewSnapshot(t)
	testutil.Exec(t, path, `INSERT INTO videos (id,source,title,published_at,scheduled_at,url,kind,is_upcoming) VALUES
		 ('video-reserved','youtube','Reserved','2026-10-18T12:00:00Z','2026-10-25T12:00:00Z','https://www.youtube.com/watch?v=reserved','upcoming',1)`)
	store, err := sqlite.OpenReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	now := time.Date(2026, 10, 25, 11, 0, 0, 0, time.UTC)
	handler := NewHandlerWithClock(store, func() time.Time { return now })
	read := func(ifNoneMatch string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/videos?from=2026-10-01&to=2026-10-31", nil)
		if ifNoneMatch != "" {
			req.Header.Set("If-None-Match", ifNoneMatch)
		}
		handler.ServeHTTP(recorder, req)
		return recorder
	}
	before := read("")
	if revalidated := read(before.Header().Get("ETag")); revalidated.Code != http.StatusNotModified || revalidated.Body.Len() != 0 {
		t.Fatalf("unchanged archive: status=%d body=%s", revalidated.Code, revalidated.Body)
	}
	// The reservation's start passes without a stored write.
	now = now.Add(5 * time.Hour)
	after := read(before.Header().Get("ETag"))
	if after.Code != http.StatusOK || after.Header().Get("ETag") == before.Header().Get("ETag") ||
		!strings.Contains(before.Body.String(), `"isUpcoming":true`) || !strings.Contains(after.Body.String(), `"isUpcoming":false`) {
		t.Fatalf("expired reservation: status=%d etag=%s body=%s", after.Code, after.Header().Get("ETag"), after.Body)
	}
}
