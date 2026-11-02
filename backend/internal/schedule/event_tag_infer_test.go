package schedule

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

// addReservation inserts an automatic YouTube reservation days ahead of today
// in Japan.
func (f *fixture) addReservation(id, title string, days int) {
	f.t.Helper()
	day := time.Now().In(jst).AddDate(0, 0, days).Format("2006-01-02")
	f.exec(`INSERT INTO events (id,source,source_item_id,source_key,title,starts_on,timezone,time_precision,event_type,provenance,manual_locked,created_at,updated_at)
		VALUES (?,'youtube',?,'reservation',?,?,'Asia/Tokyo','unknown','stream','automatic',0,?,?)`, id, id, title, day, testTimestamp, testTimestamp)
}

func tagReply(tag any) string {
	return chatReply(stringify(obj("tag", tag)))
}

// tagState is "tag|source|attempted", where attempted reports whether the
// row carries an attempt stamp.
func (f *fixture) tagState(id string) string {
	f.t.Helper()
	return f.text(`SELECT COALESCE(tag,'') || '|' || COALESCE(tag_source,'') || '|' || CASE WHEN tag_attempted_at IS NULL THEN '' ELSE 'attempted' END FROM events WHERE id=?`, id)
}

// A title every target fails on counts as failed and leaves the entry
// unjudged; the run goes on to the next titles and finishes without a store
// error.
func TestEventTagFailureCountsAndContinues(t *testing.T) {
	f, translator := newTranslatorFixture(t)
	f.addProvider("main", protocolChat, "https://translate.example.test", "synthetic-model", `["text"]`)
	f.queueTranslation("main")
	f.addReservation("a", "synthetic failing title", 1)
	f.addReservation("b", "【歌枠】synthetic", 2)
	f.addReservation("c", "【歌枠】synthetic", 3)
	f.provider.respond = func(request recordedRequest) (int, string) {
		if strings.Contains(request.body, "failing title") {
			return http.StatusServiceUnavailable, `{}`
		}
		return http.StatusOK, tagReply("song")
	}
	result := runTranslator(t, translator)
	tags, _ := result.Counts["eventTags"].(map[string]any)
	if tags["attempted"] != 3 || tags["success"] != 2 || tags["failed"] != 1 {
		t.Fatalf("eventTags=%v", tags)
	}
	// The transient failure is retried on the same target before giving up.
	if f.provider.count() != defaultAttempts+2 {
		t.Fatalf("requests=%d", f.provider.count())
	}
	for id, want := range map[string]string{"a": "||attempted", "b": "song|title|attempted", "c": "song|title|attempted"} {
		if got := f.tagState(id); got != want {
			t.Errorf("%s state=%q, want %q", id, got, want)
		}
	}
	// Every target down: each title fails, none is judged, and the provider
	// status records the failure.
	f.addReservation("d", "synthetic", 4)
	f.provider.respond = func(recordedRequest) (int, string) { return http.StatusServiceUnavailable, `{}` }
	result = runTranslator(t, translator)
	tags, _ = result.Counts["eventTags"].(map[string]any)
	if tags["failed"] != 2 || tags["success"] != 0 {
		t.Fatalf("outage eventTags=%v", tags)
	}
	if got := f.tagState("d"); got != "||attempted" {
		t.Fatalf("outage judged the entry: %q", got)
	}
	if got := f.text(`SELECT last_status FROM llm_providers WHERE id='main'`); got != "failed" {
		t.Fatalf("provider status=%q", got)
	}
}

