package schedule

import (
	"strings"
	"testing"
	"time"
)

func TestRolledYearMovesOnlyADateAboutAYearBeforeThePost(t *testing.T) {
	december := time.Date(2026, 12, 20, 3, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		date      string
		published time.Time
		want      string
	}{
		// A December notice's 1/5 means the coming January.
		{"2026-01-05", december, "2027-01-05"},
		// Late on 31 December UTC is already New Year's Day in Japan.
		{"2026-01-03", time.Date(2026, 12, 31, 16, 0, 0, 0, time.UTC), "2027-01-03"},
		// Recent and coming dates stay.
		{"2026-12-24", december, "2026-12-24"},
		{"2026-11-30", december, "2026-11-30"},
		{"2027-01-05", december, "2027-01-05"},
		// A date old either way, such as an anniversary, stays.
		{"2020-02-28", december, "2020-02-28"},
		{"2025-12-01", december, "2025-12-01"},
		// Half a year back would land too far ahead.
		{"2026-06-01", december, "2026-06-01"},
		// A leap day has no date a year later, and no publication time
		// leaves every date alone.
		{"2024-02-29", time.Date(2024, 12, 20, 0, 0, 0, 0, time.UTC), "2024-02-29"},
		{"2026-01-05", time.Time{}, "2026-01-05"},
	} {
		if got := rolledYear(tc.date, tc.published); got != tc.want {
			t.Errorf("rolledYear(%s, %s) = %s, want %s", tc.date, tc.published, got, tc.want)
		}
	}
}

func TestAnEntryRolledIntoTheNextYearTakesThatDateInItsIdentity(t *testing.T) {
	input, err := parseJSON([]byte(`{"classification":"schedule","events":[{"title":"T","detail":null,"date":"2026-01-05","time":"21:00","endTime":null,"timePrecision":"exact","status":"scheduled","eventType":"stream","url":null,"confidence":0.9,"evidence":""}]}`))
	if err != nil {
		t.Fatal(err)
	}
	published := time.Date(2026, 12, 20, 3, 0, 0, 0, time.UTC)
	result, err := normalizeExtractionResult(input, "post-1", "https://x.com/synthetic_handle/status/1", published, published)
	if err != nil {
		t.Fatal(err)
	}
	event := result.events[0]
	if event.startsOn != "2027-01-05" || event.startsAt == nil || *event.startsAt != "2027-01-05T12:00:00.000Z" ||
		!strings.HasPrefix(event.id, "x-schedule-20270105-") || !strings.HasPrefix(event.sourceKey, "2027-01-05T21:00:") || !event.isUpcoming {
		t.Fatalf("event=%+v", event)
	}
}
