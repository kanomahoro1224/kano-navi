package sqlite

import (
	"reflect"
	"testing"
	"time"
)

// projNow is 13:00 on 2026-11-02 in Japan.
var projNow = time.Date(2026, 11, 2, 4, 0, 0, 0, time.UTC)

func projAt(offset time.Duration) *string {
	value := projNow.Add(offset).Format(time.RFC3339)
	return &value
}

func projTime(value string) time.Time {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		panic(err)
	}
	return parsed
}

// projEventSpec describes one synthetic event; empty fields take the
// defaults of an exact, scheduled X entry starting on its start's Japan date.
type projEventSpec struct {
	eventType, tag, source, status, cancellation, liveStatus string
	precision, startsOn, endsOn, url                         string
	start, end                                               *string
}

func (spec projEventSpec) event(id string) DashboardEvent {
	event := DashboardEvent{
		ID: id, Source: "x", Title: id, StartsOn: dateInJapan(projNow), StartsAt: spec.start, EndsAt: spec.end,
		TimePrecision: "exact", Status: strPtr("scheduled"), CancellationStatus: "none",
		URL: strPtr("https://example.com/event/" + id),
	}
	if spec.start != nil {
		event.StartsOn = dateInJapan(projTime(*spec.start))
	}
	if spec.eventType != "" {
		event.EventType = strPtr(spec.eventType)
	}
	if spec.tag != "" {
		event.Tag = strPtr(spec.tag)
	}
	if spec.source != "" {
		event.Source = spec.source
	}
	if spec.status != "" {
		event.Status = strPtr(spec.status)
	}
	if spec.cancellation != "" {
		event.CancellationStatus = spec.cancellation
	}
	if spec.liveStatus != "" {
		event.LiveStatus = strPtr(spec.liveStatus)
	}
	if spec.precision != "" {
		event.TimePrecision = spec.precision
	}
	if spec.startsOn != "" {
		event.StartsOn = spec.startsOn
	}
	if spec.endsOn != "" {
		event.EndsOn = strPtr(spec.endsOn)
	}
	if spec.url != "" {
		event.URL = strPtr(spec.url)
	}
	return event
}

// projVideoSpec describes one synthetic video. A scheduled video is upcoming
// while its start is ahead, as the readers set it; waiting marks a waiting
// room without a start.
type projVideoSpec struct {
	kind, tag, liveStatus, availability, thumbnail string
	start, end, published                          *string
	short, cancelled, waiting                      bool
}

func (spec projVideoSpec) video(id string) Video {
	video := Video{
		ID: id, Source: "youtube", Title: id, URL: "https://example.com/video/" + id,
		ScheduledAt: spec.start, EndsAt: spec.end, PublishedAt: spec.published,
		IsCancelled: spec.cancelled, IsUpcoming: spec.waiting,
	}
	if spec.start != nil {
		video.IsUpcoming = videoUpcoming(video, projNow)
	}
	if spec.kind != "" {
		video.Kind = strPtr(spec.kind)
	}
	if spec.tag != "" {
		video.Tag = strPtr(spec.tag)
	}
	if spec.liveStatus != "" {
		video.LiveStatus = strPtr(spec.liveStatus)
	}
	if spec.availability != "" {
		video.Availability = strPtr(spec.availability)
	}
	if spec.thumbnail != "" {
		video.ThumbnailURL = strPtr(spec.thumbnail)
	}
	if spec.short {
		short := true
		video.IsShort = &short
	}
	return video
}

func projString(value *string) string {
	if value == nil {
		return "<nil>"
	}
	return *value
}

// projSale describes a shop sale opening at start (or on startsOn when start
// is nil) and closing on endsOn, at end when it is set.
func projSale(start *string, startsOn, endsOn string, end *string) projEventSpec {
	return projEventSpec{eventType: "sale", source: "shop", start: start, startsOn: startsOn, endsOn: endsOn, end: end}
}

