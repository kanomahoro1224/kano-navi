package sqlite

import (
	"context"
	"testing"
)

// visibilityFixture stores two X posts, one hidden, and events whose
// evidence names them.
func visibilityFixture(t *testing.T) *mergeFixture {
	t.Helper()
	f := newMergeFixture(t)
	for _, post := range []struct{ id, hidden string }{{"p-hidden", "'2026-10-20T00:00:00.000Z'"}, {"p-shown", "NULL"}} {
		f.exec(`INSERT INTO posts (id,source,account_handle,type,text,published_at,url,hidden_at)
			VALUES (?,'x','example','post','synthetic post','2026-10-20T00:00:00.000Z',?,`+post.hidden+`)`,
			post.id, "https://x.com/example/status/"+post.id)
	}
	return f
}

// evidence attaches an active source row to an event.
func (f *mergeFixture) evidence(eventID, source, itemID string) {
	f.t.Helper()
	f.exec(`INSERT INTO event_sources (event_id,source,source_item_id,source_key,created_at,updated_at)
		VALUES (?,?,?,?,'2026-10-01T00:00:00.000Z','2026-10-01T00:00:00.000Z')`, eventID, source, itemID, "key-"+eventID)
}

func (f *mergeFixture) publicIDs() map[string]bool {
	f.t.Helper()
	ctx := context.Background()
	page, err := f.s.GetCalendarPage(ctx, "2026-10-19", "2026-10-31", 1, 100)
	if err != nil {
		f.t.Fatal(err)
	}
	calendar := map[string]bool{}
	for _, item := range page.Items {
		calendar[item.ID] = true
	}
	if int(page.Total) != len(calendar) {
		f.t.Errorf("calendar total %d lists %d items", page.Total, len(calendar))
	}
	dashboard, err := f.s.GetDashboard(ctx, 7, mergeNow)
	if err != nil {
		f.t.Fatal(err)
	}
	board := map[string]bool{}
	for _, event := range dashboard.Events {
		board[event.ID] = true
	}
	if len(board) != len(calendar) || int(dashboard.Summary.Counts.Events) != len(calendar) {
		f.t.Errorf("dashboard %v (count %d) differs from calendar %v", board, dashboard.Summary.Counts.Events, calendar)
	}
	public, err := f.s.ListAdminEvents(ctx, AdminEventFilter{Merge: "visible", Public: true, PageSize: 100})
	if err != nil {
		f.t.Fatal(err)
	}
	if public.Total != len(calendar) {
		f.t.Errorf("public admin filter lists %d events, calendar %d", public.Total, len(calendar))
	}
	return calendar
}

func TestEventsOfHiddenPostsLeavePublicReads(t *testing.T) {
	f := visibilityFixture(t)
	ctx := context.Background()
	// Every source post hidden: left out.
	f.add("hidden", "x", "雑談", "2026-10-25", "20:00", "stream")
	f.evidence("hidden", "x", "p-hidden")
	// An operator confirmed or added the entry: the decision outranks hiding.
	f.add("locked", "x", "歌枠", "2026-10-25", "21:00", "stream", "manual_locked=1")
	f.evidence("locked", "x", "p-hidden")
	f.add("manual", "manual", "記念配信", "2026-10-26", "20:00", "stream", "provenance='manual'")
	f.evidence("manual", "x", "p-hidden")
	// Any other source keeps it shown: a YouTube reservation, a visible post,
	// or a visible notice merged into it.
	f.add("reserved", "youtube", "朝活", "2026-10-26", "08:00", "stream")
	f.evidence("reserved", "x", "p-hidden")
	f.evidence("reserved", "youtube", "synthetic-video")
	f.add("both", "x", "ゲーム", "2026-10-27", "20:00", "stream")
	f.evidence("both", "x", "p-hidden")
	f.exec(`INSERT INTO event_sources (event_id,source,source_item_id,source_key,created_at,updated_at)
		VALUES ('both','x','p-shown','key-both-shown','2026-10-01T00:00:00.000Z','2026-10-01T00:00:00.000Z')`)
	f.add("target", "x", "お絵描き", "2026-10-28", "20:00", "stream")
	f.evidence("target", "x", "p-hidden")
	f.add("notice", "x", "お絵描き", "2026-10-28", "", "stream", "merged_into='target'", "merge_source='manual'", "merge_reason='manual'")
	f.evidence("notice", "x", "p-shown")
	// No evidence at all is not a hidden source.
	f.add("bare", "x", "ASMR", "2026-10-29", "20:00", "stream")
	// A retired source row does not count.
	f.add("retired", "x", "朗読", "2026-10-30", "20:00", "stream")
	f.evidence("retired", "x", "p-hidden")
	f.exec(`INSERT INTO event_sources (event_id,source,source_item_id,source_key,deleted_at,created_at,updated_at)
		VALUES ('retired','x','p-shown','key-retired-old','2026-10-02T00:00:00.000Z','2026-10-01T00:00:00.000Z','2026-10-01T00:00:00.000Z')`)

	shown := f.publicIDs()
	for _, id := range []string{"locked", "manual", "reserved", "both", "target", "bare"} {
		if !shown[id] {
			t.Errorf("%s left the public reads", id)
		}
	}
	for _, id := range []string{"hidden", "retired", "notice"} {
		if shown[id] {
			t.Errorf("%s stayed public", id)
		}
	}

	// The console lists every row and names the ones hidden by their posts.
	all, err := f.s.ListAdminEvents(ctx, AdminEventFilter{PageSize: 100})
	if err != nil {
		t.Fatal(err)
	}
	flagged := map[string]bool{}
	for _, item := range all.Items {
		flagged[item.ID] = item.HiddenByPosts
	}
	if len(flagged) != 9 || !flagged["hidden"] || !flagged["retired"] || flagged["locked"] || flagged["manual"] || flagged["reserved"] || flagged["both"] || flagged["target"] || flagged["bare"] {
		t.Fatalf("console flags = %v", flagged)
	}
	if one, err := f.s.GetAdminEvent(ctx, "hidden"); err != nil || !one.HiddenByPosts {
		t.Fatalf("single event = %+v %v", one, err)
	}

	// Showing the post shows its events again; the rows were never changed.
	f.exec(`UPDATE posts SET hidden_at=NULL WHERE id='p-hidden'`)
	shown = f.publicIDs()
	if !shown["hidden"] || !shown["retired"] {
		t.Fatalf("shown post kept its events hidden: %v", shown)
	}
}
