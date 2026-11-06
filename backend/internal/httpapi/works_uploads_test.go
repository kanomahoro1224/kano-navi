package httpapi

import (
	"net/http"
	"strings"
	"testing"

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