func TestEventViewStates(t *testing.T) {
	sale := projSale
	deadline := sale(nil, "2026-11-02", "2026-11-02", strPtr("2026-11-02T14:59:00Z"))
	deadline.precision = "unknown"
	closesThatDay := sale(strPtr("2026-10-30T03:00:00Z"), "", "2026-11-02", nil)
	for _, tc := range []struct {
		name string
		spec projEventSpec
		at   time.Time
		want string
	}{
		{"stream ahead", projEventSpec{eventType: "stream", start: projAt(time.Hour)}, projNow, "upcoming"},
		{"date-only entry today", projEventSpec{eventType: "event"}, projNow, "upcoming"},
		{"waiting room past its start", projEventSpec{eventType: "stream", start: projAt(-time.Hour), liveStatus: "upcoming"}, projNow, "upcoming"},
		{"stream confirmed on air", projEventSpec{eventType: "stream", start: projAt(-time.Hour), liveStatus: "live"}, projNow, "live"},
		{"stream confirmed on air before its start", projEventSpec{eventType: "stream", start: projAt(time.Hour), liveStatus: "live"}, projNow, "live"},
		{"date-only membership confirmed on air", projEventSpec{eventType: "member", precision: "unknown", liveStatus: "live"}, projNow, "live"},
		{"date-only stream on its day", projEventSpec{eventType: "stream", precision: "unknown"}, projTime("2026-11-02T14:59:00Z"), "upcoming"},
		// A date-only entry has no time to go on air at, so the live grace
		// never carries it into the next day.
		{"date-only stream after its day", projEventSpec{eventType: "stream", startsOn: "2026-11-01", precision: "unknown"}, projTime("2026-11-01T16:00:00Z"), "pending"},
		{"date-only membership after its day", projEventSpec{eventType: "member", startsOn: "2026-11-01", precision: "exact"}, projTime("2026-11-01T17:00:00Z"), "done"},
		{"confirmed on air past its end", projEventSpec{eventType: "stream", start: projAt(-2 * time.Hour), end: projAt(-time.Hour), liveStatus: "live"}, projNow, "done"},
		{"stream presumed live inside the grace", projEventSpec{eventType: "stream", start: projAt(-2 * time.Hour)}, projNow, "live"},
		{"membership presumed live inside the grace", projEventSpec{eventType: "member", start: projAt(-2 * time.Hour)}, projNow, "live"},
		{"ended stream inside the grace", projEventSpec{eventType: "stream", start: projAt(-time.Hour), liveStatus: "ended"}, projNow, "done"},
		{"stream past the grace", projEventSpec{eventType: "stream", start: projAt(-4 * time.Hour)}, projNow, "done"},
		{"premiere on air", projEventSpec{eventType: "release", source: "youtube", start: projAt(-30 * time.Minute), end: projAt(30 * time.Minute)}, projNow, "live"},
		{"premiere aired", projEventSpec{eventType: "release", source: "youtube", start: projAt(-2 * time.Hour), end: projAt(-time.Hour)}, projNow, "done"},
		{"premiere without an end gets no grace", projEventSpec{eventType: "release", source: "youtube", start: projAt(-time.Hour)}, projNow, "done"},
		{"past entry", projEventSpec{eventType: "event", start: projAt(-time.Hour)}, projNow, "done"},
		{"offline event between its start and end", projEventSpec{eventType: "event", start: projAt(-time.Hour), end: projAt(time.Hour)}, projNow, "ongoing"},
		{"offline event at its end", projEventSpec{eventType: "event", start: projAt(-time.Hour), end: projAt(0)}, projNow, "done"},
		{"tentative event between its start and end", projEventSpec{eventType: "event", start: projAt(-time.Hour), end: projAt(time.Hour), status: "tentative"}, projNow, "pending"},
		{"stream ended before its stored end", projEventSpec{eventType: "stream", start: projAt(-time.Hour), end: projAt(time.Hour), liveStatus: "ended"}, projNow, "done"},
		{"stream between its start and end", projEventSpec{eventType: "stream", start: projAt(-time.Hour), end: projAt(time.Hour)}, projNow, "live"},
		{"past date-only entry of unknown time", projEventSpec{eventType: "event", startsOn: "2026-11-01", precision: "unknown"}, projNow, "pending"},
		{"past tentative entry", projEventSpec{eventType: "event", start: projAt(-time.Hour), status: "tentative"}, projNow, "pending"},
		{"past entry awaiting confirmation", projEventSpec{eventType: "event", start: projAt(-time.Hour), status: "未定"}, projNow, "pending"},
		{"operator-confirmed cancellation", projEventSpec{eventType: "stream", start: projAt(time.Hour), cancellation: "manual_confirmed"}, projNow, "cancelled"},
		{"cancelled status", projEventSpec{eventType: "stream", start: projAt(time.Hour), status: "Canceled"}, projNow, "cancelled"},
		{"suspected cancellation", projEventSpec{eventType: "stream", start: projAt(time.Hour), cancellation: "llm_suspected"}, projNow, "cancellation_review"},
		// A cancelled status is definitive whatever the model suspects.
		{"cancelled status under a suspicion", projEventSpec{eventType: "stream", start: projAt(time.Hour), status: "cancelled", cancellation: "llm_suspected"}, projNow, "cancelled"},
		{"sale with only an opening date", sale(nil, "2026-11-04", "", nil), projNow, "upcoming"},
		{"sale before its opening time", sale(projAt(time.Hour), "", "2026-11-10", nil), projNow, "upcoming"},
		{"sale before its opening date", sale(nil, "2026-11-03", "2026-11-10", nil), projNow, "upcoming"},
		{"sale opening at Japan midnight", sale(nil, "2026-11-03", "2026-11-10", nil), projTime("2026-11-02T15:00:00Z"), "open"},
		{"sale period open", sale(projAt(-time.Hour), "", "2026-11-10", strPtr("2026-11-10T14:59:00Z")), projNow, "open"},
		{"sale period closed", sale(projAt(-48*time.Hour), "", "2026-11-02", projAt(-time.Hour)), projNow, "done"},
		{"deadline days ahead", deadline, projTime("2026-10-29T00:00:00Z"), "open"},
		{"deadline open until it passes", deadline, projTime("2026-11-02T14:58:59Z"), "open"},
		{"deadline passed", deadline, projTime("2026-11-02T14:59:00Z"), "done"},
		{"close date without a time open that Japan day", closesThatDay, projTime("2026-11-02T14:59:59Z"), "open"},
		{"close date without a time ends that Japan day", closesThatDay, projTime("2026-11-02T15:00:00Z"), "done"},
		{"cancelled sale", projEventSpec{eventType: "sale", source: "shop", start: projAt(-time.Hour), endsOn: "2026-11-10", cancellation: "manual_confirmed"}, projNow, "cancelled"},
		{"sale under cancellation review", projEventSpec{eventType: "sale", source: "shop", start: projAt(-time.Hour), endsOn: "2026-11-10", cancellation: "llm_suspected"}, projNow, "cancellation_review"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			event := tc.spec.event("entry")
			applyEventView(&event, tc.at)
			if event.View.State != tc.want {
				t.Fatalf("state = %q, want %q", event.View.State, tc.want)
			}
		})
	}
}

