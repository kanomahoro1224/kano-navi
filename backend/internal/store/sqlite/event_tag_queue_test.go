package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/yexca/kano-navi/backend/internal/testutil"
)

// tagNow is a fixed attempt instant for the label queue tests.
var tagNow = time.Date(2026, 10, 24, 3, 0, 0, 0, time.UTC)

// A stored label is new to readers and moves the revision.
func TestRecordedEventTagMovesRevision(t *testing.T) {
	path := testutil.NewSnapshot(t)
	testutil.Exec(t, path, `INSERT INTO events (id,source,source_key,title,starts_on,timezone,time_precision,event_type,provenance,manual_locked,created_at,updated_at)
		VALUES ('free','youtube','reservation','Synthetic','2026-10-20','Asia/Tokyo','unknown','stream','automatic',0,'','')`)
	store, err := OpenAdminWrites(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	revision := storeRevision(t, store)
	song := "song"
	if ok, err := store.RecordEventTag(context.Background(), "free", &song, tagNow); err != nil || !ok {
		t.Fatalf("recorded=%v err=%v", ok, err)
	}
	if got := storeRevision(t, store); got == revision {
		t.Fatal("stored label left the revision")
	}
	if got := storeTagState(t, store, "free"); got != "song|title" {
		t.Fatalf("state=%q", got)
	}
}

// A failed inference only stamps the entry: it stays a candidate once its
// retry wait has passed, moves behind the entries never attempted and then
// behind more recent failures, so one title that keeps failing cannot hold
// the head of the queue or be sent again on every run.
func TestFailedEventTagAttemptsRotate(t *testing.T) {
	path := testutil.NewSnapshot(t)
	for _, row := range []struct{ id, day string }{{"a", "2026-10-20"}, {"b", "2026-10-21"}, {"c", "2026-10-22"}} {
		testutil.Exec(t, path, `INSERT INTO events (id,source,source_key,title,starts_on,timezone,time_precision,event_type,provenance,manual_locked,created_at,updated_at)
			VALUES (?,'youtube','reservation','Synthetic',?,'Asia/Tokyo','unknown','stream','automatic',0,'','')`, row.id, row.day)
	}
	store, err := OpenAdminWrites(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	ids := func(retryBefore time.Time) string {
		t.Helper()
		candidates, err := store.ListEventTagCandidates(ctx, "2026-10-01", "2026-10-31", retryBefore, 2)
		if err != nil {
			t.Fatal(err)
		}
		out := ""
		for _, item := range candidates {
			out += item.ID
		}
		return out
	}
	later := tagNow.Add(24 * time.Hour)
	if got := ids(later); got != "ab" {
		t.Fatalf("initial=%q", got)
	}
	revision := storeRevision(t, store)
	if err = store.RecordEventTagFailure(ctx, "a", tagNow); err != nil {
		t.Fatal(err)
	}
	if got := storeTagState(t, store, "a"); got != "|" {
		t.Fatalf("failure judged the entry: %q", got)
	}
	if got := storeRevision(t, store); got != revision {
		t.Fatal("failure moved the revision")
	}
	if got := ids(later); got != "bc" {
		t.Fatalf("after one failure=%q", got)
	}
	if err = store.RecordEventTagFailure(ctx, "b", tagNow.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err = store.RecordEventTagFailure(ctx, "c", tagNow.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	// Every entry failed once: none returns before its wait has passed, then
	// the oldest attempt comes back first.
	if got := ids(tagNow.Add(-time.Second)); got != "" {
		t.Fatalf("before the retry wait=%q", got)
	}
	if got := ids(tagNow.Add(time.Minute)); got != "ab" {
		t.Fatalf("after the first retry wait=%q", got)
	}
	if got := ids(later); got != "ab" {
		t.Fatalf("after every entry failed=%q", got)
	}
}

// A merge made before its notice carried a label still lends the label once
// the notice gains one, and a run that changes nothing leaves the revision.
func TestKeptMergeLendsLaterNoticeTag(t *testing.T) {
	f := newMergeFixture(t)
	f.add("x1", "x", "今夜は歌枠", "2026-10-25", "20:00", "stream")
	f.add("y1", "youtube", "【歌枠】synthetic", "2026-10-25", "19:59:16", "stream")
	if counts := f.reconcile(); counts.Merged != 1 || counts.Tagged != 0 {
		t.Fatalf("first run=%+v", counts)
	}
	f.exec(`UPDATE events SET tag='song',tag_source='llm' WHERE id='x1'`)
	revision := f.revision()
	counts := f.reconcile()
	if counts.Kept != 1 || counts.Tagged != 1 {
		t.Fatalf("second run=%+v", counts)
	}
	if got := f.tagState("y1"); got != "song|llm" {
		t.Fatalf("target label=%q", got)
	}
	if f.revision() == revision {
		t.Fatal("inherited label left the revision")
	}
	revision = f.revision()
	if counts = f.reconcile(); counts.Tagged != 0 || counts.Changed() {
		t.Fatalf("third run=%+v", counts)
	}
	if f.revision() != revision {
		t.Fatal("an unchanged run moved the revision")
	}
}

// The notice's label replacing an equal title guess changes nothing readers
// see, so only the source moves.
func TestEqualNoticeTagKeepsRevision(t *testing.T) {
	f := newMergeFixture(t)
	f.add("x1", "x", "今夜は歌枠", "2026-10-25", "20:00", "stream")
	f.add("y1", "youtube", "【歌枠】synthetic", "2026-10-25", "19:59:16", "stream", `tag='song'`, `tag_source='title'`)
	f.reconcile()
	f.exec(`UPDATE events SET tag='song',tag_source='llm' WHERE id='x1'`)
	revision := f.revision()
	if counts := f.reconcile(); counts.Tagged != 0 || counts.Changed() {
		t.Fatalf("run=%+v", counts)
	}
	if got := f.tagState("y1"); got != "song|llm" {
		t.Fatalf("target label=%q", got)
	}
	if f.revision() != revision {
		t.Fatal("equal label moved the revision")
	}
}

// An operator's label, a locked row and a manual row keep their own label
// whatever the notice merged into them says.
func TestInheritedTagSparesOperatorRows(t *testing.T) {
	f := newMergeFixture(t)
	f.add("x1", "x", "今夜は歌枠", "2026-10-25", "20:00", "stream", `tag='song'`, `tag_source='llm'`)
	f.add("y1", "youtube", "synthetic", "2026-10-25", "19:59:16", "stream", `tag='talk'`, `tag_source='manual'`, `provenance='manual'`, `manual_locked=1`)
	f.add("x2", "x", "今夜は歌枠", "2026-10-26", "20:00", "stream", `tag='song'`, `tag_source='llm'`)
	f.add("y2", "youtube", "synthetic", "2026-10-26", "19:59:16", "stream", `tag='live'`, `tag_source='title'`, `manual_locked=1`)
	f.add("x3", "x", "今夜は歌枠", "2026-10-27", "20:00", "stream", `tag='song'`, `tag_source='llm'`)
	f.add("y3", "youtube", "synthetic", "2026-10-27", "19:59:16", "stream", `provenance='manual'`)
	f.reconcile()
	f.expect(map[string]string{"x1": "y1|rule|rule_b:youtube_start", "x2": "y2|rule|rule_b:youtube_start", "x3": "y3|rule|rule_b:youtube_start"})
	for id, want := range map[string]string{"y1": "talk|manual", "y2": "live|title", "y3": "|"} {
		if got := f.tagState(id); got != want {
			t.Errorf("%s label=%q, want %q", id, got, want)
		}
	}
}

// Notices that disagree settle on one label: the smallest notice ID lends it,
// and the inherited label is not replaced on later runs.
func TestConflictingNoticeTagsSettle(t *testing.T) {
	f := newMergeFixture(t)
	f.add("x2", "x", "今夜は雑談", "2026-10-25", "20:05", "stream", `tag='talk'`, `tag_source='llm'`)
	f.add("x1", "x", "今夜は歌枠", "2026-10-25", "20:00", "stream", `tag='song'`, `tag_source='llm'`)
	f.add("y1", "youtube", "synthetic", "2026-10-25", "19:59:16", "stream")
	f.reconcile()
	f.expect(map[string]string{"x1": "y1|rule|rule_b:youtube_start", "x2": "y1|rule|rule_b:youtube_start"})
	if got := f.tagState("y1"); got != "song|llm" {
		t.Fatalf("target label=%q", got)
	}
	for range 3 {
		if counts := f.reconcile(); counts.Tagged != 0 {
			t.Fatalf("label moved again: %+v", counts)
		}
	}
	f.exec(`UPDATE events SET tag='game' WHERE id='x1'`)
	f.reconcile()
	if got := f.tagState("y1"); got != "song|llm" {
		t.Fatalf("inherited label replaced: %q", got)
	}
}

func storeRevision(t *testing.T, store *Store) string {
	t.Helper()
	var revision string
	if err := store.db.QueryRow(`SELECT COALESCE((SELECT value FROM app_settings WHERE key='dashboard_revision'),'0')`).Scan(&revision); err != nil {
		t.Fatal(err)
	}
	return revision
}

// storeTagState is "tag|source", empty fields for NULL.
func storeTagState(t *testing.T, store *Store, id string) string {
	t.Helper()
	var tag, source string
	if err := store.db.QueryRow(`SELECT COALESCE(tag,''),COALESCE(tag_source,'') FROM events WHERE id=?`, id).Scan(&tag, &source); err != nil {
		t.Fatal(err)
	}
	return tag + "|" + source
}

func (f *mergeFixture) revision() string {
	f.t.Helper()
	return storeRevision(f.t, f.s)
}
