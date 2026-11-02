package schedule

import (
	"net/http"
	"strings"
	"testing"
)

// addVideo inserts a stored video published long before the translation
// lookback, so only the tag inference reads it.
func (f *fixture) addVideo(id, source, title, kind, published string) {
	f.t.Helper()
	f.exec(`INSERT INTO videos (id,source,title,published_at,url,kind) VALUES (?,?,?,?,?,?)`,
		id, source, title, published, "https://video.example.test/"+id, kind)
}

// videoTagState is "tag|source|attempted" for a video, like tagState.
func (f *fixture) videoTagState(id string) string {
	f.t.Helper()
	return f.text(`SELECT COALESCE(tag,'') || '|' || COALESCE(tag_source,'') || '|' || CASE WHEN tag_attempted_at IS NULL THEN '' ELSE 'attempted' END FROM videos WHERE id=?`, id)
}

// Videos the schedule cannot label are asked newest first, after the
// entries; a form label (video, live) is judged without a tag, and videos
// with a manual tag, a labelled or pending entry, or a merge target are not
// asked.
func TestVideoTagsLabelVideosTheScheduleCannot(t *testing.T) {
	f, translator := newTranslatorFixture(t)
	f.addProvider("main", protocolChat, "https://translate.example.test", "synthetic-model", `["text"]`)
	f.queueTranslation("main")
	f.addVideo("cover", "youtube", "video 歌ってみた", "upload", "2020-03-03T00:00:00Z")
	f.addVideo("vlog", "youtube", "video vlog", "upload", "2020-03-02T00:00:00Z")
	f.addVideo("old", "youtube", "video old stream", "stream", "2020-03-01T00:00:00Z")
	f.addVideo("labelled", "youtube", "video labelled", "stream", "2020-03-04T00:00:00Z")
	f.addVideo("manual", "youtube", "video manual", "upload", "2020-03-05T00:00:00Z")
	f.addVideo("fresh", "youtube", "video fresh", "stream", "2020-03-06T00:00:00Z")
	f.addVideo("BV1AAAAAAAAA", "bilibili", "video merged", "upload", "2020-03-07T00:00:00Z")
	f.exec(`UPDATE videos SET manual_tag='talk' WHERE id='manual'`)
	f.exec(`UPDATE videos SET merged_into='cover' WHERE id='BV1AAAAAAAAA'`)
	f.addReservation("labelled", "entry labelled", 2)
	f.exec(`UPDATE events SET tag='game',tag_source='llm' WHERE id='labelled'`)
	f.addReservation("fresh", "entry fresh", 1)
	// An entry before the inference window is never labelled, so its video is.
	f.addReservation("old", "entry old", -60)
	asked := []string{}
	f.provider.respond = func(request recordedRequest) (int, string) {
		for _, title := range []string{"entry fresh", "video 歌ってみた", "video vlog", "video old stream", "entry", "video"} {
			if strings.Contains(request.body, title) {
				asked = append(asked, title)
				break
			}
		}
		switch {
		case strings.Contains(request.body, "歌ってみた"):
			return http.StatusOK, tagReply("song")
		case strings.Contains(request.body, "vlog"):
			return http.StatusOK, tagReply("video")
		case strings.Contains(request.body, "old stream"):
			return http.StatusOK, tagReply("talk")
		}
		return http.StatusOK, tagReply("live")
	}
	result := runTranslator(t, translator)
	if got := strings.Join(asked, ","); got != "entry fresh,video 歌ってみた,video vlog,video old stream" {
		t.Fatalf("asked=%s", got)
	}
	if !strings.Contains(f.provider.requests[1].body, "YouTube title: video 歌ってみた") {
		t.Fatalf("request=%s", f.provider.requests[1].body)
	}
	videos, _ := result.Counts["videoTags"].(map[string]any)
	if videos["attempted"] != 3 || videos["success"] != 3 || result.Status != "success" {
		t.Fatalf("videoTags=%v status=%s", videos, result.Status)
	}
	for id, want := range map[string]string{
		"cover": "song|title|attempted", "vlog": "|title|attempted", "old": "talk|title|attempted",
		"labelled": "||", "manual": "||", "fresh": "||", "BV1AAAAAAAAA": "||",
	} {
		if got := f.videoTagState(id); got != want {
			t.Errorf("%s state=%q, want %q", id, got, want)
		}
	}
	// Judged videos are not asked again.
	asked = asked[:0]
	runTranslator(t, translator)
	if len(asked) != 0 {
		t.Fatalf("asked again=%v", asked)
	}
}

// Entries spend the per-run limit first; a failed video title only stamps
// its attempt, so it stays unjudged and is asked after the others. A
// Bilibili upload is never shown, so it is never asked about.
func TestVideoTagsShareTheLimitAndFailuresStayUnjudged(t *testing.T) {
	f, translator := newTranslatorFixture(t)
	f.addProvider("main", protocolChat, "https://translate.example.test", "synthetic-model", `["text"]`)
	f.queueTranslation("main")
	f.exec(`INSERT INTO app_settings (key,value,updated_at) VALUES ('sync_translation_scan_limit','1',?)`, testTimestamp)
	f.addReservation("entry", "entry title", 1)
	f.addVideo("BV1CCCCCCCCC", "bilibili", "video bilibili", "upload", "2020-03-03T00:00:00Z")
	f.addVideo("failing", "youtube", "video failing", "upload", "2020-03-02T00:00:00Z")
	f.addVideo("later", "youtube", "video later", "upload", "2020-03-01T00:00:00Z")
	f.provider.respond = func(request recordedRequest) (int, string) {
		if strings.Contains(request.body, "failing") {
			return http.StatusBadRequest, `{}`
		}
		return http.StatusOK, tagReply("game")
	}
	result := runTranslator(t, translator)
	if videos, _ := result.Counts["videoTags"].(map[string]any); f.provider.count() != 1 || videos["attempted"] != 0 {
		t.Fatalf("requests=%d videoTags=%v", f.provider.count(), videos)
	}
	result = runTranslator(t, translator)
	if videos, _ := result.Counts["videoTags"].(map[string]any); videos["failed"] != 1 || !strings.Contains(f.provider.requests[1].body, "YouTube title: video failing") {
		t.Fatalf("videoTags=%v", videos)
	}
	if got := f.videoTagState("failing"); got != "||attempted" {
		t.Fatalf("failed state=%q", got)
	}
	runTranslator(t, translator)
	if got := f.videoTagState("later"); got != "game|title|attempted" {
		t.Fatalf("later state=%q", got)
	}
	for _, request := range f.provider.requests {
		if strings.Contains(request.body, "video bilibili") {
			t.Fatalf("a Bilibili upload was asked about: %s", request.body)
		}
	}
	if got := f.videoTagState("BV1CCCCCCCCC"); got != "||" {
		t.Fatalf("bilibili state=%q", got)
	}
}
