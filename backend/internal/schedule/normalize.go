package schedule

import (
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
)

// messageError carries one of the fixed, sanitized messages stored in
// schedule_extractions.error,
// post_llm_states.last_error and schedule_asset_reviews.llm_reason.
type messageError struct{ message string }

func (e *messageError) Error() string { return e.message }

func fail(message string) error { return &messageError{message} }

// Japan has no daylight saving time, so a fixed zone avoids needing tzdata.
var jst = time.FixedZone("JST", 9*3600)

var (
	datePattern = regexp.MustCompile(`^(\d{4})-(\d{2})-(\d{2})$`)
	// Japanese schedules count late-night hours on from the evening's date
	// (25:00 is 01:00 the next day), up to 29:59.
	timePattern = regexp.MustCompile(`^([01]\d|2\d):[0-5]\d$`)
)

// parseDate mirrors validDate: Date.UTC maps years 0-99 to 1900-1999, so
// those never round-trip and are rejected.
func parseDate(value string) (year, month, day int, ok bool) {
	match := datePattern.FindStringSubmatch(value)
	if match == nil {
		return 0, 0, 0, false
	}
	year, _ = strconv.Atoi(match[1])
	month, _ = strconv.Atoi(match[2])
	day, _ = strconv.Atoi(match[3])
	if year < 100 || month < 1 || month > 12 || day < 1 {
		return 0, 0, 0, false
	}
	date := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	if date.Year() != year || int(date.Month()) != month || date.Day() != day {
		return 0, 0, 0, false
	}
	return year, month, day, true
}

func japanInstant(date, clock string) time.Time {
	year, month, day, _ := parseDate(date)
	hour, _ := strconv.Atoi(clock[:2])
	minute, _ := strconv.Atoi(clock[3:5])
	return time.Date(year, time.Month(month), day, hour, minute, 0, 0, jst)
}

var cancelledEventStatuses = map[string]bool{
	"cancelled": true,
	"canceled":  true,
	"cancel":    true,
	"取消":        true,
	"已取消":       true,
	"中止":        true,
	"キャンセル":     true,
}

func isCancelledEventStatus(status any) bool {
	value := jsToLower(jsTrim(stringOr(status, "")))
	return cancelledEventStatuses[value] || strings.HasPrefix(value, "cancel")
}

// normalizedEvent is one model entry after validation, in the shape the event
// writer takes.
type normalizedEvent struct {
	id                    string
	canonicalSource       string
	canonicalSourceItemID string
	canonicalSourceKey    string
	sourceKey             string
	title                 string
	detail                *string
	startsOn              string
	startsAt              *string
	endsAt                *string
	// endsOn is the Japan close date of a sale period, nil for every other
	// entry.
	endsOn                *string
	timePrecision         string
	cancellationSuspected bool
	status                string
	eventType             string
	tag                   *string
	url                   string
	isUpcoming            bool
	confidence            float64
	evidence              string
}

// rawObject reproduces the property order of the JavaScript event object so
// raw_json keeps one stable form across stored rows.
func (e normalizedEvent) rawObject() *object {
	pairs := []any{
		"id", e.id,
		"canonicalSource", e.canonicalSource,
		"canonicalSourceItemId", e.canonicalSourceItemID,
		"canonicalSourceKey", e.canonicalSourceKey,
		"source_key", e.sourceKey,
		"title", e.title,
		"detail", e.detail,
		"starts_on", e.startsOn,
		"starts_at", e.startsAt,
		"ends_at", e.endsAt,
	}
	// Only a sale period carries a close date; other entries keep the
	// recorded shape of stored rows.
	if e.endsOn != nil {
		pairs = append(pairs, "ends_on", *e.endsOn)
	}
	pairs = append(pairs,
		"timezone", "Asia/Tokyo",
		"time_precision", e.timePrecision,
		"cancellationSuspected", e.cancellationSuspected,
		"status", e.status,
		"event_type", e.eventType,
	)
	// An unlabelled entry keeps the recorded shape of stored rows.
	if e.tag != nil {
		pairs = append(pairs, "tag", *e.tag)
	}
	pairs = append(pairs,
		"url", e.url,
		"is_upcoming", e.isUpcoming,
		"confidence", e.confidence,
		"raw", obj("evidence", e.evidence),
	)
	return obj(pairs...)
}

var (
	timePrecisions  = map[string]bool{"exact": true, "approximate": true, "unknown": true}
	eventTypes      = map[string]bool{"event": true, "stream": true, "member": true, "release": true, "appearance": true, "sale": true}
	classifications = map[string]bool{"schedule": true, "not_schedule": true, "uncertain": true}
	actions         = map[string]bool{"add": true, "update": true, "cancel": true, "none": true}
)

