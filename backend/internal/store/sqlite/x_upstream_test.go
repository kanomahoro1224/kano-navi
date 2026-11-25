package sqlite

import (
	"context"
	"strings"
	"testing"
	"time"

	xsource "github.com/yexca/kano-navi/backend/internal/source/x"
	"github.com/yexca/kano-navi/backend/internal/testutil"
)

// syntheticXPost is an own post of account with one image, published at.
func syntheticXPost(account, id, text string, at time.Time) xsource.Post {
	return xsource.Post{ID: id, AccountHandle: account, Type: "daily", Text: text, PublishedAt: at.UTC().Format(time.RFC3339Nano),
		URL: "https://x.com/" + account + "/status/" + id, MediaURLs: []string{"https://pbs.twimg.com/media/" + id + ".jpg"}, Raw: map[string]any{"text": text}}
}

// A post whose ID another account owns, such as one stored before a handle
// rename, is skipped with a note: the stored row and its schedule image stay
// as they are, the rest of the account's run is written, and the ID leaves
// the queue.
func TestWriteXRecentSkipsAPostAnotherAccountOwns(t *testing.T) {
	p := testutil.NewSnapshot(t)
	s, err := OpenWorkflowCore(p, true)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	at := time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC)
	owned := syntheticXPost("old_handle", "1000000000001", "stored wording", at)
	if err = s.WriteXRecent(ctx, "old_handle", xsource.Result{Account: "old_handle", Posts: []xsource.Post{owned},
		ScheduleCandidates: []xsource.ScheduleCandidate{{PostID: owned.ID, SourceURL: owned.URL, MediaURL: owned.MediaURLs[0], AccountHandle: "old_handle"}}}, true); err != nil {
		t.Fatal(err)
	}
	testutil.Exec(t, p, `INSERT INTO sync_state(source,account_id,metadata_json) VALUES('x','new_handle',?)`, `{"pendingStatusIds":["1000000000001","1000000000003"]}`)
	conflict := syntheticXPost("new_handle", owned.ID, "other wording", at)
	conflict.MediaURLs = []string{"https://pbs.twimg.com/media/other.jpg"}
	fresh := syntheticXPost("new_handle", "1000000000002", "new post", at.Add(time.Hour))
	written, err := s.WriteXRecentReport(ctx, "new_handle", xsource.Result{Account: "new_handle", DiscoveredCount: 2, RequestedCount: 2,
		Posts: []xsource.Post{conflict, fresh}, PendingIDs: []string{"1000000000003"},
		ScheduleCandidates: []xsource.ScheduleCandidate{{PostID: conflict.ID, SourceURL: conflict.URL, MediaURL: conflict.MediaURLs[0], AccountHandle: "new_handle"}},
		NextCursor:         fresh.ID, NextCursorTime: fresh.PublishedAt}, true)
	if err != nil {
		t.Fatalf("the conflict failed the account: %v", err)
	}
	if strings.Join(written.Notes, ",") != owned.ID+": "+XPostOwnerConflict {
		t.Fatalf("notes=%v", written.Notes)
	}
	for query, want := range map[string]string{
		`SELECT account_handle||'|'||text FROM posts WHERE id='1000000000001'`:                                 "old_handle|stored wording",
		`SELECT account_handle||'|'||text FROM posts WHERE id='1000000000002'`:                                 "new_handle|new post",
		`SELECT url FROM assets WHERE id='schedule-x-1000000000001'`:                                           "https://pbs.twimg.com/media/1000000000001.jpg",
		`SELECT COALESCE(last_success_at,'')!='' FROM sync_state WHERE source='x' AND account_id='new_handle'`: "1",
	} {
		var got string
		if err = s.db.QueryRow(query).Scan(&got); err != nil || got != want {
			t.Fatalf("%s = %q, want %q (err=%v)", query, got, want, err)
		}
	}
	state, err := s.XRecentState(ctx, "new_handle")
	if err != nil || strings.Join(state.PendingIDs, ",") != "1000000000003" || state.CursorID != fresh.ID {
		t.Fatalf("state=%#v err=%v", state, err)
	}
}