func TestEventViewTypeAndActivityCodes(t *testing.T) {
	for _, tc := range []struct {
		eventType, tag string
		typeCode, code string
		context        string
		isWork         bool
	}{
		{"stream", "", "stream", "", "", false},
		// An activity label replaces the plain stream chip.
		{"stream", "song", "", "song", "stream", false},
		{"stream", "game", "", "game", "stream", false},
		// Membership, event and release keep their type beside the label.
		{"member", "game", "member", "game", "stream", false},
		{"event", "talk", "event", "talk", "stream", false},
		{"event", "live", "event", "live", "stream", false},
		{"release", "song", "release", "song", "work", true},
		{"release", "video", "release", "video", "work", true},
		{"video", "song", "video", "song", "work", true},
		{"sale", "", "sale", "", "", false},
		// A label repeating its type is dropped.
		{"stream", "live", "stream", "", "", false},
		{"member", "live", "member", "", "", false},
		{"video", "video", "video", "", "", true},
		// Unknown values have no code; known ones are matched loosely.
		{"stream", "dance", "stream", "", "", false},
		{"concert", "song", "", "song", "stream", false},
		{"", "", "", "", "", false},
		{" Release ", "SONG", "release", "song", "work", true},
	} {
		t.Run(tc.eventType+"+"+tc.tag, func(t *testing.T) {
			event := projEventSpec{eventType: tc.eventType, tag: tc.tag, start: projAt(time.Hour)}.event("entry")
			applyEventView(&event, projNow)
			view := event.View
			if got := projString(view.TypeCode); got != projString(projOptional(tc.typeCode)) {
				t.Errorf("typeCode = %s, want %q", got, tc.typeCode)
			}
			if got := projString(view.ActivityCode); got != projString(projOptional(tc.code)) {
				t.Errorf("activityCode = %s, want %q", got, tc.code)
			}
			if view.ActivityContext != tc.context || view.IsWork != tc.isWork {
				t.Errorf("activityContext = %q, isWork = %v", view.ActivityContext, view.IsWork)
			}
		})
	}
}

func projOptional(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func TestEventViewCalendarEntries(t *testing.T) {
	opens, closes := strPtr("opens"), strPtr("closes")
	for _, tc := range []struct {
		name string
		spec projEventSpec
		want []CalendarEntry
	}{
		{
			"ordinary entry",
			projEventSpec{eventType: "stream", start: strPtr("2026-11-03T11:00:00Z")},
			[]CalendarEntry{{StartsOn: "2026-11-03", StartsAt: strPtr("2026-11-03T11:00:00Z"), TimePrecision: "exact"}},
		},
		{
			"ordinary date-only entry",
			projEventSpec{eventType: "event", startsOn: "2026-11-04", precision: "unknown"},
			[]CalendarEntry{{StartsOn: "2026-11-04", TimePrecision: "unknown"}},
		},
		{
			"sale period",
			projEventSpec{eventType: "sale", start: strPtr("2026-10-28T03:00:00Z"), endsOn: "2026-11-22", end: strPtr("2026-11-22T14:59:00Z")},
			[]CalendarEntry{
				{SaleMark: opens, StartsOn: "2026-10-28", StartsAt: strPtr("2026-10-28T03:00:00Z"), TimePrecision: "exact"},
				{SaleMark: closes, StartsOn: "2026-11-22", StartsAt: strPtr("2026-11-22T14:59:00Z"), TimePrecision: "exact"},
			},
		},
		{
			"sale period closing on a date without a time",
			projEventSpec{eventType: "sale", startsOn: "2026-10-28", precision: "unknown", endsOn: "2026-11-22"},
			[]CalendarEntry{
				{SaleMark: opens, StartsOn: "2026-10-28", TimePrecision: "unknown"},
				{SaleMark: closes, StartsOn: "2026-11-22", TimePrecision: "unknown"},
			},
		},
		{
			"deadline",
			projEventSpec{eventType: "sale", startsOn: "2026-11-05", precision: "unknown", endsOn: "2026-11-05", end: strPtr("2026-11-05T14:59:00Z")},
			[]CalendarEntry{{SaleMark: closes, StartsOn: "2026-11-05", StartsAt: strPtr("2026-11-05T14:59:00Z"), TimePrecision: "exact"}},
		},
		{
			"opening without a known close",
			projEventSpec{eventType: "sale", start: strPtr("2026-11-06T03:00:00Z")},
			[]CalendarEntry{{SaleMark: opens, StartsOn: "2026-11-06", StartsAt: strPtr("2026-11-06T03:00:00Z"), TimePrecision: "exact"}},
		},
		{
			// Without a close date the date is the opening, not a deadline.
			"opening date without a known close",
			projEventSpec{eventType: "sale", startsOn: "2026-11-06", precision: "unknown"},
			[]CalendarEntry{{SaleMark: opens, StartsOn: "2026-11-06", TimePrecision: "unknown"}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			event := tc.spec.event("entry")
			applyEventView(&event, projNow)
			if !reflect.DeepEqual(event.View.CalendarEntries, tc.want) {
				t.Fatalf("entries = %s, want %s", projEntries(event.View.CalendarEntries), projEntries(tc.want))
			}
		})
	}
}

func projEntries(entries []CalendarEntry) []string {
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		out = append(out, projString(entry.SaleMark)+" "+entry.StartsOn+" "+projString(entry.StartsAt)+" "+entry.TimePrecision)
	}
	return out
}