// validClock accepts a missing, null or empty time, which all mean the time
// is not known, or HH:MM up to 29:59.
func validClock(value any, defined bool) bool {
	return !defined || value == nil || value == "" || timePattern.MatchString(jsString(value))
}

// extractedTag keeps a model label only when it is one of the known codes.
// The model is asked for null when the entry does not say, so a missing or
// null field means "no label" rather than an error. An unknown value, which a
// provider without strict schema output may return, is dropped the same way
// and leaves the rest of the entry valid.
func extractedTag(value any) *string {
	text, ok := value.(string)
	if !ok || !sqlite.ValidEventTag(text) {
		return nil
	}
	return &text
}

// rolledYear moves a date the model placed about a year before the post to
// the following year, when that lands between a week before and three months
// after the post: a December notice that says 1/5 means the coming January,
// not the past one. A date that is old either way, such as an anniversary or
// a look back at the summer, stays. A zero published time leaves every date
// alone.
func rolledYear(date string, published time.Time) string {
	if published.IsZero() {
		return date
	}
	year, month, day, ok := parseDate(date)
	if !ok || month == 2 && day == 29 {
		return date
	}
	posted := published.In(jst)
	postedDay := time.Date(posted.Year(), posted.Month(), posted.Day(), 0, 0, 0, 0, time.UTC)
	value := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	if !value.Before(postedDay.AddDate(0, 0, -180)) {
		return date
	}
	next := value.AddDate(1, 0, 0)
	if next.Before(postedDay.AddDate(0, 0, -7)) || next.After(postedDay.AddDate(0, 0, 92)) {
		return date
	}
	return next.Format("2006-01-02")
}

// saleEndDate returns the rolled close date of a sale entry, "" when the
// entry is not a sale or names no close, and ok=false when the close is not a
// calendar date or falls before the opening date. Every other type ignores
// endDate: only a sale stores a period.
func saleEndDate(event *object, eventType string, startDate string, published time.Time) (string, bool) {
	value, defined := event.get("endDate")
	if eventType != "sale" || !defined || value == nil || value == "" {
		return "", true
	}
	text, isString := value.(string)
	if !isString {
		return "", false
	}
	if _, _, _, ok := parseDate(text); !ok {
		return "", false
	}
	end := rolledYear(text, published)
	if end < rolledYear(startDate, published) {
		return "", false
	}
	return end, true
}

