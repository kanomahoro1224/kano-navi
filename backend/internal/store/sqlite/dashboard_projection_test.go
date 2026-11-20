package sqlite

import (
	"testing"
	"time"
)

func TestDashboardProjectionCarriesSharedEventAndVideoSemantics(t *testing.T) {
	now := time.Date(2026, 11, 2, 4, 0, 0, 0, time.UTC)
	stream := DashboardEvent{
		ID: "stream", Source: "youtube", Title: "Stream", StartsOn: "2026-11-02",
		StartsAt: strPtr("2026-11-02T05:00:00Z"), TimePrecision: "exact",
		EventType: strPtr("stream"), Tag: strPtr("song"), CancellationStatus: "none",
	}
	sale := DashboardEvent{
		ID: "sale", Source: "shop", Title: "Sale", StartsOn: "2026-11-02",
		StartsAt: strPtr("2026-11-02T03:00:00Z"), EndsOn: strPtr("2026-11-22"),
		EndsAt: strPtr("2026-11-22T14:59:00Z"), TimePrecision: "exact",
		EventType: strPtr("sale"), CancellationStatus: "none",
	}
	video := Video{
		ID: "video", Source: "youtube", Title: "Stream", URL: "https://www.youtube.com/watch?v=video",
		ScheduledAt: strPtr("2026-11-02T05:00:00Z"), Kind: strPtr("stream"),
		IsUpcoming: true, Tag: strPtr("song"),
	}
	dashboard := &Dashboard{
		Events: []DashboardEvent{stream, sale}, Videos: []Video{video},
		Summary: DashboardSummary{NextEvent: &stream, NextStream: &video},
	}

	applyDashboardProjection(dashboard, now)

	if got := dashboard.Events[0].View.State; got != "upcoming" {
		t.Fatalf("stream state=%q", got)
	}
	if dashboard.Events[0].View.TypeCode != nil || dashboard.Events[0].View.ActivityCode == nil || *dashboard.Events[0].View.ActivityCode != "song" {
		t.Fatalf("stream view=%+v", dashboard.Events[0].View)
	}
	if got := dashboard.Events[1].View.State; got != "open" {
		t.Fatalf("sale state=%q", got)
	}
	entries := dashboard.Events[1].View.CalendarEntries
	if len(entries) != 2 || entries[0].SaleMark == nil || *entries[0].SaleMark != "opens" || entries[1].SaleMark == nil || *entries[1].SaleMark != "closes" {
		t.Fatalf("sale entries=%+v", entries)
	}
	if dashboard.Videos[0].View.Phase != "upcoming" || dashboard.Videos[0].View.FormCode == nil || *dashboard.Videos[0].View.FormCode != "upcoming" {
		t.Fatalf("video view=%+v", dashboard.Videos[0].View)
	}
	if len(dashboard.Projection.Focus.EventIDs) != 1 || dashboard.Projection.Focus.EventIDs[0] != "stream" || len(dashboard.Projection.Focus.VideoIDs) != 1 || dashboard.Projection.Focus.VideoIDs[0] != "video" {
		t.Fatalf("focus=%+v", dashboard.Projection.Focus)
	}
	if dashboard.Projection.Spotlight == nil || dashboard.Projection.Spotlight.EventID == nil || *dashboard.Projection.Spotlight.EventID != "stream" {
		t.Fatalf("spotlight=%+v", dashboard.Projection.Spotlight)
	}
}

// The projection lists every sale the dashboard carries, so the board can
// open the ones beyond its row.
func TestDashboardProjectionListsEverySale(t *testing.T) {
	now := time.Date(2026, 11, 2, 4, 0, 0, 0, time.UTC)
	sale := func(id, opens, closes string) DashboardEvent {
		return DashboardEvent{
			ID: id, Source: "shop", Title: id, StartsOn: opens,
			StartsAt: strPtr(opens + "T03:00:00Z"), EndsOn: strPtr(closes),
			EndsAt: strPtr(closes + "T14:59:00Z"), TimePrecision: "exact",
			EventType: strPtr("sale"), CancellationStatus: "none",
		}
	}
	dashboard := &Dashboard{Events: []DashboardEvent{
		sale("e", "2026-11-05", "2026-11-30"),
		sale("a", "2026-10-20", "2026-11-04"),
		sale("d", "2026-10-25", "2026-11-20"),
		sale("b", "2026-10-28", "2026-11-08"),
		sale("c", "2026-11-01", "2026-11-12"),
		sale("later", "2026-11-20", "2026-12-20"),
	}}

	applyDashboardProjection(dashboard, now)

	want := []string{"a", "b", "c", "d", "e"}
	got := dashboard.Projection.Sales
	if len(got) != len(want) {
		t.Fatalf("sales=%v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sales=%v", got)
		}
	}
}