func TestVideoViewPhaseAndForm(t *testing.T) {
	for _, tc := range []struct {
		name   string
		spec   projVideoSpec
		phase  string
		form   string
		latest bool
		focus  bool
	}{
		{"cancelled reservation", projVideoSpec{kind: "stream", start: projAt(time.Hour), cancelled: true}, "cancelled", "upcoming", false, false},
		{"stream reservation", projVideoSpec{kind: "stream", start: projAt(time.Hour)}, "upcoming", "upcoming", false, true},
		{"waiting room without a start", projVideoSpec{kind: "stream", waiting: true}, "upcoming", "upcoming", false, true},
		{"waiting room past its start", projVideoSpec{kind: "stream", start: projAt(-time.Hour), liveStatus: "upcoming"}, "upcoming", "upcoming", false, true},
		{"stream on air", projVideoSpec{kind: "stream", start: projAt(-time.Hour), liveStatus: "live"}, "live", "stream", true, true},
		{"stream on air past its end", projVideoSpec{kind: "stream", start: projAt(-2 * time.Hour), end: projAt(-time.Hour), liveStatus: "live"}, "past", "archive", true, false},
		{"stream presumed on air inside the grace", projVideoSpec{kind: "stream", start: projAt(-2 * time.Hour)}, "started", "stream", true, true},
		{"ended stream inside the grace", projVideoSpec{kind: "stream", start: projAt(-time.Hour), liveStatus: "ended"}, "past", "archive", true, false},
		{"stream past the grace", projVideoSpec{kind: "stream", start: projAt(-4 * time.Hour)}, "past", "archive", true, false},
		{"stream without a start", projVideoSpec{kind: "stream", published: projAt(-time.Hour)}, "past", "archive", true, false},
		{"premiere ahead", projVideoSpec{kind: "premiere", start: projAt(time.Hour)}, "upcoming", "premiere", false, true},
		{"premiere showing", projVideoSpec{kind: "premiere", start: projAt(-30 * time.Minute), end: projAt(30 * time.Minute)}, "started", "premiere", true, true},
		{"premiere confirmed on air", projVideoSpec{kind: "premiere", start: projAt(-30 * time.Minute), liveStatus: "live"}, "live", "premiere", true, true},
		{"premiere aired", projVideoSpec{kind: "premiere", start: projAt(-2 * time.Hour), end: projAt(-time.Hour)}, "past", "", true, false},
		// Without an end or a confirmed broadcast a premiere has aired at its
		// start, and no longer competes for the stage.
		{"premiere past its start without an end", projVideoSpec{kind: "premiere", start: projAt(-time.Hour)}, "past", "", true, false},
		{"upload", projVideoSpec{kind: "video", published: projAt(-24 * time.Hour)}, "past", "video", true, false},
		{"upload of unknown kind", projVideoSpec{published: projAt(-24 * time.Hour)}, "past", "video", true, false},
		{"private upload", projVideoSpec{kind: "video", published: projAt(-24 * time.Hour), availability: "private"}, "past", "video", false, false},
		{"Short before its stream form", projVideoSpec{kind: "stream", start: projAt(time.Hour), short: true}, "upcoming", "short", false, true},
		{"Short upload", projVideoSpec{kind: "video", published: projAt(-time.Hour), short: true}, "past", "short", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			video := tc.spec.video("entry")
			applyVideoView(&video, projNow)
			view := video.View
			if view.Phase != tc.phase || projString(view.FormCode) != projString(projOptional(tc.form)) {
				t.Fatalf("phase = %q, form = %s; want %q, %q", view.Phase, projString(view.FormCode), tc.phase, tc.form)
			}
			if view.LatestEligible != tc.latest || view.FocusEligible != tc.focus {
				t.Fatalf("latestEligible = %v, focusEligible = %v; want %v, %v", view.LatestEligible, view.FocusEligible, tc.latest, tc.focus)
			}
			wantContext := "work"
			if tc.spec.kind == "stream" {
				wantContext = "stream"
			}
			if view.TagContext != wantContext {
				t.Fatalf("tagContext = %q, want %q", view.TagContext, wantContext)
			}
		})
	}
}

func projShelf(t *testing.T, featured string, specs map[string]projVideoSpec) *Dashboard {
	t.Helper()
	dashboard := &Dashboard{}
	if featured != "" {
		dashboard.Meta.FeaturedVideoID = strPtr(featured)
	}
	// Map order is random, so the shelf cannot lean on the input order.
	for id, spec := range specs {
		dashboard.Videos = append(dashboard.Videos, spec.video(id))
	}
	applyDashboardProjection(dashboard, projNow)
	return dashboard
}

func projGroups(dashboard *Dashboard) map[string]string {
	groups := map[string]string{}
	for _, video := range dashboard.Videos {
		groups[video.ID] = video.View.ShelfGroup
	}
	return groups
}

func TestShelfGroupsAndOrdersEveryList(t *testing.T) {
	dashboard := projShelf(t, "", map[string]projVideoSpec{
		"res-late":       {kind: "stream", start: projAt(48 * time.Hour)},
		"res-soon":       {kind: "stream", start: projAt(time.Hour)},
		"res-live":       {kind: "stream", start: projAt(-time.Hour), liveStatus: "live"},
		"res-waiting":    {kind: "stream", waiting: true, published: projAt(-72 * time.Hour)},
		"res-cancelled":  {kind: "stream", start: projAt(2 * time.Hour), cancelled: true},
		"gone-cancelled": {kind: "stream", start: projAt(-2 * time.Hour), cancelled: true},
		"song-old":       {kind: "video", tag: "song", published: projAt(-72 * time.Hour)},
		"upload-new":     {kind: "video", published: projAt(-time.Hour)},
		"upload-mid":     {kind: "video", published: projAt(-24 * time.Hour)},
		"archive-old":    {kind: "stream", start: projAt(-96 * time.Hour)},
		"archive-new":    {kind: "stream", start: projAt(-10 * time.Hour)},
		"short-old":      {kind: "video", short: true, published: projAt(-48 * time.Hour)},
		"short-new":      {kind: "video", short: true, published: projAt(-2 * time.Hour)},
	})
	shelf := dashboard.Projection.Shelf
	// Reservations run earliest first; a waiting room without a start sorts
	// by its publication. A cancelled reservation stays until its start.
	want := DashboardShelf{
		Reservations: []string{"res-waiting", "res-live", "res-soon", "res-cancelled", "res-late"},
		Works:        []string{"upload-new", "upload-mid"},
		Archives:     []string{"archive-new", "archive-old"},
		Shorts:       []string{"short-new", "short-old"},
		LeadID:       strPtr("song-old"),
	}
	if !reflect.DeepEqual(shelf, want) {
		t.Fatalf("shelf = %+v (lead %s)", shelf, projString(shelf.LeadID))
	}
	groups := projGroups(dashboard)
	for id, group := range map[string]string{
		"res-live": "reservation", "res-cancelled": "reservation", "gone-cancelled": "",
		"song-old": "work", "upload-mid": "work", "archive-old": "archive", "short-new": "short",
	} {
		if groups[id] != group {
			t.Errorf("shelfGroup[%s] = %q, want %q", id, groups[id], group)
		}
	}
}

