package schedule

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

// taggedScheduleReply is the recorded reply shape with the optional calendar
// label the extraction schema now asks for.
func taggedScheduleReply(tag any) string {
	return stringify(obj(
		"classification", "schedule",
		"events", []any{obj(
			"title", songTitle, "detail", nil, "date", songDate, "time", songClock, "endTime", nil,
			"timePrecision", "exact", "status", "scheduled", "eventType", "stream", "tag", tag, "url", nil,
			"confidence", 0.9, "evidence", "synthetic evidence")},
		"confidence", 0.9, "evidence", "synthetic evidence", "reason", nil, "action", "add",
	))
}

// A missing, null or unknown label means the entry simply has no label; an
// unknown value is not kept, and it does not cost the entry itself.
func TestNormalizeKeepsOnlyKnownEventTags(t *testing.T) {
	items := func(tag any, withTag bool) []any {
		pairs := []any{
			"title", songTitle, "detail", nil, "date", songDate, "time", songClock, "endTime", nil,
			"timePrecision", "exact", "status", "scheduled", "eventType", "stream"}
		if withTag {
			pairs = append(pairs, "tag", tag)
		}
		pairs = append(pairs, "url", nil, "confidence", 0.9, "evidence", "synthetic evidence")
		return []any{obj(pairs...)}
	}
	now := time.Date(2099, 5, 1, 0, 0, 0, 0, time.UTC)
	for _, tt := range []struct {
		name    string
		tag     any
		withTag bool
		want    string
	}{
		{"known", "song", true, "song"},
		{"null", nil, true, ""},
		{"absent", nil, false, ""},
		{"unknown", "karaoke", true, ""},
		{"not a string", 3, true, ""},
	} {
		events, err := normalizeExtractedEvents(items(tt.tag, tt.withTag), "post-1", "https://x.com/example/status/1", now)
		if err != nil {
			t.Fatalf("%s: %v", tt.name, err)
		}
		got := ""
		if events[0].tag != nil {
			got = *events[0].tag
		}
		if got != tt.want {
			t.Fatalf("%s tag=%q, want %q", tt.name, got, tt.want)
		}
		if len(events) != 1 || events[0].title != songTitle {
			t.Fatalf("%s events=%+v", tt.name, events)
		}
	}
}

// The extraction prompt and the title inference send the same label
// definition, so a stream or premiere gets the same label whichever path
// judges it.
func TestExtractionAndTitleInferenceShareTheTagRule(t *testing.T) {
	f := messageFixture(t)
	f.provider.respond = func(recordedRequest) (int, string) {
		return http.StatusOK, chatReply(taggedScheduleReply(nil))
	}
	f.scan()
	titles, translator := newTranslatorFixture(t)
	titles.addProvider("main", protocolChat, "https://translate.example.test", "synthetic-model", `["text"]`)
	titles.queueTranslation("main")
	titles.addReservation("a", "【歌枠】synthetic", 1)
	titles.provider.respond = func(recordedRequest) (int, string) { return http.StatusOK, tagReply("song") }
	runTranslator(t, translator)
	for name, provider := range map[string]*fakeProvider{"extraction": f.provider, "title inference": titles.provider} {
		if provider.count() == 0 {
			t.Fatalf("%s sent no request", name)
		}
		if !strings.Contains(provider.requests[0].body, eventTagRule) {
			t.Errorf("%s request lacks the shared tag rule", name)
		}
	}
}

// A provider without strict schema output may answer a label outside the
// list; the schedule it extracted is stored without a label.
func TestExtractionKeepsEventWithUnknownTag(t *testing.T) {
	f := messageFixture(t)
	f.provider.respond = func(recordedRequest) (int, string) {
		return http.StatusOK, chatReply(taggedScheduleReply("singing"))
	}
	f.scan()
	if got := f.text(`SELECT title || '|' || COALESCE(tag,'') || '|' || COALESCE(tag_source,'') FROM events`); got != songTitle+"||" {
		t.Fatalf("stored event = %q", got)
	}
}

// A post that names no label, or only a form, keeps the content label the
// title inference stored on the same entry, and a content label the post
// states replaces it.
func TestExtractionKeepsTitleTagUnlessThePostStatesOne(t *testing.T) {
	f := messageFixture(t)
	f.provider.respond = func(recordedRequest) (int, string) {
		return http.StatusOK, chatReply(taggedScheduleReply(nil))
	}
	f.scan()
	f.exec(`UPDATE events SET tag='song',tag_source='title'`)
	f.requestReprocess("post-1")
	f.scan()
	if got := f.text(`SELECT COALESCE(tag,'') || '|' || COALESCE(tag_source,'') FROM events`); got != "song|title" {
		t.Fatalf("null extraction cleared the inferred label: %q", got)
	}
	// A form label names less than the inferred content label, so it stays.
	f.provider.respond = func(recordedRequest) (int, string) {
		return http.StatusOK, chatReply(taggedScheduleReply("live"))
	}
	f.requestReprocess("post-1")
	f.scan()
	if got := f.text(`SELECT COALESCE(tag,'') || '|' || COALESCE(tag_source,'') FROM events`); got != "song|title" {
		t.Fatalf("form label replaced the inferred content label: %q", got)
	}
	f.provider.respond = func(recordedRequest) (int, string) {
		return http.StatusOK, chatReply(taggedScheduleReply("talk"))
	}
	f.requestReprocess("post-1")
	f.scan()
	if got := f.text(`SELECT COALESCE(tag,'') || '|' || COALESCE(tag_source,'') FROM events`); got != "talk|llm" {
		t.Fatalf("stated label did not replace the inferred one: %q", got)
	}
}

// The label the model returns is stored with its source, and an operator's own
// label and lock survive the next extraction of the same post.
func TestExtractionStoresEventTagAndKeepsOperatorValue(t *testing.T) {
	f := messageFixture(t)
	f.provider.respond = func(recordedRequest) (int, string) {
		return http.StatusOK, chatReply(taggedScheduleReply("song"))
	}
	f.scan()
	if got := f.text(`SELECT COALESCE(tag,'') || '|' || COALESCE(tag_source,'') FROM events`); got != "song|llm" {
		t.Fatalf("stored tag = %q", got)
	}
	f.exec(`UPDATE events SET tag='talk',tag_source='manual',provenance='manual',manual_locked=1`)
	f.provider.respond = func(recordedRequest) (int, string) {
		return http.StatusOK, chatReply(taggedScheduleReply("game"))
	}
	f.requestReprocess("post-1")
	f.scan()
	if got := f.text(`SELECT COALESCE(tag,'') || '|' || COALESCE(tag_source,'') FROM events`); got != "talk|manual" {
		t.Fatalf("operator label overwritten: %q", got)
	}
}
