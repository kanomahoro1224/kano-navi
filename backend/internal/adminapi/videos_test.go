package adminapi

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/yexca/kano-navi/backend/internal/works"
)

func TestVideoCorrectionsHistoryAndRefresh(t *testing.T) {
	c := newConsole(t, "development", "", &gatedSteps{})
	c.exec(t, `INSERT INTO videos(id,source,title,published_at,url,kind) VALUES('AAAAAAAAAAA','youtube','Song cover premiere','2026-03-01T09:00:00Z','https://www.youtube.com/watch?v=AAAAAAAAAAA','upload')`)
	c.exec(t, `INSERT INTO video_revisions(video_id,origin,title,replaced_at) VALUES('AAAAAAAAAAA','sync','Earlier title','2026-03-01T10:00:00Z')`)

	code, payload := c.call(t, "PUT", "/api/admin/videos/AAAAAAAAAAA", `{"title":"Song cover","kind":"Premiere","tag":"song"}`)
	video, _ := payload["video"].(map[string]any)
	if code != 200 || video["title"] != "Song cover" || video["sourceTitle"] != "Song cover premiere" || video["kind"] != "premiere" || video["tag"] != "song" || video["manualTag"] != "song" {
		t.Fatalf("update=%d %v", code, payload)
	}
	code, payload = c.call(t, "PUT", "/api/admin/videos/AAAAAAAAAAA", `{"kind":null,"tag":null}`)
	video, _ = payload["video"].(map[string]any)
	if code != 200 || video["title"] != "Song cover" || video["kind"] != "upload" || video["tag"] != nil {
		t.Fatalf("clear kind=%d %v", code, payload)
	}
	// A format correction lies over the feed's, which sync keeps writing.
	c.exec(t, `UPDATE videos SET is_short=0 WHERE id='AAAAAAAAAAA'`)
	code, payload = c.call(t, "PUT", "/api/admin/videos/AAAAAAAAAAA", `{"isShort":true}`)
	video, _ = payload["video"].(map[string]any)
	if code != 200 || video["isShort"] != true || video["sourceIsShort"] != false || video["manualIsShort"] != true || video["title"] != "Song cover" {
		t.Fatalf("mark short=%d %v", code, payload)
	}
	code, payload = c.call(t, "PUT", "/api/admin/videos/AAAAAAAAAAA", `{"isShort":null}`)
	video, _ = payload["video"].(map[string]any)
	if code != 200 || video["isShort"] != false || video["manualIsShort"] != nil {
		t.Fatalf("clear short=%d %v", code, payload)
	}
	for body, want := range map[string]int{
		`{"title":""}`:                       400,
		`{"kind":"SHORTS"}`:                  400,
		`{"kind":"LIVE ARCHIVE"}`:            400,
		`{"tag":"live"}`:                     400,
		`{"isShort":"yes"}`:                  400,
		`{"thumbnail":{"mediaId":"x"}}`:      400,
		`{"description":"not a correction"}`: 400,
		`{"title":7}`:                        400,
	} {
		if code, payload = c.call(t, "PUT", "/api/admin/videos/AAAAAAAAAAA", body); code != want {
			t.Fatalf("%s=%d %v", body, code, payload)
		}
	}
	if code, _ = c.call(t, "PUT", "/api/admin/videos/missing", `{"title":"x"}`); code != 404 {
		t.Fatalf("missing video=%d", code)
	}

	code, payload = c.call(t, "GET", "/api/admin/videos/AAAAAAAAAAA/history", "")
	revisions, _ := payload["revisions"].([]any)
	if code != 200 || len(revisions) != 1 || revisions[0].(map[string]any)["title"] != "Earlier title" {
		t.Fatalf("history=%d %v", code, payload)
	}

	code, payload = c.call(t, "POST", "/api/admin/videos/AAAAAAAAAAA/refresh", "")
	if code != 202 || payload["started"] != true {
		t.Fatalf("refresh=%d %v", code, payload)
	}
	if got := c.text(t, `SELECT refresh_requested_at FROM videos WHERE id='AAAAAAAAAAA'`); got == "<null>" {
		t.Fatal("refresh request was not recorded")
	}
	run := c.waitForRun(t, runID(t, payload), "completed")
	if fmt.Sprint(run["steps"]) != "[youtube media reconcile]" {
		t.Fatalf("run=%v", run)
	}
}

