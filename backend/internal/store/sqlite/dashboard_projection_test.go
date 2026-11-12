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
