package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/yexca/kano-navi/backend/internal/works"
)

func worksTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := OpenInitialized(filepath.Join(t.TempDir(), "data", "database", "snapshot.sqlite"), true, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestWorksMergeConflictsListOnlyDisagreements(t *testing.T) {
	s := worksTestStore(t)
	ctx := context.Background()
	if _, err := s.db.ExecContext(ctx, `INSERT INTO videos (id,source,title,url,merged_into) VALUES
		('aaaaaaaaaaa','youtube','A','https://www.youtube.com/watch?v=aaaaaaaaaaa',NULL),
		('bbbbbbbbbbb','youtube','B','https://www.youtube.com/watch?v=bbbbbbbbbbb',NULL),
		('BV1AAAAAAAAA','bilibili','A','https://www.bilibili.com/video/BV1AAAAAAAAA','aaaaaaaaaaa'),
		('BV1BBBBBBBBB','bilibili','B','https://www.bilibili.com/video/BV1BBBBBBBBB','aaaaaaaaaaa'),
		('BV1CCCCCCCCC','bilibili','C','https://www.bilibili.com/video/BV1CCCCCCCCC',NULL),
		('BV1DDDDDDDDD','bilibili','D','https://www.bilibili.com/video/BV1DDDDDDDDD',NULL)`); err != nil {
		t.Fatal(err)
	}
	index := works.Index{Uploads: []works.Upload{
		// Agrees: merged into the paired video.
		{ID: "2026-01-01-aaaaaa", Date: "2026-01-01", Videos: []works.Video{{Platform: works.YouTube, ID: "aaaaaaaaaaa"}, {Platform: works.Bilibili, ID: "BV1AAAAAAAAA"}}},
		// Merged into another video than the list's.
		{ID: "2026-01-02-bbbbbb", Date: "2026-01-02", Videos: []works.Video{{Platform: works.YouTube, ID: "bbbbbbbbbbb"}, {Platform: works.Bilibili, ID: "BV1BBBBBBBBB"}}},
		// Not merged although its stored YouTube video is listed with it.
		{ID: "2026-01-03-cccccc", Date: "2026-01-03", Videos: []works.Video{{Platform: works.YouTube, ID: "bbbbbbbbbbb"}, {Platform: works.Bilibili, ID: "BV1CCCCCCCCC"}}},
		// A Bilibili-only upload stays apart.
		{ID: "2026-01-04-dddddd", Date: "2026-01-04", Videos: []works.Video{{Platform: works.Bilibili, ID: "BV1DDDDDDDDD"}}},
	}}
	got, err := s.WorksMergeConflicts(ctx, index)
	if err != nil || len(got) != 2 || got[0].Bilibili != "BV1BBBBBBBBB" || *got[0].MergedInto != "aaaaaaaaaaa" || got[1].Bilibili != "BV1CCCCCCCCC" || got[1].MergedInto != nil {
		t.Fatalf("conflicts=%#v err=%v", got, err)
	}
}

func TestWorksOnlyImageIsNotRetriedOnceMissing(t *testing.T) {
	s := worksTestStore(t)
	ctx := context.Background()
	index := works.Index{Uploads: []works.Upload{
		{ID: "2026-01-01-aaaaaa", Date: "2026-01-01", Videos: []works.Video{{Platform: works.YouTube, ID: "aaaaaaaaaaa"}, {Platform: works.Niconico, ID: "sm9"}}},
	}}
	plan, err := s.PlanWorkCovers(ctx, index, time.Now())
	if err != nil || plan.Queued != 1 || len(plan.Lookups) != 0 {
		t.Fatalf("plan=%#v err=%v", plan, err)
	}
	pending, err := s.PendingMedia(ctx, 10)
	if err != nil || len(pending) != 1 || pending[0].SourceURL != "https://i.ytimg.com/vi/aaaaaaaaaaa/maxresdefault.jpg" {
		t.Fatalf("pending=%#v err=%v", pending, err)
	}
	if err = s.SaveMediaDownload(ctx, pending[0], nil, "", "", "", "missing", "media_missing"); err != nil {
		t.Fatal(err)
	}
	// Even after the retry delay a works-only image that is gone stays gone.
	if _, err = s.db.ExecContext(ctx, `UPDATE media_assets SET next_retry_at='2000-01-01T00:00:00.000Z'`); err != nil {
		t.Fatal(err)
	}
	if pending, err = s.PendingMedia(ctx, 10); err != nil || len(pending) != 0 {
		t.Fatalf("pending=%#v err=%v", pending, err)
	}
	// The upload moves on to its niconico video.
	plan, err = s.PlanWorkCovers(ctx, index, time.Now())
	if err != nil || plan.Queued != 0 || len(plan.Lookups) != 1 || plan.Lookups[0].ID != "sm9" {
		t.Fatalf("plan=%#v err=%v", plan, err)
	}
	if err = s.SaveWorkCoverLookup(ctx, plan.Lookups[0], WorkLookupMissing, "", "niconico_not_found", time.Now()); err != nil {
		t.Fatal(err)
	}
	if plan, err = s.PlanWorkCovers(ctx, index, time.Now()); err != nil || plan.Uncovered != 1 || len(plan.Lookups) != 0 {
		t.Fatalf("plan=%#v err=%v", plan, err)
	}
}

func TestStoredVideoThumbnailCoversItsWork(t *testing.T) {
	s := worksTestStore(t)
	ctx := context.Background()
	if _, err := s.db.ExecContext(ctx, `INSERT INTO videos (id,source,title,url,thumbnail_url) VALUES
		('aaaaaaaaaaa','youtube','A','https://www.youtube.com/watch?v=aaaaaaaaaaa','https://i.ytimg.com/vi/aaaaaaaaaaa/hqdefault.jpg')`); err != nil {
		t.Fatal(err)
	}
	index := works.Index{Uploads: []works.Upload{
		{ID: "2026-01-01-aaaaaa", Date: "2026-01-01", Videos: []works.Video{{Platform: works.YouTube, ID: "aaaaaaaaaaa"}}},
	}}
	// The stored video's own image is on its way, so nothing new is queued.
	plan, err := s.PlanWorkCovers(ctx, index, time.Now())
	if err != nil || plan.Queued != 0 || plan.Waiting != 1 {
		t.Fatalf("plan=%#v err=%v", plan, err)
	}
	var links int
	if err = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM media_links WHERE owner_type='work-video'`).Scan(&links); err != nil || links != 0 {
		t.Fatalf("links=%d err=%v", links, err)
	}
}
