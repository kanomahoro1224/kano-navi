package japantime

import (
	"testing"
	"time"
)

func TestDateRollsOverAtJapanMidnight(t *testing.T) {
	before := time.Date(2026, 3, 2, 14, 59, 59, 0, time.UTC)
	if got := Date(before); got != "2026-03-02" {
		t.Fatalf("Date(%s) = %s", before, got)
	}
	if got := Date(before.Add(time.Second)); got != "2026-03-03" {
		t.Fatalf("Date(%s) = %s", before.Add(time.Second), got)
	}
}

func TestParseDateIsJapanMidnight(t *testing.T) {
	got, err := ParseDate("2026-03-03")
	if err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2026, 3, 2, 15, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Fatalf("ParseDate = %s, want %s", got.UTC(), want)
	}
	if got.Format(time.RFC3339) != "2026-03-03T00:00:00+09:00" {
		t.Fatalf("offset = %s", got.Format(time.RFC3339))
	}
	for _, bad := range []string{"", "2026-02-30", "2026-3-3", "2026-03-03T00:00"} {
		if _, err := ParseDate(bad); err == nil {
			t.Fatalf("ParseDate(%q) accepted", bad)
		}
	}
}

func TestParseLocalReadsJapanWallClock(t *testing.T) {
	got, err := ParseLocal("2006-01-02T15:04:05.999", "2026-03-03T23:59:59.999")
	if err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2026, 3, 3, 14, 59, 59, 999000000, time.UTC); !got.Equal(want) {
		t.Fatalf("ParseLocal = %s, want %s", got.UTC(), want)
	}
}

func TestZoneMatchesTokyoWithoutDaylightSaving(t *testing.T) {
	for _, month := range []time.Month{time.January, time.July} {
		name, offset := time.Date(2026, month, 1, 0, 0, 0, 0, Zone).Zone()
		if name != "JST" || offset != 9*60*60 {
			t.Fatalf("%s zone = %s %d", month, name, offset)
		}
	}
}
