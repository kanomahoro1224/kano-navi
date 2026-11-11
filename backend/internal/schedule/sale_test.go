package schedule

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// saleEntry is one model entry for the sale tests; fields left out of extra
// keep the values of an ordinary dated entry.
func saleEntry(extra map[string]any) string {
	entry := map[string]any{
		"title": "受注期間", "detail": nil, "date": "2099-08-22", "time": nil, "endTime": nil, "endDate": nil,
		"timePrecision": "unknown", "status": "scheduled", "eventType": "sale", "tag": nil,
		"url": "https://shop.example.test/collections/synthetic", "confidence": 0.8, "evidence": "",
	}
	for key, value := range extra {
		entry[key] = value
	}
	return `{"classification":"schedule","events":[` + stringify(toObject(entry)) + `],"confidence":0.8,"evidence":"","reason":null,"action":"add"}`
}

func toObject(values map[string]any) *object {
	pairs := []any{}
	for _, key := range []string{"title", "detail", "date", "time", "endTime", "endDate", "timePrecision", "status", "eventType", "tag", "url", "confidence", "evidence"} {
		if value, ok := values[key]; ok {
			pairs = append(pairs, key, value)
		}
	}
	return obj(pairs...)
}

func normalizeSale(t *testing.T, extra map[string]any) (normalizedExtraction, error) {
	t.Helper()
	input, err := parseJSON([]byte(saleEntry(extra)))
	if err != nil {
		t.Fatal(err)
	}
	published := time.Date(2099, 8, 20, 3, 0, 0, 0, time.UTC)
	return normalizeExtractionResult(input, "post-1", "https://x.com/synthetic_handle/status/1", published, published)
}

func orNil(value *string) string {
	if value == nil {
		return "<nil>"
	}
	return *value
}

func TestASalePeriodKeepsItsOpeningAndItsClose(t *testing.T) {
	result, err := normalizeSale(t, map[string]any{"time": "12:00", "timePrecision": "exact", "endDate": "2099-09-22", "endTime": "23:59"})
	if err != nil {
		t.Fatal(err)
	}
	event := result.events[0]
	if event.eventType != "sale" || event.startsOn != "2099-08-22" || orNil(event.startsAt) != "2099-08-22T03:00:00.000Z" ||
		orNil(event.endsOn) != "2099-09-22" || orNil(event.endsAt) != "2099-09-22T14:59:00.000Z" {
		t.Fatalf("event = %+v (endsOn %s, endsAt %s)", event, orNil(event.endsOn), orNil(event.endsAt))
	}
	raw := stringify(event.rawObject())
	if want := `"ends_at":"2099-09-22T14:59:00.000Z","ends_on":"2099-09-22","timezone"`; !strings.Contains(raw, want) {
		t.Fatalf("raw = %s", raw)
	}
}

func TestADeadlineWithoutItsOpeningIsADateOnlyPeriod(t *testing.T) {
	// 「9/22 23:59まで」: the deadline is the date and the close date, and the
	// close time needs no start time.
	result, err := normalizeSale(t, map[string]any{"date": "2099-09-22", "endDate": "2099-09-22", "endTime": "23:59"})
	if err != nil {
		t.Fatal(err)
	}
	event := result.events[0]
	if event.startsOn != "2099-09-22" || event.startsAt != nil || orNil(event.endsOn) != "2099-09-22" ||
		orNil(event.endsAt) != "2099-09-22T14:59:00.000Z" || event.timePrecision != "unknown" {
		t.Fatalf("event = %+v (endsOn %s, endsAt %s)", event, orNil(event.endsOn), orNil(event.endsAt))
	}
	// A close date alone stores no close time.
	result, err = normalizeSale(t, map[string]any{"endDate": "2099-09-22"})
	if err != nil {
		t.Fatal(err)
	}
	if event = result.events[0]; orNil(event.endsOn) != "2099-09-22" || event.endsAt != nil {
		t.Fatalf("event = %+v", event)
	}
}

func TestASaleCloseTimeWithoutADateClosesOnTheDateItFallsOn(t *testing.T) {
	// 「12:00〜18:00」 closes on the opening date; 「18:00〜26:00」 on the next.
	for endTime, want := range map[string][2]string{
		"18:00": {"2099-08-22", "2099-08-22T09:00:00.000Z"},
		"02:00": {"2099-08-23", "2099-08-22T17:00:00.000Z"},
	} {
		start := "12:00"
		if endTime == "02:00" {
			start = "18:00"
		}
		result, err := normalizeSale(t, map[string]any{"time": start, "timePrecision": "exact", "endTime": endTime})
		if err != nil {
			t.Fatal(err)
		}
		if event := result.events[0]; orNil(event.endsOn) != want[0] || orNil(event.endsAt) != want[1] {
			t.Errorf("%s: endsOn %s, endsAt %s", endTime, orNil(event.endsOn), orNil(event.endsAt))
		}
	}
	// Every other type keeps its end time without a close date.
	result, err := normalizeSale(t, map[string]any{"eventType": "event", "time": "12:00", "timePrecision": "exact", "endTime": "18:00"})
	if err != nil {
		t.Fatal(err)
	}
	if event := result.events[0]; event.endsOn != nil || event.endsAt == nil {
		t.Fatalf("event = %+v", event)
	}
}

func TestOnlyASaleStoresACloseDate(t *testing.T) {
	result, err := normalizeSale(t, map[string]any{"eventType": "event", "endDate": "2099-09-22"})
	if err != nil {
		t.Fatal(err)
	}
	if event := result.events[0]; event.endsOn != nil {
		t.Fatalf("event = %+v", event)
	}
	if raw := stringify(result.events[0].rawObject()); strings.Contains(raw, "ends_on") {
		t.Fatalf("raw = %s", raw)
	}
	// Without a close date, an end time still needs a start time.
	if _, err = normalizeSale(t, map[string]any{"eventType": "event", "endDate": "2099-09-22", "endTime": "23:59"}); !invalidEvent(err) {
		t.Fatalf("err = %v", err)
	}
}

func TestASaleCloseMustBeACalendarDateNotBeforeItsOpening(t *testing.T) {
	for _, endDate := range []any{"2099-08-21", "2099-02-30", "9/22", 22} {
		if _, err := normalizeSale(t, map[string]any{"endDate": endDate}); !invalidEvent(err) {
			t.Errorf("endDate %v: err = %v", endDate, err)
		}
	}
}

func invalidEvent(err error) bool {
	var message *messageError
	return errors.As(err, &message) && message.message == "schedule extraction contained an invalid event"
}