func TestVideoMergeRoutes(t *testing.T) {
	c := newConsole(t, "development", "", &gatedSteps{})
	c.exec(t, `INSERT INTO videos(id,source,title,scheduled_at,published_at,url,kind,duration_seconds) VALUES('e6acfMolexU','youtube','奏（スキマスイッチ） - 鹿乃まほろ cover.','2026-10-24T09:00:09Z','2026-10-20T12:00:00Z','https://www.youtube.com/watch?v=e6acfMolexU','MUSIC',289)`)
	c.exec(t, `INSERT INTO videos(id,source,title,published_at,url,kind,duration_seconds) VALUES('BV1CmHw6eEGE','bilibili','鹿乃 翻唱《奏》','2026-10-24T09:05:00Z','https://www.bilibili.com/video/BV1CmHw6eEGE','MUSIC',289)`)
	c.exec(t, `INSERT INTO videos(id,source,title,published_at,url,kind) VALUES('BV1other00000','bilibili','鹿乃 翻唱《春》','2026-08-01T00:00:00Z','https://www.bilibili.com/video/BV1other00000','MUSIC')`)

	code, payload := c.call(t, "GET", "/api/admin/videos/merges", "")
	items, _ := payload["items"].([]any)
	if code != 200 || len(items) != 1 {
		t.Fatalf("suggestions=%d %v", code, payload)
	}
	item := items[0].(map[string]any)
	candidates, _ := item["candidates"].([]any)
	if item["status"] != "rule" || item["bilibili"].(map[string]any)["id"] != "BV1CmHw6eEGE" || len(candidates) != 1 ||
		candidates[0].(map[string]any)["youtube"].(map[string]any)["id"] != "e6acfMolexU" || candidates[0].(map[string]any)["complete"] != true {
		t.Fatalf("suggestion=%v", item)
	}

	code, payload = c.call(t, "POST", "/api/admin/videos/BV1CmHw6eEGE/merge", `{"into":"https://youtu.be/e6acfMolexU"}`)
	video, _ := payload["video"].(map[string]any)
	if code != 200 || video["mergedInto"] != "e6acfMolexU" || video["mergeSource"] != "manual" || video["mergedIntoTitle"] != "奏（スキマスイッチ） - 鹿乃まほろ cover." {
		t.Fatalf("merge=%d %v", code, payload)
	}
	code, payload = c.call(t, "GET", "/api/admin/videos?source=youtube", "")
	listed, _ := payload["items"].([]any)
	if code != 200 || len(listed) != 1 {
		t.Fatalf("videos=%d %v", code, payload)
	}
	alternates, _ := listed[0].(map[string]any)["alternates"].([]any)
	if len(alternates) != 1 || alternates[0].(map[string]any)["id"] != "BV1CmHw6eEGE" || alternates[0].(map[string]any)["url"] != "https://www.bilibili.com/video/BV1CmHw6eEGE" {
		t.Fatalf("alternates=%v", listed[0])
	}
	if code, payload = c.call(t, "POST", "/api/admin/videos/BV1other00000/merge", `{"into":"e6acfMolexU"}`); code != 409 || payload["error"] != "merge_conflict" {
		t.Fatalf("conflict=%d %v", code, payload)
	}
	for body, want := range map[string]int{`{}`: 400, `{"into":7}`: 400, `{"into":"https://media.example.test/v"}`: 400, `{"into":"BV1other00000"}`: 400} {
		if code, payload = c.call(t, "POST", "/api/admin/videos/BV1CmHw6eEGE/merge", body); code != want {
			t.Fatalf("%s=%d %v", body, code, payload)
		}
	}
	if code, _ = c.call(t, "POST", "/api/admin/videos/missing/merge", `{"into":"e6acfMolexU"}`); code != 404 {
		t.Fatalf("missing upload=%d", code)
	}

	code, payload = c.call(t, "POST", "/api/admin/videos/BV1other00000/separate", "")
	video, _ = payload["video"].(map[string]any)
	if code != 200 || video["mergedInto"] != nil || video["mergeSource"] != "manual" || video["mergeReason"] != "kept_separate" {
		t.Fatalf("keep separate=%d %v", code, payload)
	}
	for path, want := range map[string]int{"BV1CmHw6eEGE": 409, "e6acfMolexU": 400, "missing": 404} {
		if code, payload = c.call(t, "POST", "/api/admin/videos/"+path+"/separate", ""); code != want {
			t.Fatalf("separate %s=%d %v", path, code, payload)
		}
	}

	code, payload = c.call(t, "POST", "/api/admin/videos/BV1CmHw6eEGE/unmerge", "")
	video, _ = payload["video"].(map[string]any)
	if code != 200 || video["mergedInto"] != nil || video["mergeSource"] != "manual" || video["mergeReason"] != "unmerged_from:e6acfMolexU" {
		t.Fatalf("unmerge=%d %v", code, payload)
	}
	if code, payload = c.call(t, "POST", "/api/admin/videos/BV1CmHw6eEGE/unmerge", ""); code != 400 {
		t.Fatalf("second unmerge=%d %v", code, payload)
	}
	code, payload = c.call(t, "GET", "/api/admin/videos/BV1CmHw6eEGE/merges", "")
	items, _ = payload["items"].([]any)
	if code != 200 || len(items) != 1 || items[0].(map[string]any)["status"] != "separate" {
		t.Fatalf("separate=%d %v", code, payload)
	}
	if got := c.text(t, `SELECT COUNT(*) FROM translations WHERE owner_id='e6acfMolexU' AND platform='bilibili'`); got != "0" {
		t.Fatalf("undone merge kept %s translations", got)
	}
}