func TestShelfLead(t *testing.T) {
	for _, tc := range []struct {
		name     string
		featured string
		specs    map[string]projVideoSpec
		lead     string
		group    string
	}{
		{
			"newest watchable song over newer uploads",
			"",
			map[string]projVideoSpec{
				"upload":       {kind: "video", published: projAt(-time.Hour)},
				"song-private": {kind: "video", tag: "song", published: projAt(-2 * time.Hour), availability: "private"},
				"song":         {kind: "video", tag: "Song", published: projAt(-48 * time.Hour)},
				"song-older":   {kind: "video", tag: "song", published: projAt(-72 * time.Hour)},
			},
			"song", "work",
		},
		{
			"aired premiere counts as a song would",
			"",
			map[string]projVideoSpec{
				"upload":   {kind: "video", published: projAt(-time.Hour)},
				"premiere": {kind: "premiere", start: projAt(-24 * time.Hour), end: projAt(-23 * time.Hour)},
			},
			"premiere", "work",
		},
		{
			"newest watchable work without a song or premiere",
			"",
			map[string]projVideoSpec{
				"removed": {kind: "video", published: projAt(-time.Hour), availability: "removed"},
				"upload":  {kind: "video", published: projAt(-24 * time.Hour)},
				"older":   {kind: "video", published: projAt(-48 * time.Hour)},
			},
			"upload", "work",
		},
		{
			"no watchable work",
			"",
			map[string]projVideoSpec{
				"private": {kind: "video", tag: "song", published: projAt(-time.Hour), availability: "private"},
				"archive": {kind: "stream", start: projAt(-24 * time.Hour)},
				"short":   {kind: "video", tag: "song", short: true, published: projAt(-time.Hour)},
			},
			"", "",
		},
		{
			"Featured upload",
			"featured",
			map[string]projVideoSpec{
				"song":     {kind: "video", tag: "song", published: projAt(-time.Hour)},
				"featured": {kind: "video", published: projAt(-720 * time.Hour)},
			},
			"featured", "work",
		},
		{
			"Featured Short",
			"featured",
			map[string]projVideoSpec{
				"upload":   {kind: "video", published: projAt(-time.Hour)},
				"featured": {kind: "video", short: true, published: projAt(-2 * time.Hour)},
			},
			"featured", "short",
		},
		{
			"Featured reservation",
			"featured",
			map[string]projVideoSpec{"featured": {kind: "stream", start: projAt(time.Hour)}},
			"featured", "reservation",
		},
		{
			"Featured archive",
			"featured",
			map[string]projVideoSpec{"featured": {kind: "stream", start: projAt(-24 * time.Hour)}},
			"featured", "archive",
		},
		{
			// The lead is on the shelf, so it falls back to the work group.
			"Featured cancelled stream past its start",
			"featured",
			map[string]projVideoSpec{"featured": {kind: "stream", start: projAt(-time.Hour), cancelled: true}},
			"featured", "work",
		},
		{
			"Featured video not in the response",
			"missing",
			map[string]projVideoSpec{"upload": {kind: "video", published: projAt(-time.Hour)}},
			"upload", "work",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dashboard := projShelf(t, tc.featured, tc.specs)
			shelf := dashboard.Projection.Shelf
			if projString(shelf.LeadID) != projString(projOptional(tc.lead)) {
				t.Fatalf("lead = %s, want %q", projString(shelf.LeadID), tc.lead)
			}
			if shelf.LeadIsFeatured != (tc.lead != "" && tc.lead == tc.featured) {
				t.Fatalf("leadIsFeatured = %v", shelf.LeadIsFeatured)
			}
			if tc.lead == "" {
				return
			}
			for _, list := range [][]string{shelf.Reservations, shelf.Works, shelf.Archives, shelf.Shorts} {
				for _, id := range list {
					if id == tc.lead {
						t.Fatalf("lead %q is also listed: %+v", tc.lead, shelf)
					}
				}
			}
			if got := projGroups(dashboard)[tc.lead]; got != tc.group {
				t.Fatalf("lead shelfGroup = %q, want %q", got, tc.group)
			}
		})
	}
}