func normalizeExtractedEvents(items []any, postID, postURL string, published, now time.Time) ([]normalizedEvent, error) {
	for _, item := range items {
		event, _ := item.(*object)
		if event == nil {
			return nil, fail("schedule extraction contained an invalid event")
		}
		title, _ := event.get("title")
		date, _ := event.get("date")
		clock, clockDefined := event.get("time")
		endClock, endDefined := event.get("endTime")
		precision, _ := event.get("timePrecision")
		eventType, _ := event.get("eventType")
		confidenceValue, confidenceDefined := event.get("confidence")
		confidence := jsNumber(confidenceValue, confidenceDefined)
		precisionText, precisionIsString := precision.(string)
		typeText, typeIsString := eventType.(string)
		_, _, _, dateOK := parseDate(stringOr(date, ""))
		endDate, endDateOK := "", false
		if dateOK && typeIsString {
			endDate, endDateOK = saleEndDate(event, typeText, stringOr(date, ""), published)
		}
		// An end time needs a start time, except the deadline of a sale period
		// whose close date is given.
		valid := jsTrim(stringOr(title, "")) != "" &&
			dateOK && endDateOK &&
			validClock(clock, clockDefined) &&
			validClock(endClock, endDefined) &&
			!(truthy(endClock) && !truthy(clock) && endDate == "") &&
			precisionIsString && timePrecisions[precisionText] &&
			typeIsString && eventTypes[typeText] &&
			finite(confidence) && confidence >= 0 && confidence <= 1
		if !valid {
			return nil, fail("schedule extraction contained an invalid event")
		}
	}
	out := make([]normalizedEvent, 0, len(items))
	for _, item := range items {
		event := item.(*object)
		titleValue, _ := event.get("title")
		dateValue, _ := event.get("date")
		clockValue, _ := event.get("time")
		endValue, _ := event.get("endTime")
		precision, _ := event.get("timePrecision")
		eventType, _ := event.get("eventType")
		tagValue, _ := event.get("tag")
		statusValue, _ := event.get("status")
		detailValue, _ := event.get("detail")
		urlValue, _ := event.get("url")
		evidenceValue, _ := event.get("evidence")
		confidenceValue, confidenceDefined := event.get("confidence")

		title := sliceUTF16(jsTrim(jsString(titleValue)), 240)
		// postedDate is the date the post counts the times from; a late-night
		// start such as 25:00 belongs to the next calendar date.
		postedDate := rolledYear(jsString(dateValue), published)
		date := postedDate
		clock := ""
		var start time.Time
		if truthy(clockValue) {
			start = japanInstant(postedDate, jsString(clockValue))
			date, clock = start.In(jst).Format("2006-01-02"), start.In(jst).Format("15:04")
		}
		clockLabel := clock
		if clockLabel == "" {
			clockLabel = "unknown"
		}
		sourceKey := date + "T" + clockLabel + ":" + sha256Hex(jsToLower(title))[:10]
		eventURL := ""
		if truthy(urlValue) {
			eventURL = jsString(urlValue)
		}
		youtubeID := youtubeIDFromURL(eventURL)
		normalized := normalizedEvent{
			id:                    "x-schedule-" + strings.ReplaceAll(date, "-", "") + "-" + sha256Hex(sourceKey)[:10],
			canonicalSource:       "x",
			canonicalSourceItemID: postID,
			canonicalSourceKey:    sourceKey,
			sourceKey:             sourceKey,
			title:                 title,
			startsOn:              date,
			timePrecision:         "unknown",
			eventType:             eventType.(string),
			tag:                   extractedTag(tagValue),
			url:                   normalizePublicURL(eventURL, postURL),
			isUpcoming:            true,
			evidence:              sliceUTF16(stringOr(evidenceValue, ""), 1000),
		}
		if youtubeID != "" {
			normalized.id = "youtube-" + youtubeID
			normalized.canonicalSource = "youtube"
			normalized.canonicalSourceItemID = youtubeID
			normalized.canonicalSourceKey = "reservation"
		}
		if truthy(detailValue) {
			detail := sliceUTF16(jsTrim(jsString(detailValue)), 500)
			normalized.detail = &detail
		}
		if clock != "" {
			startsAt := sqlite.ISOTime(start)
			normalized.startsAt = &startsAt
			normalized.timePrecision = precision.(string)
			normalized.isUpcoming = !start.Before(now.Truncate(time.Millisecond))
		}
		endDate, _ := saleEndDate(event, normalized.eventType, jsString(dateValue), published)
		switch {
		case endDate != "" && truthy(endValue):
			// A sale closes at its stated date and time; 24:00 belongs to the
			// next calendar date, as a start time does.
			end := japanInstant(endDate, jsString(endValue))
			endsOn := end.In(jst).Format("2006-01-02")
			normalized.endsOn = &endsOn
			if clock == "" || !end.Before(start) {
				endsAt := sqlite.ISOTime(end)
				normalized.endsAt = &endsAt
			}
		case endDate != "":
			normalized.endsOn = &endDate
		case truthy(endValue):
			end := japanInstant(postedDate, jsString(endValue))
			if clock != "" && end.Before(start) {
				end = end.AddDate(0, 0, 1)
			}
			endsAt := sqlite.ISOTime(end)
			normalized.endsAt = &endsAt
			// A sale's close time always has its close date, the date it
			// falls on in Japan.
			if normalized.eventType == "sale" {
				endsOn := end.In(jst).Format("2006-01-02")
				normalized.endsOn = &endsOn
			}
		}
		if normalized.endsOn != nil && *normalized.endsOn < normalized.startsOn {
			normalized.endsOn, normalized.endsAt = nil, nil
		}
		normalized.cancellationSuspected = isCancelledEventStatus(statusValue)
		status := "pending"
		if !normalized.cancellationSuspected {
			status = stringOr(statusValue, "pending")
		}
		normalized.status = sliceUTF16(jsTrim(status), 80)
		confidence := jsNumber(confidenceValue, confidenceDefined)
		if math.IsNaN(confidence) {
			confidence = 0
		}
		normalized.confidence = math.Min(1, math.Max(0, confidence))
		out = append(out, normalized)
	}
	return out, nil
}

// inferredClassification accepts results without the classification field
// when they carry an events list: those are schedules exactly when the list
// has entries. A result that is not an object, or names neither, is invalid.
func inferredClassification(result any) (string, error) {
	root, isObject := result.(*object)
	if !isObject || root == nil {
		return "", fail("schedule extraction did not return an object")
	}
	if value, ok := root.get("classification"); ok {
		if text, isString := value.(string); isString {
			normalized := jsToLower(jsTrim(text))
			if classifications[normalized] {
				return normalized, nil
			}
			return "", fail("schedule extraction returned an invalid classification")
		}
	}
	events, ok := root.get("events")
	if !ok || events == nil {
		return "", fail("schedule extraction returned no classification")
	}
	if list, isArray := events.([]any); isArray && len(list) > 0 {
		return "schedule", nil
	}
	return "not_schedule", nil
}