// With a budget of one title per run, a title that keeps failing waits behind
// the titles not yet attempted and is asked again once they have had a turn.
func TestFailingEventTagDoesNotHoldTheQueue(t *testing.T) {
	f, translator := newTranslatorFixture(t)
	f.addProvider("main", protocolChat, "https://translate.example.test", "synthetic-model", `["text"]`)
	f.queueTranslation("main")
	f.exec(`INSERT INTO app_settings (key,value,updated_at) VALUES ('sync_translation_scan_limit','1',?)`, testTimestamp)
	f.addReservation("a", "synthetic failing title", 1)
	f.addReservation("b", "synthetic b", 2)
	f.addReservation("c", "synthetic c", 3)
	failing := true
	asked := []string{}
	f.provider.respond = func(request recordedRequest) (int, string) {
		for _, title := range []string{"failing title", "synthetic b", "synthetic c"} {
			if strings.Contains(request.body, title) {
				asked = append(asked, title)
			}
		}
		if failing && strings.Contains(request.body, "failing title") {
			return http.StatusBadRequest, `{}`
		}
		return http.StatusOK, tagReply("talk")
	}
	runTranslator(t, translator)
	runTranslator(t, translator)
	runTranslator(t, translator)
	if strings.Join(asked, ",") != "failing title,synthetic b,synthetic c" {
		t.Fatalf("asked=%v", asked)
	}
	failing = false
	runTranslator(t, translator)
	if got := f.tagState("a"); got != "talk|title|attempted" {
		t.Fatalf("retried title state=%q", got)
	}
}

// An over-long title costs no call; it is judged without a label.
func TestOverlongEventTagTitleIsSkipped(t *testing.T) {
	f, translator := newTranslatorFixture(t)
	f.addProvider("main", protocolChat, "https://translate.example.test", "synthetic-model", `["text"]`)
	f.queueTranslation("main")
	f.addReservation("a", strings.Repeat("歌", maxTranslationInput+1), 1)
	result := runTranslator(t, translator)
	tags, _ := result.Counts["eventTags"].(map[string]any)
	if f.provider.count() != 0 || tags["skipped"] != 1 || tags["attempted"] != 0 {
		t.Fatalf("requests=%d eventTags=%v", f.provider.count(), tags)
	}
	if got := f.tagState("a"); got != "|title|attempted" {
		t.Fatalf("state=%q", got)
	}
}

// A label outside the codes is an answer without one: the title is judged
// and not asked again, and the provider is not marked failed.
func TestUnknownEventTagIsJudgedWithoutALabel(t *testing.T) {
	f, translator := newTranslatorFixture(t)
	f.addProvider("main", protocolChat, "https://translate.example.test", "synthetic-model", `["text"]`)
	f.queueTranslation("main")
	f.addReservation("a", "synthetic", 1)
	f.provider.respond = func(recordedRequest) (int, string) { return http.StatusOK, tagReply("music") }
	result := runTranslator(t, translator)
	tags, _ := result.Counts["eventTags"].(map[string]any)
	if tags["success"] != 1 || tags["failed"] != 0 || result.Status != "success" {
		t.Fatalf("eventTags=%v status=%s", tags, result.Status)
	}
	if got := f.tagState("a"); got != "|title|attempted" {
		t.Fatalf("state=%q", got)
	}
	runTranslator(t, translator)
	if f.provider.count() != 1 {
		t.Fatalf("asked again: requests=%d", f.provider.count())
	}
}

// The request names the entry's form, so a premiere whose title shows no
// content is not judged a stream.
func TestEventTagRequestNamesTheForm(t *testing.T) {
	f, translator := newTranslatorFixture(t)
	f.addProvider("main", protocolChat, "https://translate.example.test", "synthetic-model", `["text"]`)
	f.queueTranslation("main")
	f.addReservation("stream", "synthetic stream", 1)
	f.addReservation("premiere", "synthetic premiere", 2)
	f.exec(`UPDATE events SET event_type='release',detail='YouTube Premiere' WHERE id='premiere'`)
	f.provider.respond = func(recordedRequest) (int, string) { return http.StatusOK, tagReply(nil) }
	runTranslator(t, translator)
	bodies := ""
	for _, request := range f.provider.requests {
		bodies += request.body
	}
	for _, want := range []string{"YouTube stream title: synthetic stream", "YouTube premiere title: synthetic premiere"} {
		if !strings.Contains(bodies, want) {
			t.Errorf("no request with %q in %s", want, bodies)
		}
	}
}
