package schedule

import (
	"net/http"
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

// An unknown label is a broken reply, not a guess to keep; a missing or null
// label means the entry simply has no label.
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
	}
	if _, err := normalizeExtractedEvents(items("karaoke", true), "post-1", "https://x.com/example/status/1", now); err == nil {
		t.Fatal("unknown tag accepted")
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