type normalizedExtraction struct {
	classification string
	events         []normalizedEvent
	confidence     float64
	evidence       string
	reason         string
	action         string
}

func normalizeExtractionResult(result any, postID, postURL string, published, now time.Time) (normalizedExtraction, error) {
	classification, err := inferredClassification(result)
	if err != nil {
		return normalizedExtraction{}, err
	}
	root, _ := result.(*object)
	var items []any
	if value, _ := root.get("events"); value != nil {
		list, isArray := value.([]any)
		if !isArray {
			return normalizedExtraction{}, fail("schedule extraction returned non-array events")
		}
		items = list
	}
	events, err := normalizeExtractedEvents(items, postID, postURL, published, now)
	if err != nil {
		return normalizedExtraction{}, err
	}
	if classification == "not_schedule" && len(events) > 0 {
		return normalizedExtraction{}, fail("not_schedule extraction contained events")
	}
	if classification == "uncertain" && len(events) > 0 {
		return normalizedExtraction{}, fail("uncertain extraction contained events")
	}
	confidence := 0.0
	if value, _ := root.get("confidence"); value == nil {
		for _, event := range events {
			confidence = math.Max(confidence, event.confidence)
		}
	} else {
		confidence = jsNumber(value, true)
	}
	if !finite(confidence) || confidence < 0 || confidence > 1 {
		return normalizedExtraction{}, fail("schedule extraction returned an invalid confidence")
	}
	fallbackAction := "none"
	if classification == "schedule" {
		fallbackAction = "add"
	}
	actionValue, _ := root.get("action")
	action := jsToLower(jsTrim(stringOr(actionValue, fallbackAction)))
	if !actions[action] {
		return normalizedExtraction{}, fail("schedule extraction returned an invalid action")
	}
	// A schedule without entries would retire every automatic event of the
	// post; only cancellation evidence may come without entries.
	if classification == "schedule" && len(events) == 0 && action != "cancel" {
		return normalizedExtraction{}, fail("schedule extraction contained no events")
	}
	evidence, _ := root.get("evidence")
	reason, _ := root.get("reason")
	return normalizedExtraction{
		classification: classification,
		events:         events,
		confidence:     confidence,
		evidence:       sliceUTF16(jsTrim(stringOr(evidence, "")), 1000),
		reason:         sliceUTF16(jsTrim(stringOr(reason, "")), 500),
		action:         action,
	}, nil
}

type assetVerification struct {
	classification string
	confidence     float64
	evidence       string
	reason         string
}

func normalizeAssetVerification(result any) (assetVerification, error) {
	root, _ := result.(*object)
	value, _ := root.get("classification")
	classification := jsToLower(jsTrim(stringOr(value, "")))
	if !classifications[classification] {
		return assetVerification{}, fail("schedule image verification returned an invalid classification")
	}
	confidenceValue, defined := root.get("confidence")
	confidence := jsNumber(confidenceValue, defined)
	if !finite(confidence) || confidence < 0 || confidence > 1 {
		return assetVerification{}, fail("schedule image verification returned an invalid confidence")
	}
	evidence, _ := root.get("evidence")
	reason, _ := root.get("reason")
	return assetVerification{
		classification: classification,
		confidence:     confidence,
		evidence:       sliceUTF16(jsTrim(stringOr(evidence, "")), 1000),
		reason:         sliceUTF16(jsTrim(stringOr(reason, "")), 500),
	}, nil
}

// parseInstant mirrors Date.parse for the timestamp shapes stored in the
// snapshot. ok=false stands for NaN.
func parseInstant(value string) (time.Time, bool) {
	if value == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04Z07:00"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed, true
		}
	}
	if parsed, err := time.Parse("2006-01-02", value); err == nil {
		return parsed, true
	}
	for _, layout := range []string{"2006-01-02T15:04:05.999999999", "2006-01-02T15:04", "2006-01-02 15:04:05.999999999", "2006-01-02 15:04"} {
		if parsed, err := time.ParseInLocation(layout, value, time.Local); err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

// instantMillis returns epoch milliseconds, or 0 when the value is not a date.
func instantMillis(value string) int64 {
	parsed, ok := parseInstant(value)
	if !ok {
		return 0
	}
	return parsed.UnixMilli()
}
