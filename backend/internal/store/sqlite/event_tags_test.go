package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/yexca/kano-navi/backend/internal/testutil"
)

// A label an operator picks is stored with its own source, survives a list
// read, and clearing it leaves the row manual so nothing fills it again.
func TestManualEventTagRoundTrips(t *testing.T) {
	path := testutil.NewSnapshot(t)
	store, err := OpenAdminWrites(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	for _, tag := range EventTags {
		event, err := store.CreateManualEvent(ctx, EventInput{ID: "tag-" + tag, Title: "Synthetic " + tag, StartsOn: "2026-10-12", Tag: tag})
		if err != nil {
			t.Fatalf("%s: %v", tag, err)
		}
		if event.Tag == nil || *event.Tag != tag || event.TagSource == nil || *event.TagSource != TagSourceManual {
			t.Fatalf("%s event=%+v", tag, event)
		}
	}
	if _, err = store.CreateManualEvent(ctx, EventInput{ID: "tag-unknown", Title: "Synthetic", StartsOn: "2026-10-12", Tag: "karaoke"}); err != ErrEventInvalid {
		t.Fatalf("unknown tag err=%v", err)
	}
	if _, err = store.UpdateManualEvent(ctx, "tag-song", EventInput{Title: "Synthetic song", StartsOn: "2026-10-12", Tag: "karaoke"}); err != ErrEventInvalid {
		t.Fatalf("unknown update tag err=%v", err)
	}
	page, err := store.ListAdminEvents(ctx, AdminEventFilter{Page: 1, PageSize: 50})
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	for _, event := range page.Items {
		if event.Tag == nil {
			continue
		}
		seen++
		if event.TagSource == nil || *event.TagSource != TagSourceManual {
			t.Fatalf("%s source=%v", event.ID, event.TagSource)
		}
	}
	if seen != len(EventTags) {
		t.Fatalf("listed %d labelled events, want %d", seen, len(EventTags))
	}
	// Clearing the label keeps the row manual: provenance stays and the
	// source becomes empty, so no later extraction fills it on its own.
	cleared, err := store.UpdateManualEvent(ctx, "tag-video", EventInput{Title: "Synthetic video", StartsOn: "2026-10-12"})
	if err != nil {
		t.Fatal(err)
	}
	if cleared.Tag != nil || cleared.TagSource != nil || cleared.Provenance != "manual" {
		t.Fatalf("cleared=%+v", cleared)
	}
}

// A save that sends the label back unchanged, such as a cancellation
// decision, keeps who decided it; only a changed label becomes the
// operator's.
func TestUnchangedLabelKeepsItsSourceOnSave(t *testing.T) {
	path := testutil.NewSnapshot(t)
	for _, row := range []struct {
		id        string
		tag       any
		tagSource any
	}{{"auto-live", "live", "llm"}, {"auto-judged", nil, "title"}, {"auto-changed", "live", "llm"}} {
		testutil.Exec(t, path, `INSERT INTO events (id,source,source_key,title,starts_on,timezone,time_precision,event_type,provenance,manual_locked,tag,tag_source,created_at,updated_at)
			VALUES (?,'youtube','reservation','Synthetic','2026-10-20','Asia/Tokyo','unknown','stream','automatic',0,?,?,'','')`,
			row.id, row.tag, row.tagSource)
	}
	store, err := OpenAdminWrites(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	for id, input := range map[string]EventInput{
		"auto-live":    {Title: "Synthetic", StartsOn: "2026-10-20", Tag: "live", CancellationStatus: "none"},
		"auto-judged":  {Title: "Synthetic", StartsOn: "2026-10-20", CancellationStatus: "none"},
		"auto-changed": {Title: "Synthetic", StartsOn: "2026-10-20", Tag: "game", CancellationStatus: "none"},
	} {
		if _, err := store.UpdateManualEvent(ctx, id, input); err != nil {
			t.Fatalf("%s: %v", id, err)
		}
	}
	for id, want := range map[string]string{"auto-live": "live|llm", "auto-judged": "|title", "auto-changed": "game|manual"} {
		if got := storeTagState(t, store, id); got != want {
			t.Errorf("%s state=%q, want %q", id, got, want)
		}
	}
}

// The queue offers only automatic YouTube entries that carry no label yet and
// were never judged, and records a null answer so the title is not asked again.
func TestEventTagCandidatesSkipLockedJudgedAndLabelled(t *testing.T) {
	path := testutil.NewSnapshot(t)
	for _, row := range []struct {
		id, source, key, day string
		locked               int
		provenance           string
		tag                  any
		tagSource            any
	}{
		{"free", "youtube", "reservation", "2026-10-20", 0, "automatic", nil, nil},
		{"locked", "youtube", "reservation", "2026-10-20", 1, "manual", nil, nil},
		{"labelled", "youtube", "reservation", "2026-10-20", 0, "automatic", "song", "llm"},
		{"judged", "youtube", "reservation", "2026-10-20", 0, "automatic", nil, "title"},
		{"notice", "x", "status", "2026-10-20", 0, "automatic", nil, nil},
		{"far", "youtube", "reservation", "2027-06-01", 0, "automatic", nil, nil},
		{"lent", "youtube", "reservation", "2026-10-20", 0, "automatic", nil, nil},
		{"lender", "x", "status", "2026-10-20", 0, "automatic", "talk", "llm"},
	} {
		testutil.Exec(t, path, `INSERT INTO events (id,source,source_key,title,starts_on,timezone,time_precision,event_type,provenance,manual_locked,tag,tag_source,created_at,updated_at)
			VALUES (?,?,?,'Synthetic',?,'Asia/Tokyo','unknown','stream',?,?,?,?,'','')`,
			row.id, row.source, row.key, row.day, row.provenance, row.locked, row.tag, row.tagSource)
	}
	// A labelled notice merged into an entry lends it its label, so its title
	// is not worth a model call.
	testutil.Exec(t, path, `UPDATE events SET merged_into='lent',merge_source='rule' WHERE id='lender'`)
	store, err := OpenAdminWrites(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	candidates, err := store.ListEventTagCandidates(ctx, "2026-10-01", "2026-10-31", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 || candidates[0].ID != "free" {
		t.Fatalf("candidates=%+v", candidates)
	}
	if ok, err := store.RecordEventTag(ctx, "locked", nil, tagNow); err != nil || ok {
		t.Fatalf("locked recorded=%v err=%v", ok, err)
	}
	// A null judgement is stored as an attempt: the row keeps no label, the
	// same title is not offered again, and readers see nothing new.
	revision := storeRevision(t, store)
	if ok, err := store.RecordEventTag(ctx, "free", nil, tagNow); err != nil || !ok {
		t.Fatalf("null judgement recorded=%v err=%v", ok, err)
	}
	if got := storeRevision(t, store); got != revision {
		t.Fatalf("null judgement moved the revision %s -> %s", revision, got)
	}
	if got := storeTagState(t, store, "free"); got != "|title" {
		t.Fatalf("null judgement state=%q", got)
	}
	if candidates, err = store.ListEventTagCandidates(ctx, "2026-10-01", "2026-10-31", 10); err != nil || len(candidates) != 0 {
		t.Fatalf("after judgement candidates=%+v err=%v", candidates, err)
	}
	if ok, err := store.RecordEventTag(ctx, "free", nil, tagNow); err != nil || ok {
		t.Fatalf("second judgement recorded=%v err=%v", ok, err)
	}
}

// A notice's label follows it onto the surviving calendar entry. It replaces
// a label the title inference guessed, an empty judgement, or a form label
// with content, and a form never replaces a content label.
func TestMergedNoticeLendsItsTag(t *testing.T) {
	f := newMergeFixture(t)
	f.add("x1", "x", "今夜は歌枠", "2026-10-25", "20:00", "stream", `tag='song'`, `tag_source='llm'`)
	f.add("y1", "youtube", "【歌枠】synthetic", "2026-10-25", "19:59:16", "stream")
	f.add("x2", "x", "今夜は雑談", "2026-10-26", "20:00", "stream", `tag='talk'`, `tag_source='llm'`)
	f.add("y2", "youtube", "【雑談】synthetic", "2026-10-26", "19:59:16", "stream", `tag='live'`, `tag_source='llm'`)
	f.add("x3", "x", "今夜は雑談", "2026-10-27", "20:00", "stream", `tag='talk'`, `tag_source='llm'`)
	f.add("y3", "youtube", "synthetic", "2026-10-27", "19:59:16", "stream", `tag='live'`, `tag_source='title'`)
	f.add("x4", "x", "今夜はゲーム", "2026-10-28", "20:00", "stream", `tag='game'`, `tag_source='llm'`)
	f.add("y4", "youtube", "synthetic", "2026-10-28", "19:59:16", "stream", `tag_source='title'`)
	f.add("x5", "x", "今夜は配信", "2026-10-29", "20:00", "stream", `tag='live'`, `tag_source='llm'`)
	f.add("y5", "youtube", "【マイクラ】synthetic", "2026-10-29", "19:59:16", "stream", `tag='game'`, `tag_source='title'`)
	counts := f.reconcile()
	for id, want := range map[string]string{"x1": "song|llm", "y1": "song|llm", "y2": "talk|llm", "y3": "talk|llm", "y4": "game|llm", "y5": "game|title"} {
		if got := f.tagState(id); got != want {
			t.Errorf("%s label=%q, want %q", id, got, want)
		}
	}
	var target string
	if err := f.s.DB().QueryRow(`SELECT COALESCE(merged_into,'') FROM events WHERE id='x5'`).Scan(&target); err != nil || target != "y5" {
		t.Fatalf("x5 merged into %q err=%v, want y5", target, err)
	}
	if counts.Tagged != 4 {
		t.Fatalf("tagged=%d, want 4", counts.Tagged)
	}
}

// The calendar and the dashboard derive the label the same way, so a tagged
// entry reaches every reader with the same value.
func TestCalendarAndDashboardExposeEventTags(t *testing.T) {
	path := testutil.NewSnapshot(t)
	for _, row := range []struct {
		id, tag, tagSource any
	}{
		{"tagged", "song", "llm"},
		{"untagged", nil, nil},
	} {
		testutil.Exec(t, path, `INSERT INTO events (id,source,title,starts_on,starts_at,timezone,time_precision,event_type,tag,tag_source,url,created_at,updated_at)
			VALUES (?,'youtube','Synthetic','2026-10-24','2026-10-24T11:00:00.000Z','Asia/Tokyo','exact','stream',?,?,'https://www.youtube.com/watch?v=AAAAAAAAAAA','','')`,
			row.id, row.tag, row.tagSource)
	}
	store, err := OpenReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	check := func(route string, events []DashboardEvent) {
		t.Helper()
		labels := map[string]string{}
		for _, event := range events {
			if event.Tag != nil {
				labels[event.ID] = *event.Tag
			} else {
				labels[event.ID] = ""
			}
		}
		want := map[string]string{"tagged": "song", "untagged": ""}
		for id, label := range want {
			if got, ok := labels[id]; !ok || got != label {
				t.Fatalf("%s %s=%q, want %q (all=%v)", route, id, got, label, labels)
			}
		}
	}
	page, err := store.GetCalendarPage(context.Background(), "2026-10-19", "2026-10-25", 1, 100)
	if err != nil {
		t.Fatal(err)
	}
	check("calendar", page.Items)
	dashboard, err := store.GetDashboard(context.Background(), 3, time.Date(2026, 10, 24, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	check("dashboard", dashboard.Events)
}

// tagState is "tag|source", empty fields for NULL.
func (f *mergeFixture) tagState(id string) string {
	f.t.Helper()
	var tag, source string
	if err := f.s.DB().QueryRow(`SELECT COALESCE(tag,''),COALESCE(tag_source,'') FROM events WHERE id=?`, id).Scan(&tag, &source); err != nil {
		f.t.Fatal(err)
	}
	return tag + "|" + source
}
