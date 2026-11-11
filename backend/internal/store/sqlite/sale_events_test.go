package sqlite

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/yexca/kano-navi/backend/internal/testutil"
)

// insertSaleFixture stores a sale period that opens in October and closes in
// November, a deadline whose opening is not known, and a stream.
func insertSaleFixture(t *testing.T, path string) {
	t.Helper()
	for _, row := range []struct {
		id, eventType, startsOn string
		startsAt, endsAt        *string
		endsOn                  *string
	}{
		{"sale-period", "sale", "2026-10-28", strPtr("2026-10-28T03:00:00.000Z"), strPtr("2026-11-22T14:59:00.000Z"), strPtr("2026-11-22")},
		{"sale-deadline", "sale", "2026-11-05", nil, strPtr("2026-11-05T14:59:00.000Z"), strPtr("2026-11-05")},
		{"stream", "stream", "2026-11-03", strPtr("2026-11-03T11:00:00.000Z"), nil, nil},
	} {
		testutil.Exec(t, path, `INSERT INTO events (id,source,title,starts_on,starts_at,ends_at,ends_on,time_precision,event_type,status,created_at,updated_at) VALUES (?,'x',?,?,?,?,?,CASE WHEN ? IS NULL THEN 'unknown' ELSE 'exact' END,?,'scheduled','2026-10-20T00:00:00.000Z','2026-10-20T00:00:00.000Z')`,
			row.id, row.id, row.startsOn, row.startsAt, row.endsAt, row.endsOn, row.startsAt, row.eventType)
	}
}

func eventIDs(events []DashboardEvent) []string {
	ids := []string{}
	for _, event := range events {
		ids = append(ids, event.ID)
	}
	return ids
}

func TestTheCalendarShowsASaleOnItsCloseDate(t *testing.T) {
	path := testutil.NewSnapshot(t)
	insertSaleFixture(t, path)
	store, err := OpenReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()

	// November's grid reaches the close of the October sale, but not its
	// opening.
	page, err := store.GetCalendarPage(ctx, "2026-11-01", "2026-11-30", 1, 100)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := eventIDs(page.Items), []string{"sale-period", "stream", "sale-deadline"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("items = %v, want %v", got, want)
	}
	if page.Items[0].EndsOn == nil || *page.Items[0].EndsOn != "2026-11-22" {
		t.Fatalf("endsOn = %v", page.Items[0].EndsOn)
	}
	// A deadline counts once on its date; the period counts on its close.
	want := map[string]int64{"2026-11-03": 1, "2026-11-05": 1, "2026-11-22": 1}
	if page.Total != 3 || !reflect.DeepEqual(page.DayCounts, want) {
		t.Fatalf("total = %d, dayCounts = %v", page.Total, page.DayCounts)
	}
	// The close date is a date with entries for the adjacent-date shortcuts.
	page, err = store.GetCalendarPage(ctx, "2026-12-01", "2026-12-31", 1, 100)
	if err != nil {
		t.Fatal(err)
	}
	if page.Previous == nil || *page.Previous != "2026-11-22" {
		t.Fatalf("previous = %v", page.Previous)
	}
	page, err = store.GetCalendarPage(ctx, "2026-11-06", "2026-11-20", 1, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 0 || page.Next == nil || *page.Next != "2026-11-22" {
		t.Fatalf("items = %v, next = %v", eventIDs(page.Items), page.Next)
	}
}

func TestASaleIsNeverTheNextEvent(t *testing.T) {
	path := testutil.NewSnapshot(t)
	insertSaleFixture(t, path)
	store, err := OpenReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	// Before the October opening, the sale comes first, but the stream is
	// the next event. The week of the close date lists the period too.
	dashboard, err := store.GetDashboard(context.Background(), 3, time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if next := dashboard.Summary.NextEvent; next == nil || next.ID != "stream" {
		t.Fatalf("next event = %+v", next)
	}
	dashboard, err = store.GetDashboard(context.Background(), 3, time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if !containsEvent(dashboard.Events, "sale-period") {
		t.Fatalf("events = %v", eventIDs(dashboard.Events))
	}
}

func TestTheDashboardCarriesSalesOnSaleOrOpeningSoon(t *testing.T) {
	path := testutil.NewSnapshot(t)
	for _, row := range []struct{ id, startsOn, endsOn string }{
		// The upcoming list already carries a sale opening later; these are the
		// ones only the on-sale selection reaches, and one that has closed.
		{"long-period", "2026-09-20", "2026-11-12"},  // opened weeks ago, still on sale
		{"far-deadline", "2026-12-01", "2026-12-01"}, // a deadline: on sale until it passes
		{"soon", "2026-11-05", "2026-11-30"},         // opens within the week
		{"closed", "2026-09-01", "2026-10-20"},
	} {
		testutil.Exec(t, path, `INSERT INTO events (id,source,title,starts_on,ends_on,time_precision,event_type,status,created_at,updated_at) VALUES (?,'x',?,?,?,'unknown','sale','scheduled','2026-10-20T00:00:00.000Z','2026-10-20T00:00:00.000Z')`,
			row.id, row.id, row.startsOn, row.endsOn)
	}
	store, err := OpenReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	// Wednesday 28 October in Japan; the week runs 26 October to 1 November.
	dashboard, err := store.GetDashboard(context.Background(), 3, time.Date(2026, 10, 28, 3, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	for id, want := range map[string]bool{"long-period": true, "far-deadline": true, "soon": true, "closed": false} {
		if containsEvent(dashboard.Events, id) != want {
			t.Errorf("%s included = %v, want %v", id, !want, want)
		}
	}
}