func TestSpotlightModes(t *testing.T) {
	upload := projVideoSpec{kind: "video", published: projAt(-24 * time.Hour), thumbnail: "https://example.com/image/upload.jpg"}

	empty := &Dashboard{}
	applyDashboardProjection(empty, projNow)
	if empty.Projection.Spotlight != nil {
		t.Fatalf("empty spotlight = %+v", empty.Projection.Spotlight)
	}

	latest := &Dashboard{Videos: []Video{upload.video("upload")}}
	applyDashboardProjection(latest, projNow)
	if got := latest.Projection.Spotlight; got == nil || !reflect.DeepEqual(*got, DashboardSpotlight{Mode: "latest", VideoID: strPtr("upload")}) {
		t.Fatalf("latest spotlight = %+v", got)
	}

	next := &Dashboard{
		Events: []DashboardEvent{projEventSpec{eventType: "stream", start: projAt(time.Hour)}.event("stream")},
		Videos: []Video{upload.video("upload")},
	}
	applyDashboardProjection(next, projNow)
	want := DashboardSpotlight{Mode: "next", EventID: strPtr("stream"), CompanionVideoID: strPtr("upload")}
	if got := next.Projection.Spotlight; got == nil || !reflect.DeepEqual(*got, want) {
		t.Fatalf("next spotlight = %+v", got)
	}

	// The room's unmatched broadcast is the live ticket, even with a
	// reservation minutes away.
	room := &Dashboard{
		LiveNow: &LiveNow{Platform: "bilibili", URL: "https://example.com/live/room", Title: "Room", StartedAt: projNow.Add(-30 * time.Minute).Format(time.RFC3339)},
		Events:  []DashboardEvent{projEventSpec{eventType: "stream", start: projAt(20 * time.Minute)}.event("soon")},
	}
	applyDashboardProjection(room, projNow)
	if got := room.Projection.Spotlight; got == nil || !reflect.DeepEqual(*got, DashboardSpotlight{Mode: "next", LiveNow: true, IsLive: true}) {
		t.Fatalf("liveNow spotlight = %+v", got)
	}
}

func TestSpotlightLiveYield(t *testing.T) {
	presumed := projEventSpec{eventType: "stream", start: projAt(-time.Hour)}
	for _, tc := range []struct {
		name         string
		events       []DashboardEvent
		videos       []Video
		event, video string
		isLive       bool
	}{
		{
			"presumed-live stream yields to a reservation within 30 minutes",
			[]DashboardEvent{presumed.event("presumed"), projEventSpec{eventType: "stream", start: projAt(20 * time.Minute)}.event("soon")},
			nil, "soon", "", false,
		},
		{
			"presumed-live stream keeps the stage before a later reservation",
			[]DashboardEvent{presumed.event("presumed"), projEventSpec{eventType: "stream", start: projAt(40 * time.Minute)}.event("later")},
			nil, "presumed", "", true,
		},
		{
			"confirmed live event keeps the stage",
			[]DashboardEvent{
				projEventSpec{eventType: "stream", start: projAt(-time.Hour), liveStatus: "live"}.event("live"),
				projEventSpec{eventType: "stream", start: projAt(20 * time.Minute)}.event("soon"),
			},
			nil, "live", "", true,
		},
		{
			"presumed-live video yields to a reserved video",
			nil,
			[]Video{
				projVideoSpec{kind: "stream", start: projAt(-time.Hour), thumbnail: "https://example.com/image/a.jpg"}.video("presumed"),
				projVideoSpec{kind: "stream", start: projAt(20 * time.Minute), thumbnail: "https://example.com/image/b.jpg"}.video("soon"),
			},
			"", "soon", false,
		},
		{
			"confirmed live video keeps the stage",
			nil,
			[]Video{
				projVideoSpec{kind: "stream", start: projAt(-time.Hour), liveStatus: "live", thumbnail: "https://example.com/image/a.jpg"}.video("live"),
				projVideoSpec{kind: "stream", start: projAt(20 * time.Minute), thumbnail: "https://example.com/image/b.jpg"}.video("soon"),
			},
			"", "live", true,
		},
		{
			"presumed-live event yields to a reserved video",
			[]DashboardEvent{presumed.event("presumed")},
			[]Video{projVideoSpec{kind: "stream", start: projAt(20 * time.Minute), thumbnail: "https://example.com/image/b.jpg"}.video("soon")},
			"", "soon", false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dashboard := &Dashboard{Events: tc.events, Videos: tc.videos}
			applyDashboardProjection(dashboard, projNow)
			got := dashboard.Projection.Spotlight
			if got == nil || got.Mode != "next" || projString(got.EventID) != projString(projOptional(tc.event)) ||
				projString(got.VideoID) != projString(projOptional(tc.video)) || got.IsLive != tc.isLive || got.LiveNow {
				t.Fatalf("spotlight = %+v (event %s, video %s)", got, projString(got.EventID), projString(got.VideoID))
			}
		})
	}
}

