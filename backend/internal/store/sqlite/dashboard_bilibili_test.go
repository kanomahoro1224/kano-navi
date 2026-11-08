package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/yexca/kano-navi/backend/internal/testutil"
)

// A Bilibili upload with no YouTube copy is a work published only there, so
// the board lists it under its own cover. An upload merged into a YouTube
// video stays out of the list and reaches readers as that video's alternate
// instead, so one work never shows two covers.
func TestDashboardListsBilibiliUploadsOnlyWithoutACopy(t *testing.T) {
	path := testutil.NewSnapshot(t)
	testutil.Exec(t, path, `INSERT INTO videos (id,source,title,published_at,url,kind) VALUES
		('yt1','youtube','YouTube cover','2026-10-20T00:00:00.000Z','https://www.youtube.com/watch?v=yt1','upload'),
		('BV1own','bilibili','Bilibili only','2026-10-21T00:00:00.000Z','https://www.bilibili.com/video/BV1own','upload'),
		('BV1copy','bilibili','YouTube copy','2026-10-20T00:00:00.000Z','https://www.bilibili.com/video/BV1copy','upload')`)
	testutil.Exec(t, path, `UPDATE videos SET merged_into='yt1',merge_source='rule',merged_at='2026-10-20T01:00:00.000Z' WHERE id='BV1copy'`)

	store, err := OpenReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	dashboard, err := store.GetDashboard(context.Background(), 3, time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	listed := map[string]*Video{}
	for i := range dashboard.Videos {
		listed[dashboard.Videos[i].ID] = &dashboard.Videos[i]
	}
	if listed["yt1"] == nil || listed["BV1own"] == nil {
		t.Fatalf("board should list the YouTube video and the Bilibili-only upload: %v", listed)
	}
	if listed["BV1copy"] != nil {
		t.Fatalf("the merged upload must not get a second cover")
	}
	alternates := listed["yt1"].Alternates
	if len(alternates) != 1 || alternates[0].Source != "bilibili" || alternates[0].URL != "https://www.bilibili.com/video/BV1copy" {
		t.Fatalf("alternates=%+v", alternates)
	}
}
