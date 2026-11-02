package sqlite

import (
	"context"
	"testing"

	"github.com/yexca/kano-navi/backend/internal/testutil"
)

// The App reads the calendar, so an operator's label has to arrive on the
// event of the video it was set on. An event that carries its own label keeps
// it: the video label only covers an entry that has none.
func TestCalendarCarriesTheOperatorTag(t *testing.T) {
	path := testutil.NewSnapshot(t)
	testutil.Exec(t, path, `INSERT INTO videos(id,source,title,scheduled_at,url,manual_tag) VALUES
		('AAAAAAAAAAA','youtube','Song','2026-11-01T12:00:00.000Z','https://www.youtube.com/watch?v=AAAAAAAAAAA','song'),
		('BBBBBBBBBBB','youtube','Plain','2026-11-01T13:00:00.000Z','https://www.youtube.com/watch?v=BBBBBBBBBBB',NULL),
		('CCCCCCCCCCC','youtube','Both','2026-11-01T14:00:00.000Z','https://www.youtube.com/watch?v=CCCCCCCCCCC','game')`)
	testutil.Exec(t, path, `INSERT INTO events(id,source,source_item_id,source_key,title,starts_on,starts_at,url) VALUES
		('youtube-AAAAAAAAAAA','youtube','AAAAAAAAAAA','reservation','Song','2026-11-01','2026-11-01T12:00:00.000Z','https://www.youtube.com/watch?v=AAAAAAAAAAA'),
		('youtube-BBBBBBBBBBB','youtube','BBBBBBBBBBB','reservation','Plain','2026-11-01','2026-11-01T13:00:00.000Z','https://www.youtube.com/watch?v=BBBBBBBBBBB')`)
	testutil.Exec(t, path, `INSERT INTO events(id,source,source_item_id,source_key,title,starts_on,starts_at,url,tag,tag_source) VALUES
		('youtube-CCCCCCCCCCC','youtube','CCCCCCCCCCC','reservation','Both','2026-11-01','2026-11-01T14:00:00.000Z','https://www.youtube.com/watch?v=CCCCCCCCCCC','live','manual')`)
	store, err := OpenReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	page, err := store.GetCalendarPage(context.Background(), "2026-11-01", "2026-11-02", 1, 10)
	if err != nil {
		t.Fatal(err)
	}
	tags := map[string]string{}
	for _, item := range page.Items {
		if item.Tag != nil {
			tags[item.ID] = *item.Tag
		}
	}
	if len(page.Items) != 3 || tags["youtube-AAAAAAAAAAA"] != "song" {
		t.Fatalf("items=%d tags=%v", len(page.Items), tags)
	}
	if _, ok := tags["youtube-BBBBBBBBBBB"]; ok {
		t.Fatalf("an untagged video must not invent a tag: %v", tags)
	}
	if tags["youtube-CCCCCCCCCCC"] != "live" {
		t.Fatalf("an event label must outrank its video's: %v", tags)
	}
}