// A stored post whose status a run drops as gone is marked deleted upstream:
// the public board and archive leave it and its schedule image out, the
// console lists it with the mark, and nothing it owns is removed. A listing
// or detail answer that carries the status again clears the mark.
func TestGoneStatusMarksThePostDeletedUpstreamUntilItIsReadAgain(t *testing.T) {
	p := testutil.NewSnapshot(t)
	s, err := OpenWorkflowCore(p, true)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	now := time.Now().UTC()
	post := syntheticXPost("kano", "1000000000010", "今週の配信予定", now.Add(-time.Hour))
	kept := syntheticXPost("kano", "1000000000011", "still there", now.Add(-2*time.Hour))
	if err = s.WriteXRecent(ctx, "kano", xsource.Result{Account: "kano", Posts: []xsource.Post{post, kept},
		ScheduleCandidates: []xsource.ScheduleCandidate{{PostID: post.ID, SourceURL: post.URL, MediaURL: post.MediaURLs[0], AccountHandle: "kano"}}}, true); err != nil {
		t.Fatal(err)
	}
	// The weekly schedule image of the post is on the board.
	testutil.Exec(t, p, `UPDATE assets SET manual_period_start=?,manual_period_end=? WHERE id='schedule-x-1000000000010'`, now.AddDate(0, 0, -1).Format("2006-01-02"), now.AddDate(0, 0, 5).Format("2006-01-02"))
	testutil.Exec(t, p, `UPDATE schedule_asset_reviews SET manual_status='schedule' WHERE asset_id='schedule-x-1000000000010'`)
	shown := func() (board, archive []string, schedule bool, mark *string) {
		t.Helper()
		dashboard, dashErr := s.GetDashboard(ctx, 7, now)
		if dashErr != nil {
			t.Fatal(dashErr)
		}
		for _, item := range dashboard.Posts {
			board = append(board, item.ID)
		}
		// The board and the calendar read the week's schedule images here.
		assets, assetsErr := s.queryScheduleAssets(ctx, now.Format("2006-01-02"), now.AddDate(0, 0, 1).Format("2006-01-02"))
		if assetsErr != nil {
			t.Fatal(assetsErr)
		}
		for _, asset := range assets {
			schedule = schedule || asset.ID == "schedule-x-"+post.ID
		}
		page, archiveErr := s.GetPostTimeline(ctx, "", "", 1, 50)
		if archiveErr != nil {
			t.Fatal(archiveErr)
		}
		for _, item := range page.Items {
			archive = append(archive, item.ID)
		}
		admin, adminErr := s.AdminPostByID(ctx, post.ID)
		if adminErr != nil {
			t.Fatal(adminErr)
		}
		return board, archive, schedule, admin.UpstreamDeletedAt
	}
	if board, archive, schedule, mark := shown(); strings.Join(board, ",") != post.ID+","+kept.ID || len(archive) != 2 || !schedule || mark != nil {
		t.Fatalf("before: board=%v archive=%v schedule=%v mark=%v", board, archive, schedule, mark)
	}

	gone := xsource.Result{Account: "kano", RequestedCount: 1, Dropped: []xsource.DroppedStatus{{ID: post.ID, Code: xsource.DroppedGone}}}
	// Another account's run never marks the post.
	if err = s.WriteXRecent(ctx, "_Kanotic", gone, true); err != nil {
		t.Fatal(err)
	}
	if _, _, _, mark := shown(); mark != nil {
		t.Fatalf("another account marked the post: %v", *mark)
	}
	if err = s.WriteXRecent(ctx, "kano", gone, true); err != nil {
		t.Fatal(err)
	}
	board, archive, schedule, mark := shown()
	if strings.Join(board, ",") != kept.ID || strings.Join(archive, ",") != kept.ID || schedule || mark == nil {
		t.Fatalf("after gone: board=%v archive=%v schedule=%v mark=%v", board, archive, schedule, mark)
	}
	for query, want := range map[string]string{
		`SELECT json_extract(raw_json,'$.text') FROM posts WHERE id='1000000000010'`:            post.Text,
		`SELECT COUNT(*) FROM media_links WHERE owner_type='post' AND owner_id='1000000000010'`: "1",
		`SELECT COUNT(*) FROM assets WHERE id='schedule-x-1000000000010'`:                       "1",
		`SELECT deleted_at IS NULL AND hidden_at IS NULL FROM posts WHERE id='1000000000010'`:   "1",
	} {
		var got string
		if err = s.db.QueryRow(query).Scan(&got); err != nil || got != want {
			t.Fatalf("%s = %q, want %q (err=%v)", query, got, want, err)
		}
	}
	if months, _, monthsErr := s.GetPostMonths(ctx); monthsErr != nil || len(months) != 1 || months[0].Total != 1 {
		t.Fatalf("months=%+v err=%v", months, monthsErr)
	}

	// A timeline page lists the stored status again.
	if err = s.WriteXRecent(ctx, "kano", xsource.Result{Account: "kano", Posts: []xsource.Post{post}, FromListing: map[string]bool{post.ID: true}}, true); err != nil {
		t.Fatal(err)
	}
	if board, _, schedule, mark := shown(); len(board) != 2 || !schedule || mark != nil {
		t.Fatalf("after listing: board=%v schedule=%v mark=%v", board, schedule, mark)
	}
	// So does a detail answer.
	if err = s.WriteXRecent(ctx, "kano", gone, true); err != nil {
		t.Fatal(err)
	}
	if err = s.WriteXRecent(ctx, "kano", xsource.Result{Account: "kano", Posts: []xsource.Post{post}}, true); err != nil {
		t.Fatal(err)
	}
	if board, _, _, mark := shown(); len(board) != 2 || mark != nil {
		t.Fatalf("after detail: board=%v mark=%v", board, mark)
	}
}