func TestSpotlightCurrentEntries(t *testing.T) {
	fair := projEventSpec{eventType: "event", start: projAt(-time.Hour), end: projAt(3 * time.Hour)}
	premiere := projVideoSpec{kind: "premiere", start: projAt(-time.Hour), thumbnail: "https://example.com/image/premiere.jpg"}
	for _, tc := range []struct {
		name         string
		at           time.Time
		events       []DashboardEvent
		videos       []Video
		mode         string
		event, video string
		isLive       bool
	}{
		{
			"stream confirmed on air before its start",
			projNow,
			[]DashboardEvent{projEventSpec{eventType: "stream", start: projAt(time.Hour), liveStatus: "live"}.event("early")},
			nil, "next", "early", "", true,
		},
		{
			"date-only stream on its day",
			projTime("2026-11-02T14:00:00Z"),
			[]DashboardEvent{projEventSpec{eventType: "stream", precision: "unknown"}.event("today")},
			nil, "next", "today", "", false,
		},
		{
			"date-only stream after its day",
			projTime("2026-11-02T16:00:00Z"),
			[]DashboardEvent{projEventSpec{eventType: "stream", precision: "unknown"}.event("today")},
			nil, "", "", "", false,
		},
		{
			"ongoing offline event, not on air",
			projNow,
			[]DashboardEvent{fair.event("fair")},
			nil, "next", "fair", "", false,
		},
		{
			"ongoing offline event before a later stream",
			projNow,
			[]DashboardEvent{fair.event("fair"), projEventSpec{eventType: "stream", start: projAt(time.Hour)}.event("later")},
			nil, "next", "fair", "", false,
		},
		{
			"ongoing offline event yields to a stream within 30 minutes",
			projNow,
			[]DashboardEvent{fair.event("fair"), projEventSpec{eventType: "stream", start: projAt(20 * time.Minute)}.event("soon")},
			nil, "next", "soon", "", false,
		},
		{
			"ongoing offline event yields to a reserved video",
			projNow,
			[]DashboardEvent{fair.event("fair")},
			[]Video{projVideoSpec{kind: "stream", start: projAt(20 * time.Minute), thumbnail: "https://example.com/image/soon.jpg"}.video("soon")},
			"next", "", "soon", false,
		},
		{
			"premiere past its start without an end",
			projNow,
			nil,
			[]Video{premiere.video("premiere")},
			"latest", "", "premiere", false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dashboard := &Dashboard{Events: tc.events, Videos: tc.videos}
			applyDashboardProjection(dashboard, tc.at)
			got := dashboard.Projection.Spotlight
			if tc.mode == "" {
				if got != nil {
					t.Fatalf("spotlight = %+v, want none", got)
				}
				return
			}
			if got == nil || got.Mode != tc.mode || projString(got.EventID) != projString(projOptional(tc.event)) ||
				projString(got.VideoID) != projString(projOptional(tc.video)) || got.IsLive != tc.isLive || got.LiveNow {
				t.Fatalf("spotlight = %+v (event %s, video %s)", got, projString(got.EventID), projString(got.VideoID))
			}
		})
	}
}

func TestSpotlightPairsAndMembersChip(t *testing.T) {
	pairedURL := "https://example.com/video/paired"
	for _, tc := range []struct {
		name                   string
		eventType, tag         string
		videoTag, availability string
		chip                   bool
	}{
		{"membership with a game label", "member", "game", "", "", true},
		{"membership with a talk label on a public video", "member", "talk", "", "public", true},
		{"membership whose video carries the song tag", "member", "", "song", "", true},
		{"membership without a label", "member", "", "", "", false},
		{"membership with the dropped live label", "member", "live", "", "", false},
		{"members-only video", "member", "game", "", "members", false},
		{"private video", "member", "game", "", "private", false},
		{"stream with a game label", "stream", "game", "", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			event := projEventSpec{eventType: tc.eventType, tag: tc.tag, start: projAt(time.Hour), url: pairedURL}.event("event")
			video := projVideoSpec{kind: "stream", start: projAt(time.Hour), tag: tc.videoTag, availability: tc.availability, thumbnail: "https://example.com/image/paired.jpg"}.video("paired")
			dashboard := &Dashboard{Events: []DashboardEvent{event}, Videos: []Video{video}}
			applyDashboardProjection(dashboard, projNow)
			got := dashboard.Projection.Spotlight
			if got == nil || projString(got.EventID) != "event" || projString(got.VideoID) != "paired" || got.MembersChip != tc.chip {
				t.Fatalf("spotlight = %+v", got)
			}
		})
	}
}

func TestSpotlightCompanionVideo(t *testing.T) {
	upload := projVideoSpec{kind: "video", published: projAt(-24 * time.Hour)}
	for _, tc := range []struct {
		name      string
		events    []DashboardEvent
		videos    []Video
		companion string
	}{
		{
			"event without a video",
			[]DashboardEvent{projEventSpec{eventType: "stream", start: projAt(time.Hour)}.event("stream")},
			[]Video{upload.video("upload")},
			"upload",
		},
		{
			"event linking the latest video",
			[]DashboardEvent{projEventSpec{eventType: "event", start: projAt(time.Hour), url: "https://example.com/video/upload"}.event("event")},
			[]Video{upload.video("upload")},
			"",
		},
		{
			"video with a thumbnail",
			nil,
			[]Video{upload.video("upload"), projVideoSpec{kind: "stream", start: projAt(time.Hour), thumbnail: "https://example.com/image/next.jpg"}.video("next")},
			"",
		},
		{
			"video without a thumbnail",
			nil,
			[]Video{upload.video("upload"), projVideoSpec{kind: "stream", start: projAt(time.Hour)}.video("next")},
			"upload",
		},
		{
			// A stream presumed on air is itself the latest video.
			"video that is the latest video",
			nil,
			[]Video{upload.video("upload"), projVideoSpec{kind: "stream", start: projAt(-time.Hour)}.video("next")},
			"",
		},
		{
			"no latest video",
			[]DashboardEvent{projEventSpec{eventType: "stream", start: projAt(time.Hour)}.event("stream")},
			nil,
			"",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dashboard := &Dashboard{Events: tc.events, Videos: tc.videos}
			applyDashboardProjection(dashboard, projNow)
			got := dashboard.Projection.Spotlight
			if got == nil || got.Mode != "next" || projString(got.CompanionVideoID) != projString(projOptional(tc.companion)) {
				t.Fatalf("spotlight = %+v (companion %s)", got, projString(got.CompanionVideoID))
			}
		})
	}
}

