package sqlite

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/yexca/kano-navi/backend/internal/testutil"
)

// The calendar, the event view lookup and the video archive carry the same
// views as the dashboard at the same instant.
func TestReadersCarryTheDashboardViews(t *testing.T) {
	path := testutil.NewSnapshot(t)
	insertSaleFixture(t, path)
	testutil.Exec(t, path, `INSERT INTO videos (id,source,title,published_at,url,kind) VALUES ('upload','youtube','Upload','2026-10-30T00:00:00.000Z','https://example.com/video/upload','video')`)
	testutil.Exec(t, path, `INSERT INTO videos (id,source,title,scheduled_at,url,kind,is_upcoming) VALUES ('reserved','youtube','Reserved','2026-11-03T11:00:00.000Z','https://example.com/video/reserved','stream',1)`)
	store, err := OpenReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	now := time.Date(2026, 11, 2, 4, 0, 0, 0, time.UTC)

	board, err := store.GetDashboard(ctx, 3, now)
	if err != nil {
		t.Fatal(err)
	}
	boardEvents := map[string]EventView{}
	for _, event := range board.Events {
		boardEvents[event.ID] = event.View
	}
	boardVideos := map[string]VideoView{}
	for _, video := range board.Videos {
		boardVideos[video.ID] = video.View
	}
	if _, ok := boardEvents["sale-period"]; !ok {
		t.Fatalf("board events lack the sale period: %v", eventIDs(board.Events))
	}
	if _, ok := boardEvents["stream"]; !ok {
		t.Fatalf("board events lack the stream: %v", eventIDs(board.Events))
	}
	if _, ok := boardVideos["reserved"]; !ok {
		t.Fatalf("board videos lack the reservation: %v", boardVideos)
	}

	page, err := store.GetCalendarPageAt(ctx, "2026-10-26", "2026-11-30", 1, 100, now)
	if err != nil {
		t.Fatal(err)
	}
	states := map[string]string{}
	for _, event := range page.Items {
		states[event.ID] = event.View.State
		if want, ok := boardEvents[event.ID]; ok && !reflect.DeepEqual(event.View, want) {
			t.Errorf("calendar view of %s = %+v, board %+v", event.ID, event.View, want)
		}
	}
	if want := map[string]string{"sale-period": "open", "sale-deadline": "open", "stream": "upcoming"}; !reflect.DeepEqual(states, want) {
		t.Fatalf("calendar states = %v, want %v", states, want)
	}

	views, err := store.PublicEventViews(ctx, []string{"sale-period", "stream"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(views) != 2 || views["stream"].State != "upcoming" || projString(views["stream"].TypeCode) != "stream" ||
		len(views["sale-period"].CalendarEntries) != 2 {
		t.Fatalf("event views = %+v", views)
	}

	archive, err := store.GetVideoArchive(ctx, "2026-10-26", "2026-11-30", "", 1, 20, now)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][2]string{"upload": {"past", "video"}, "reserved": {"upcoming", "upcoming"}}
	if len(archive.Items) != len(want) {
		t.Fatalf("archive items = %d", len(archive.Items))
	}
	for _, video := range archive.Items {
		if got := [2]string{video.View.Phase, projString(video.View.FormCode)}; got != want[video.ID] {
			t.Errorf("archive view of %s = %v, want %v", video.ID, got, want[video.ID])
		}
		if board, ok := boardVideos[video.ID]; ok && (board.Phase != video.View.Phase || !reflect.DeepEqual(board.FormCode, video.View.FormCode) || board.TagContext != video.View.TagContext) {
			t.Errorf("archive view of %s = %+v, board %+v", video.ID, video.View, board)
		}
	}
}