// An operator's availability lies over what the checks record, which sync
// keeps writing beneath it, and the board reads the correction.
func TestVideoAvailabilityCorrection(t *testing.T) {
	c := newConsole(t, "development", "", &gatedSteps{})
	c.exec(t, `INSERT INTO videos(id,source,title,published_at,url,kind,availability) VALUES('AAAAAAAAAAA','youtube','Archive','2026-03-01T09:00:00Z','https://www.youtube.com/watch?v=AAAAAAAAAAA','upload','public')`)
	code, payload := c.call(t, "PUT", "/api/admin/videos/AAAAAAAAAAA", `{"availability":" Private "}`)
	video, _ := payload["video"].(map[string]any)
	if code != 200 || video["availability"] != "private" || video["sourceAvailability"] != "public" || video["manualAvailability"] != "private" {
		t.Fatalf("set=%d %v", code, payload)
	}
	board, err := c.store.GetDashboard(context.Background(), 3, time.Date(2026, 3, 2, 3, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if board.Summary.LatestVideo != nil || len(board.Videos) != 1 || *board.Videos[0].Availability != "private" {
		t.Fatalf("board latest=%v videos=%v", board.Summary.LatestVideo, board.Videos)
	}
	c.exec(t, `UPDATE videos SET availability='unlisted' WHERE id='AAAAAAAAAAA'`)
	code, payload = c.call(t, "PUT", "/api/admin/videos/AAAAAAAAAAA", `{"availability":null}`)
	video, _ = payload["video"].(map[string]any)
	if code != 200 || video["availability"] != "unlisted" || video["manualAvailability"] != nil {
		t.Fatalf("clear=%d %v", code, payload)
	}
	for _, body := range []string{`{"availability":"hidden"}`, `{"availability":true}`} {
		if code, payload = c.call(t, "PUT", "/api/admin/videos/AAAAAAAAAAA", body); code != 400 {
			t.Fatalf("%s=%d %v", body, code, payload)
		}
	}
}

// The console lists the Works page's recent uploads with their origin and
// the excluded ones, and sets or clears the decision per video.
func TestWorksRecentDecisionRoutes(t *testing.T) {
	c := newConsole(t, "development", "", &gatedSteps{})
	updated, err := time.Parse("2006-01-02", works.Packaged().UpdatedAt)
	if err != nil {
		t.Fatal(err)
	}
	at := updated.AddDate(0, 0, 2).Format("2006-01-02") + "T03:00:00.000Z"
	c.exec(t, `INSERT INTO videos(id,source,title,published_at,url,kind,tag,tag_source) VALUES
		('SONGUPLOAD1','youtube','Song','`+at+`','https://www.youtube.com/watch?v=SONGUPLOAD1','upload','song','title'),
		('BV1ONLYUPLOAD','bilibili','Bilibili only','`+at+`','https://www.bilibili.com/video/BV1ONLYUPLOAD','upload',NULL,NULL)`)
	listed := func() string {
		t.Helper()
		code, payload := c.call(t, "GET", "/api/admin/videos/works-recent", "")
		items, _ := payload["items"].([]any)
		if code != 200 || payload["updatedAt"] != works.Packaged().UpdatedAt {
			t.Fatalf("list=%d %v", code, payload)
		}
		out := []string{}
		for _, raw := range items {
			item := raw.(map[string]any)
			out = append(out, fmt.Sprintf("%s=%v/%v", item["id"], item["origin"], item["work"]))
		}
		return strings.Join(out, ",")
	}
	if got := listed(); got != "SONGUPLOAD1=auto/<nil>" {
		t.Fatalf("before=%s", got)
	}
	for id, body := range map[string]string{"SONGUPLOAD1": `{"work":false}`, "BV1ONLYUPLOAD": `{"work":true}`} {
		code, payload := c.call(t, "PUT", "/api/admin/videos/"+id, body)
		if video, _ := payload["video"].(map[string]any); code != 200 || video["manualWork"] == nil {
			t.Fatalf("%s %s=%d %v", id, body, code, payload)
		}
	}
	if got := listed(); got != "BV1ONLYUPLOAD=operator/true,SONGUPLOAD1=<nil>/false" {
		t.Fatalf("after=%s", got)
	}
	if code, payload := c.call(t, "PUT", "/api/admin/videos/SONGUPLOAD1", `{"work":null}`); code != 200 || payload["video"].(map[string]any)["manualWork"] != nil {
		t.Fatalf("clear=%d %v", code, payload)
	}
	if code, payload := c.call(t, "PUT", "/api/admin/videos/SONGUPLOAD1", `{"work":"yes"}`); code != 400 {
		t.Fatalf("invalid=%d %v", code, payload)
	}
}