func TestSalesListsOpenThenSoonExcludingCancellations(t *testing.T) {
	sale := func(start *string, endsOn string, end *string) projEventSpec { return projSale(start, "", endsOn, end) }
	cancelled := sale(projAt(-time.Hour), "2026-11-03", nil)
	cancelled.cancellation = "manual_confirmed"
	review := sale(projAt(-time.Hour), "2026-11-03", nil)
	review.cancellation = "llm_suspected"
	deadline := projEventSpec{eventType: "sale", source: "shop", startsOn: "2026-11-04", endsOn: "2026-11-04", precision: "unknown"}
	dashboard := &Dashboard{Events: []DashboardEvent{
		sale(projAt(time.Hour+7*24*time.Hour), "2026-12-01", nil).event("far"),
		sale(projAt(7*24*time.Hour), "2026-12-01", nil).event("edge"),
		sale(projAt(48*time.Hour), "2026-12-01", nil).event("near"),
		sale(projAt(24*time.Hour), "2026-12-01", nil).event("sooner"),
		sale(projAt(-48*time.Hour), "2026-11-20", nil).event("open-late"),
		sale(projAt(-48*time.Hour), "2026-11-05", strPtr("2026-11-05T03:00:00Z")).event("open-soon"),
		deadline.event("deadline"),
		sale(projAt(-72*time.Hour), "2026-11-01", nil).event("closed"),
		cancelled.event("cancelled"),
		review.event("review"),
		projEventSpec{eventType: "stream", start: projAt(time.Hour)}.event("stream"),
	}}
	applyDashboardProjection(dashboard, projNow)
	want := []string{"deadline", "open-soon", "open-late", "sooner", "near", "edge"}
	if got := dashboard.Projection.Sales; !reflect.DeepEqual(got, want) {
		t.Fatalf("sales = %v, want %v", got, want)
	}
}

func TestNextTransitionAt(t *testing.T) {
	for _, tc := range []struct {
		name   string
		events []DashboardEvent
		videos []Video
		want   string
	}{
		{"nothing", nil, nil, ""},
		{"nothing ahead", []DashboardEvent{projEventSpec{eventType: "event", start: projAt(-24 * time.Hour)}.event("past")}, []Video{projVideoSpec{kind: "video", published: projAt(-time.Hour)}.video("upload")}, ""},
		{"event start", []DashboardEvent{projEventSpec{eventType: "event", start: projAt(2 * time.Hour)}.event("event")}, nil, "2026-11-02T06:00:00Z"},
		{"date-only event at its Japan day", []DashboardEvent{projEventSpec{eventType: "event", startsOn: "2026-11-03"}.event("event")}, nil, "2026-11-02T15:00:00Z"},
		{"date-only event at the end of its Japan day", []DashboardEvent{projEventSpec{eventType: "event", startsOn: "2026-11-02"}.event("event")}, nil, "2026-11-02T15:00:00Z"},
		{"date-only stream gets no grace", []DashboardEvent{projEventSpec{eventType: "stream", startsOn: "2026-11-02", precision: "unknown"}.event("stream")}, nil, "2026-11-02T15:00:00Z"},
		{"stream grace", []DashboardEvent{projEventSpec{eventType: "stream", start: projAt(-2 * time.Hour)}.event("stream")}, nil, "2026-11-02T05:00:00Z"},
		{"membership grace", []DashboardEvent{projEventSpec{eventType: "member", start: projAt(-150 * time.Minute)}.event("member")}, nil, "2026-11-02T04:30:00Z"},
		{"event end", []DashboardEvent{projEventSpec{eventType: "event", start: projAt(-time.Hour), end: projAt(30 * time.Minute)}.event("event")}, nil, "2026-11-02T04:30:00Z"},
		{"sale opening", []DashboardEvent{projSale(projAt(time.Hour), "", "2026-11-10", nil).event("sale")}, nil, "2026-11-02T05:00:00Z"},
		{"sale opening date", []DashboardEvent{projSale(nil, "2026-11-04", "2026-11-10", nil).event("sale")}, nil, "2026-11-03T15:00:00Z"},
		{"sale close", []DashboardEvent{projSale(projAt(-24*time.Hour), "", "2026-11-02", projAt(3*time.Hour)).event("sale")}, nil, "2026-11-02T07:00:00Z"},
		{"sale close date", []DashboardEvent{projSale(projAt(-24*time.Hour), "", "2026-11-02", nil).event("sale")}, nil, "2026-11-02T14:59:59.999Z"},
		{"deadline", []DashboardEvent{projSale(nil, "2026-11-03", "2026-11-03", nil).event("sale")}, nil, "2026-11-03T14:59:59.999Z"},
		{"video start", nil, []Video{projVideoSpec{kind: "stream", start: projAt(90 * time.Minute)}.video("stream")}, "2026-11-02T05:30:00Z"},
		{"video end", nil, []Video{projVideoSpec{kind: "premiere", start: projAt(-30 * time.Minute), end: projAt(45 * time.Minute)}.video("premiere")}, "2026-11-02T04:45:00Z"},
		{"video stream grace", nil, []Video{projVideoSpec{kind: "stream", start: projAt(-150 * time.Minute)}.video("stream")}, "2026-11-02T04:30:00Z"},
		{
			"earliest change",
			[]DashboardEvent{
				projEventSpec{eventType: "event", start: projAt(2 * time.Hour)}.event("event"),
				projSale(projAt(-24*time.Hour), "", "2026-11-02", projAt(3*time.Hour)).event("sale"),
			},
			[]Video{projVideoSpec{kind: "stream", start: projAt(90 * time.Minute)}.video("stream")},
			"2026-11-02T05:30:00Z",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dashboard := &Dashboard{Events: tc.events, Videos: tc.videos}
			applyDashboardProjection(dashboard, projNow)
			if got := projString(dashboard.Projection.NextTransitionAt); got != projString(projOptional(tc.want)) {
				t.Fatalf("nextTransitionAt = %s, want %q", got, tc.want)
			}
		})
	}
}
