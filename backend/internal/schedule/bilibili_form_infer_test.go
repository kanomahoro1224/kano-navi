package schedule

import (
	"net/http"
	"strings"
	"testing"
)

// bilibiliFormState is "kind|kind_source|kind_review|attempts|tag" for a
// Bilibili upload.
func (f *fixture) bilibiliFormState(id string) string {
	f.t.Helper()
	return f.text(`SELECT COALESCE(kind,'') || '|' || COALESCE(kind_source,'') || '|' || COALESCE(kind_review,'') || '|' || kind_attempts || '|' || COALESCE(tag,'') FROM videos WHERE id=?`, id)
}

func formReply(form string, tag any) string {
	return chatReply(stringify(obj("form", form, "tag", tag)))
}

// The translate step settles Bilibili forms by rule without a model, asks
// the model for form and tag of the uploads the rules leave open, and sends
// an uncertain answer, or the third failure, to the Review queue. An upload a
// rule settled is public and gets the ordinary tag question.
func TestBilibiliFormsSettleByRuleThenModel(t *testing.T) {
	f, translator := newTranslatorFixture(t)
	f.addProvider("main", protocolChat, "https://translate.example.test", "synthetic-model", `["text"]`)
	f.queueTranslation("main")
	add := func(id, title string, duration int) {
		f.addVideo(id, "bilibili", title, "upload", "2020-03-01T00:00:00Z")
		f.exec(`UPDATE videos SET duration_seconds=?,raw_json='{"description":"synthetic description"}' WHERE id=?`, duration, id)
	}
	add("BV1REPLAY001", "【直播回放】synthetic night", 7200)
	add("BV1SHORTCLIP", "synthetic clip", 120)
	add("BV1ASKSTREAM", "synthetic singing", 3000)
	add("BV1UNCERTAIN", "synthetic talk", 1500)
	add("BV1FAILING01", "synthetic failing", 1500)
	asked := []string{}
	f.provider.respond = func(request recordedRequest) (int, string) {
		switch {
		case strings.Contains(request.body, "kano_event_tag"):
			if !strings.Contains(request.body, "Bilibili title: 【直播回放】synthetic night") && !strings.Contains(request.body, "Bilibili title: synthetic clip") {
				t.Errorf("tag request=%s", request.body)
			}
			return http.StatusOK, tagReply("game")
		case strings.Contains(request.body, "synthetic singing"):
			asked = append(asked, "singing")
			if !strings.Contains(request.body, "Length: 50 min 0 s") || !strings.Contains(request.body, "Description start: synthetic description") {
				t.Errorf("request=%s", request.body)
			}
			return http.StatusOK, formReply("stream", "song")
		case strings.Contains(request.body, "synthetic talk"):
			asked = append(asked, "talk")
			return http.StatusOK, formReply("uncertain", "talk")
		case strings.Contains(request.body, "synthetic failing"):
			asked = append(asked, "failing")
			return http.StatusBadRequest, `{}`
		}
		t.Errorf("unexpected request=%s", request.body)
		return http.StatusBadRequest, `{}`
	}
	result := runTranslator(t, translator)
	forms, _ := result.Counts["bilibiliForms"].(map[string]any)
	if forms["ruled"] != 2 || forms["success"] != 1 || forms["uncertain"] != 1 || forms["failed"] != 1 {
		t.Fatalf("bilibiliForms=%v", forms)
	}
	for id, want := range map[string]string{
		"BV1REPLAY001": "stream|rule||0|game",
		"BV1SHORTCLIP": "upload|rule||0|game",
		"BV1ASKSTREAM": "stream|llm||0|song",
		"BV1UNCERTAIN": "upload||uncertain|0|talk",
		"BV1FAILING01": "upload|||1|",
	} {
		if got := f.bilibiliFormState(id); got != want {
			t.Errorf("%s state=%q, want %q", id, got, want)
		}
	}
	// The third failure, once each retry delay passed, sends the upload to
	// the Review queue; a waiting upload is not asked again.
	for range 2 {
		f.exec(`UPDATE videos SET kind_attempted_at='2000-01-01T00:00:00.000Z' WHERE id='BV1FAILING01'`)
		runTranslator(t, translator)
	}
	if got := f.bilibiliFormState("BV1FAILING01"); got != "upload||failed|3|" {
		t.Fatalf("failing state=%q", got)
	}
	asked = asked[:0]
	f.exec(`UPDATE videos SET kind_attempted_at='2000-01-01T00:00:00.000Z'`)
	runTranslator(t, translator)
	if len(asked) != 0 {
		t.Fatalf("asked again=%v", asked)
	}
}
