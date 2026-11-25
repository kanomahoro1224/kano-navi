package sqlite

import (
	"context"
	"database/sql"
	"testing"

	ytsource "github.com/yexca/kano-navi/backend/internal/source/youtube"
	"github.com/yexca/kano-navi/backend/internal/testutil"
)

// An X notice's event of a video is retired when the notice's evidence moves
// to another event and leaves it none; the video's reservation restores it.
func TestReservationRestoresAnEventWhoseEvidenceMovedAway(t *testing.T) {
	p := testutil.NewSnapshot(t)
	s, err := OpenWorkflowCore(p, true)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	testutil.Exec(t, p, `INSERT INTO events(id,source,source_item_id,source_key,title,starts_on,provenance,manual_locked,deleted_at,created_at,updated_at)
		VALUES('youtube-AAAAAAAAAAA','youtube','AAAAAAAAAAA','reservation','notice','2026-01-03','automatic',0,'2026-01-01T01:00:00Z','2026-01-01T00:00:00Z','2026-01-01T01:00:00Z')`)
	result := ytsource.Result{Videos: []ytsource.Video{{ID: "AAAAAAAAAAA", Title: "reservation", PublishedAt: "2026-01-01T00:00:00Z", ScheduledAt: "2026-01-02T17:00:00Z", URL: "https://www.youtube.com/watch?v=AAAAAAAAAAA", ThumbnailURL: "https://i.ytimg.com/vi/AAAAAAAAAAA/hqdefault.jpg", Kind: "UPCOMING LIVE", IsUpcoming: true}}, NextCursor: "AAAAAAAAAAA", NextCursorTime: "2026-01-01T00:00:00Z", Queue: []string{"AAAAAAAAAAA"}}
	if err = s.WriteYoutubeRecent(context.Background(), "UCAAAAAAAAAAAAAAAAAAAAAA", result); err != nil {
		t.Fatal(err)
	}
	db, _ := sql.Open("sqlite", p)
	defer db.Close()
	var title string
	var deleted sql.NullString
	if err = db.QueryRow(`SELECT title,deleted_at FROM events WHERE id='youtube-AAAAAAAAAAA'`).Scan(&title, &deleted); err != nil {
		t.Fatal(err)
	}
	if title != "reservation" || deleted.Valid {
		t.Fatalf("the reservation did not restore the event: %s deleted=%v", title, deleted)
	}
	// An operator's tombstone has no evidence either and stays.
	testutil.Exec(t, p, `UPDATE events SET provenance='manual',manual_locked=1,deleted_at='2026-01-01T02:00:00Z' WHERE id='youtube-AAAAAAAAAAA'`)
	testutil.Exec(t, p, `DELETE FROM event_sources WHERE event_id='youtube-AAAAAAAAAAA'`)
	if err = s.WriteYoutubeRecent(context.Background(), "UCAAAAAAAAAAAAAAAAAAAAAA", result); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT deleted_at FROM events WHERE id='youtube-AAAAAAAAAAA'`).Scan(&deleted); err != nil || !deleted.Valid {
		t.Fatalf("a reservation resurrected an operator's tombstone: %v %v", deleted, err)
	}
}
