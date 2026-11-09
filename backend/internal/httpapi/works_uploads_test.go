package httpapi

import (
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/yexca/kano-navi/backend/internal/works"
)

// packagedVideo is the first video of a platform in the shipped works list,
// with its upload.
func packagedVideo(t *testing.T, platform string) (string, string) {
	t.Helper()
	for _, upload := range works.Packaged().Uploads {
		for _, video := range upload.Videos {
			if video.Platform == platform && !video.Members {
				return upload.ID, video.ID
			}
		}
	}
	t.Fatalf("no %s video in the works list", platform)
	return "", ""
}

func TestWorksUploadsJoinStoredVideosByID(t *testing.T) {
	upload, id := packagedVideo(t, works.YouTube)
	handler := archiveHandler(t, `
		INSERT INTO videos (id,source,title,manual_title,published_at,url,thumbnail_url,duration_seconds,availability,raw_json) VALUES
		 ('`+id+`','youtube','Platform title','Corrected title','2026-01-01T00:00:00Z','https://www.youtube.com/watch?v=`+id+`','https://i.ytimg.com/vi/`+id+`/maxresdefault.jpg',245,'removed','{"private":"synthetic"}'),
		 ('zzzzzzzzzzz','youtube','Not a work','','2026-01-01T00:00:00Z','https://www.youtube.com/watch?v=zzzzzzzzzzz',NULL,NULL,NULL,NULL);
		INSERT INTO app_settings (key,value,updated_at) VALUES ('dashboard_revision','7','');`)

	recorder := request(t, handler, http.MethodGet, "/api/works/uploads")
	if recorder.Code != http.StatusOK || !strings.HasPrefix(recorder.Header().Get("ETag"), `W/"`) {
		t.Fatalf("status=%d headers=%v body=%s", recorder.Code, recorder.Header(), recorder.Body)
	}
	body := recorder.Body.String()
	for _, leak := range []string{"private", "raw", "zzzzzzzzzzz", "i.ytimg.com"} {
		if strings.Contains(body, leak) {
			t.Fatalf("works uploads leaked %q: %s", leak, body)
		}
	}
	payload := decodeObject(t, recorder)
	if payload["updatedAt"] != works.Packaged().UpdatedAt || payload["revision"] != float64(7) {
		t.Fatalf("payload=%#v", payload)
	}
	uploads := payload["uploads"].(map[string]any)
	if len(uploads) != 1 {
		t.Fatalf("only the stored work is listed: %#v", uploads)
	}
	item := uploads[upload].(map[string]any)
	videos := item["videos"].([]any)
	video := videos[0].(map[string]any)
	// The image is not cached yet, so there is no cover to show.
	if item["cover"] != nil || len(videos) != 1 || video["title"] != "Corrected title" || video["availability"] != "removed" || video["durationSeconds"] != float64(245) {
		t.Fatalf("item=%#v", item)
	}

	if recorder := request(t, handler, http.MethodPost, "/api/works/uploads"); recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST status=%d", recorder.Code)
	}
}

// The recent uploads after the list's date carry only their public fields,
// and a Bilibili-only upload reaches them but not the video archive.
func TestWorksUploadsListRecentUploadsWithPublicFieldsOnly(t *testing.T) {
	updated, err := time.Parse("2006-01-02", works.Packaged().UpdatedAt)
	if err != nil {
		t.Fatal(err)
	}
	day := updated.AddDate(0, 0, 2).Format("2006-01-02")
	at := day + "T03:00:00.000Z"
	handler := archiveHandler(t, `
		INSERT INTO videos (id,source,title,manual_title,published_at,url,thumbnail_url,kind,tag,tag_source,raw_json,merged_into,manual_work) VALUES
		 ('RECENTSONG1','youtube','Platform title','Corrected title','`+at+`','https://www.youtube.com/watch?v=RECENTSONG1','https://i.ytimg.com/vi/RECENTSONG1/hqdefault.jpg','upload','song','title','{"private":"synthetic"}',NULL,NULL),
		 ('BV1MERGEDCOPY','bilibili','Copy','','`+at+`','https://www.bilibili.com/video/BV1MERGEDCOPY',NULL,'upload',NULL,NULL,'{}','RECENTSONG1',NULL),
		 ('BV1ONLYUPLOAD','bilibili','Bilibili only','','`+at+`','https://www.bilibili.com/video/BV1ONLYUPLOAD',NULL,'upload',NULL,NULL,'{}',NULL,1),
		 ('EXCLUDEDSNG','youtube','Excluded','','`+at+`','https://www.youtube.com/watch?v=EXCLUDEDSNG',NULL,'upload','song','title','{}',NULL,0);`)
	recorder := request(t, handler, http.MethodGet, "/api/works/uploads")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body)
	}
	body := recorder.Body.String()
	for _, leak := range []string{"private", "raw", "i.ytimg.com", "manual", "EXCLUDEDSNG", "Platform title"} {
		if strings.Contains(body, leak) {
			t.Fatalf("works uploads leaked %q: %s", leak, body)
		}
	}
	recent := decodeObject(t, recorder)["recent"].([]any)
	if len(recent) != 2 {
		t.Fatalf("recent=%#v", recent)
	}
	keys := func(item map[string]any) string {
		names := []string{}
		for key := range item {
			names = append(names, key)
		}
		slices.Sort(names)
		return strings.Join(names, ",")
	}
	byID := map[string]map[string]any{}
	for _, raw := range recent {
		item := raw.(map[string]any)
		if got := keys(item); got != "alternates,cover,id,origin,publishedAt,source,sourceLanguage,title,translations,url" {
			t.Fatalf("fields=%s", got)
		}
		byID[item["id"].(string)] = item
	}
	song, bilibili := byID["RECENTSONG1"], byID["BV1ONLYUPLOAD"]
	alternates, _ := song["alternates"].([]any)
	if song["title"] != "Corrected title" || song["origin"] != "auto" || song["cover"] != nil || song["publishedAt"] != at ||
		len(alternates) != 1 || alternates[0].(map[string]any)["url"] != "https://www.bilibili.com/video/BV1MERGEDCOPY" {
		t.Fatalf("song=%#v", song)
	}
	if bilibili["source"] != "bilibili" || bilibili["origin"] != "operator" {
		t.Fatalf("bilibili=%#v", bilibili)
	}

	archive := decodeObject(t, request(t, handler, http.MethodGet, "/api/videos?from="+day+"&to="+day))
	if ids := archiveIDs(t, archive); len(ids) != 2 || slices.Contains(ids, "BV1ONLYUPLOAD") {
		t.Fatalf("archive ids=%v", ids)
	}
}