// A video confirmed on air is the next stream and the stage's live ticket,
// even when a waiting room without a start was published earlier.
func TestDashboardProjectionPrefersALiveVideoOverAnUnscheduledWaitingRoom(t *testing.T) {
	now := time.Date(2026, 11, 2, 4, 0, 0, 0, time.UTC)
	waiting := Video{
		ID: "waiting", Source: "youtube", Title: "Waiting room", URL: "https://www.youtube.com/watch?v=waiting",
		PublishedAt: strPtr("2026-10-01T00:00:00Z"), Kind: strPtr("stream"), IsUpcoming: true,
	}
	live := Video{
		ID: "live", Source: "youtube", Title: "On air", URL: "https://www.youtube.com/watch?v=live",
		PublishedAt: strPtr("2026-11-01T00:00:00Z"), ScheduledAt: strPtr("2026-11-02T03:30:00Z"),
		Kind: strPtr("stream"), LiveStatus: strPtr("live"),
	}
	dashboard := &Dashboard{Videos: []Video{waiting, live}}

	applyDashboardProjection(dashboard, now)

	if next := dashboard.Summary.NextStream; next == nil || next.ID != "live" {
		t.Fatalf("next stream = %+v, want live", next)
	}
	if got := dashboard.Projection.Focus.VideoIDs; len(got) != 2 || got[0] != "live" || got[1] != "waiting" {
		t.Fatalf("focus videos = %v, want the waiting room last", got)
	}
	spotlight := dashboard.Projection.Spotlight
	if spotlight == nil || spotlight.Mode != "next" || spotlight.VideoID == nil || *spotlight.VideoID != "live" || !spotlight.IsLive {
		t.Fatalf("spotlight = %+v", spotlight)
	}
}

// A scheduled reservation leads an unscheduled waiting room; the waiting
// room is the next stream only when nothing scheduled is ahead.
func TestDashboardProjectionPutsAnUnscheduledWaitingRoomLast(t *testing.T) {
	now := time.Date(2026, 11, 2, 4, 0, 0, 0, time.UTC)
	waiting := Video{
		ID: "waiting", Source: "youtube", Title: "Waiting room", URL: "https://www.youtube.com/watch?v=waiting",
		PublishedAt: strPtr("2026-10-01T00:00:00Z"), Kind: strPtr("stream"), IsUpcoming: true,
	}
	reserved := Video{
		ID: "reserved", Source: "youtube", Title: "Tomorrow", URL: "https://www.youtube.com/watch?v=reserved",
		ScheduledAt: strPtr("2026-11-03T11:00:00Z"), Kind: strPtr("stream"), IsUpcoming: true,
	}
	dashboard := &Dashboard{Videos: []Video{waiting, reserved}}
	applyDashboardProjection(dashboard, now)
	if next := dashboard.Summary.NextStream; next == nil || next.ID != "reserved" {
		t.Fatalf("next stream = %+v, want reserved", next)
	}

	alone := &Dashboard{Videos: []Video{waiting}}
	applyDashboardProjection(alone, now)
	if next := alone.Summary.NextStream; next == nil || next.ID != "waiting" {
		t.Fatalf("next stream = %+v, want waiting", next)
	}
}

// The Featured video may be the latest video as a Short, and its view says
// so; other Shorts may not.
func TestDashboardProjectionMarksTheFeaturedShortLatestEligible(t *testing.T) {
	short := true
	now := time.Date(2026, 11, 2, 4, 0, 0, 0, time.UTC)
	featured := Video{
		ID: "featured-short", Source: "youtube", Title: "Featured", URL: "https://www.youtube.com/shorts/featured",
		PublishedAt: strPtr("2026-11-01T00:00:00Z"), Kind: strPtr("video"), IsShort: &short,
	}
	other := Video{
		ID: "other-short", Source: "youtube", Title: "Other", URL: "https://www.youtube.com/shorts/other",
		PublishedAt: strPtr("2026-11-01T06:00:00Z"), Kind: strPtr("video"), IsShort: &short,
	}
	dashboard := &Dashboard{Videos: []Video{featured, other}, Meta: DashboardMeta{FeaturedVideoID: strPtr("featured-short")}}

	applyDashboardProjection(dashboard, now)

	if !dashboard.Videos[0].View.LatestEligible || dashboard.Videos[1].View.LatestEligible {
		t.Fatalf("latestEligible = %v, %v", dashboard.Videos[0].View.LatestEligible, dashboard.Videos[1].View.LatestEligible)
	}
	if latest := dashboard.Summary.LatestVideo; latest == nil || latest.ID != "featured-short" || !latest.View.LatestEligible {
		t.Fatalf("latest video = %+v", latest)
	}
}
