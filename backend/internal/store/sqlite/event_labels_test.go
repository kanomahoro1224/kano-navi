package sqlite

import (
	"context"
	"slices"
	"strings"
	"testing"
)

// labelVideo inserts a synthetic video with optional operator and inferred
// tags ("" for NULL).
func (f *mergeFixture) labelVideo(id, manualTag, tag string) {
	f.t.Helper()
	f.video(id, "synthetic", "2026-10-25T11:00:00.000Z", "2026-10-25T11:00:00.000Z", "", "0", 3600)
	f.exec(`UPDATE videos SET manual_tag=NULLIF(?,''),tag=NULLIF(?,''),tag_source=CASE WHEN ?<>'' THEN 'title' END WHERE id=?`, manualTag, tag, tag, id)
}

// The calendar shows an operator's entry label as set, then a video's
// operator tag, the entry's content label, a video's inferred tag and the
// entry's form label, in that order.
func TestCalendarLabelPrefersContentAcrossEntryAndVideo(t *testing.T) {
	f := newMergeFixture(t)
	f.labelVideo("VIDEOFORM01", "", "game")
	f.add("youtube-VIDEOFORM01", "youtube", "【マイクラ】synthetic", "2026-10-25", "20:00", "stream", `tag='live'`, `tag_source='llm'`)
	f.labelVideo("VIDEOMANUAL", "song", "")
	f.add("youtube-VIDEOMANUAL", "youtube", "synthetic", "2026-10-25", "20:01", "stream", `tag='game'`, `tag_source='title'`)
	f.labelVideo("ENTRYMANUAL", "song", "")
	f.add("youtube-ENTRYMANUAL", "youtube", "synthetic", "2026-10-25", "20:02", "stream", `tag='talk'`, `tag_source='manual'`, `provenance='manual'`, `manual_locked=1`)
	f.labelVideo("ENTRYCLEARD", "song", "")
	// An operator's row without a label shows what its video says.
	f.add("youtube-ENTRYCLEARD", "youtube", "synthetic", "2026-10-25", "20:03", "stream", `provenance='manual'`, `manual_locked=1`)
	f.labelVideo("ENTRYCONTNT", "", "talk")
	f.add("youtube-ENTRYCONTNT", "youtube", "synthetic", "2026-10-25", "20:04", "stream", `tag='song'`, `tag_source='llm'`)
	// The video belongs to an entry merged into the one readers see.
	f.labelVideo("MERGEDCHILD", "game", "")
	f.add("notice", "x", "synthetic", "2026-10-25", "20:05", "stream", `tag='live'`, `tag_source='llm'`)
	f.add("youtube-MERGEDCHILD", "youtube", "synthetic", "2026-10-25", "20:05", "stream", `merged_into='notice'`, `merge_source='rule'`)
	f.add("formonly", "x", "synthetic", "2026-10-25", "20:06", "release", `tag='video'`, `tag_source='llm'`)
	f.add("unlabelled", "x", "synthetic", "2026-10-25", "20:07", "stream")

	page, err := f.s.GetCalendarPage(context.Background(), "2026-10-25", "2026-10-25", 1, 100)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, event := range page.Items {
		got[event.ID] = ""
		if event.Tag != nil {
			got[event.ID] = *event.Tag
		}
	}
	want := map[string]string{
		"youtube-VIDEOFORM01": "game", "youtube-VIDEOMANUAL": "song", "youtube-ENTRYMANUAL": "talk",
		"youtube-ENTRYCLEARD": "song", "youtube-ENTRYCONTNT": "song", "notice": "game", "formonly": "video", "unlabelled": "",
	}
	for id, label := range want {
		if value, ok := got[id]; !ok || value != label {
			t.Errorf("%s label=%q (present=%v), want %q", id, value, ok, label)
		}
	}
	// The stored row keeps its own label; the order is applied on each read.
	if state := f.tagState("youtube-VIDEOFORM01"); state != "live|llm" {
		t.Fatalf("stored label=%q", state)
	}
}

// Every public event read derives the label per row, so it must look each
// entry and video up by ID instead of walking YouTube rows by source.
func TestEventLabelLooksEntriesAndVideosUpByID(t *testing.T) {
	f := newMergeFixture(t)
	rows, err := f.s.DB().Query(`EXPLAIN QUERY PLAN SELECT `+eventLabelSQL("o")+` FROM events o WHERE o.id=?`, "synthetic")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	steps := 0
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		steps++
		if strings.HasPrefix(detail, "SCAN ") && detail != "SCAN CONSTANT ROW" || strings.Contains(detail, "(source=?") {
			t.Errorf("plan step %q does not look up by ID", detail)
		}
	}
	if err := rows.Err(); err != nil || steps == 0 {
		t.Fatalf("plan steps=%d err=%v", steps, err)
	}
}

// A video reads a content label from its entry, and a form label on the
// entry it is merged into does not hide that.
func TestVideoTagReadsEntryContentPastATargetForm(t *testing.T) {
	f := newMergeFixture(t)
	f.labelVideo("CHILDCONTNT", "", "")
	f.add("target", "x", "synthetic", "2026-10-25", "20:00", "stream", `tag='live'`, `tag_source='llm'`)
	f.add("youtube-CHILDCONTNT", "youtube", "synthetic", "2026-10-25", "20:00", "stream", `tag='game'`, `tag_source='title'`, `merged_into='target'`, `merge_source='rule'`)
	var tag string
	if err := f.s.DB().QueryRow(`SELECT COALESCE(` + videoTagSQL("videos") + `,'') FROM videos WHERE id='CHILDCONTNT'`).Scan(&tag); err != nil {
		t.Fatal(err)
	}
	if tag != "game" {
		t.Fatalf("video tag=%q, want game", tag)
	}
}

// A video whose entry carries only a form label from a notice, or that an
// operator saved without a label, is left to its own title; content, a title
// judgement, an operator's label or a pending reservation judgement keep it
// out, and a Bilibili upload, which has no entry, is asked about too.
func TestVideoTagCandidatesSkipOnlyDecidedEntries(t *testing.T) {
	f := newMergeFixture(t)
	add := func(video, day string, options ...string) {
		f.labelVideo(video, "", "")
		f.add("youtube-"+video, "youtube", "synthetic", day, "20:00", "stream", options...)
	}
	add("FORMNOTICE1", "2026-10-01", `tag='live'`, `tag_source='llm'`)
	add("FORMTITLE01", "2026-10-01", `tag='live'`, `tag_source='title'`)
	add("CONTENTLLM1", "2026-10-01", `tag='game'`, `tag_source='llm'`)
	add("OPERATOR001", "2026-10-01", `tag='live'`, `tag_source='manual'`, `provenance='manual'`, `manual_locked=1`)
	add("PENDINGRES1", "2026-10-30", `source_key='reservation'`)
	add("FORMRES0001", "2026-10-30", `source_key='reservation'`, `tag='live'`, `tag_source='llm'`)
	add("OPERATOR002", "2026-10-01", `provenance='manual'`, `manual_locked=1`)
	add("LOCKEDRES01", "2026-10-30", `source_key='reservation'`, `manual_locked=1`)
	f.labelVideo("BV1BBBBBBBBB", "", "")
	f.exec(`UPDATE videos SET source='bilibili' WHERE id='BV1BBBBBBBBB'`)
	f.add("contenttarget", "x", "synthetic", "2026-10-02", "20:00", "stream", `tag='song'`, `tag_source='llm'`)
	add("MERGEDSONG1", "2026-10-02", `merged_into='contenttarget'`, `merge_source='rule'`)
	f.add("formtarget", "x", "synthetic", "2026-10-03", "20:00", "stream", `tag='live'`, `tag_source='llm'`)
	add("MERGEDFORM1", "2026-10-03", `merged_into='formtarget'`, `merge_source='rule'`)

	candidates, err := f.s.ListVideoTagCandidates(context.Background(), "2026-10-20", tagNow, 20)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, candidate := range candidates {
		got[candidate.ID] = true
	}
	for id, want := range map[string]bool{
		"FORMNOTICE1": true, "FORMTITLE01": false, "CONTENTLLM1": false, "OPERATOR001": false,
		"PENDINGRES1": false, "FORMRES0001": true, "MERGEDSONG1": false, "MERGEDFORM1": true,
		"OPERATOR002": true, "LOCKEDRES01": true, "BV1BBBBBBBBB": false,
	} {
		if got[id] != want {
			t.Errorf("%s candidate=%v, want %v", id, got[id], want)
		}
	}
}

// An operator who clears an entry's label decides it has none: the video is
// not sent to the title inference because of the clear, and the entry shows
// no label.
func TestClearedEntryLabelHoldsBackItsVideo(t *testing.T) {
	f := newMergeFixture(t)
	f.labelVideo("CLEARED0001", "", "")
	f.add("youtube-CLEARED0001", "youtube", "synthetic", "2026-10-01", "20:00", "stream", `source_key='reservation'`, `tag='game'`, `tag_source='title'`)
	ctx := context.Background()
	candidates := func() int {
		f.t.Helper()
		items, err := f.s.ListVideoTagCandidates(ctx, "2026-09-01", tagNow, 20)
		if err != nil {
			t.Fatal(err)
		}
		return len(items)
	}
	if n := candidates(); n != 0 {
		t.Fatalf("before the clear candidates=%d", n)
	}
	if _, err := f.s.UpdateManualEvent(ctx, "youtube-CLEARED0001", EventInput{Title: "synthetic", StartsOn: "2026-10-01"}); err != nil {
		t.Fatal(err)
	}
	if n := candidates(); n != 0 {
		t.Fatalf("the clear made the video a candidate: %d", n)
	}
	if got := f.tagState("youtube-CLEARED0001"); got != "|manual" {
		t.Fatalf("cleared state=%q", got)
	}
	var label string
	if err := f.s.DB().QueryRow(`SELECT COALESCE(` + eventLabelSQL("e") + `,'') FROM events e WHERE id='youtube-CLEARED0001'`).Scan(&label); err != nil {
		t.Fatal(err)
	}
	if label != "" {
		t.Fatalf("cleared entry label=%q", label)
	}
}

// Saving an entry's label as automatic hands it back: a locked entry judged
// without content, or one an operator cleared, no longer holds its video back,
// forgets the attempt, and reads on to the video's inferred tag.
func TestAutomaticLabelReleasesALockedEntry(t *testing.T) {
	f := newMergeFixture(t)
	ctx := context.Background()
	for _, id := range []string{"JUDGEDLOCK1", "CLEAREDLOC1"} {
		f.labelVideo(id, "", "")
	}
	f.add("youtube-JUDGEDLOCK1", "youtube", "synthetic", "2026-10-01", "20:00", "stream", `source_key='reservation'`, `tag_source='title'`, `tag_attempted_at='2026-10-01T00:00:00.000Z'`, `provenance='manual'`, `manual_locked=1`)
	f.add("youtube-CLEAREDLOC1", "youtube", "synthetic", "2026-10-01", "20:00", "stream", `source_key='reservation'`, `tag_source='manual'`, `provenance='manual'`, `manual_locked=1`)
	candidates := func() string {
		f.t.Helper()
		items, err := f.s.ListVideoTagCandidates(ctx, "2026-09-01", tagNow, 20)
		if err != nil {
			t.Fatal(err)
		}
		ids := []string{}
		for _, item := range items {
			ids = append(ids, item.ID)
		}
		slices.Sort(ids)
		return strings.Join(ids, ",")
	}
	if got := candidates(); got != "" {
		t.Fatalf("before candidates=%q", got)
	}
	for _, id := range []string{"youtube-JUDGEDLOCK1", "youtube-CLEAREDLOC1"} {
		saved, err := f.s.UpdateManualEvent(ctx, id, EventInput{Title: "synthetic", StartsOn: "2026-10-01", Tag: TagAutomatic})
		if err != nil {
			t.Fatal(err)
		}
		if saved.Tag != nil || saved.TagSource != nil || saved.Provenance != "manual" {
			t.Fatalf("%s saved=%+v", id, saved)
		}
	}
	if got := candidates(); got != "CLEAREDLOC1,JUDGEDLOCK1" {
		t.Fatalf("after candidates=%q", got)
	}
	var attempted int
	if err := f.s.DB().QueryRow(`SELECT COUNT(*) FROM events WHERE tag_attempted_at IS NOT NULL`).Scan(&attempted); err != nil || attempted != 0 {
		t.Fatalf("attempted=%d err=%v", attempted, err)
	}
	f.exec(`UPDATE videos SET tag='talk',tag_source='title' WHERE id='JUDGEDLOCK1'`)
	var label string
	if err := f.s.DB().QueryRow(`SELECT COALESCE(` + eventLabelSQL("e") + `,'') FROM events e WHERE id='youtube-JUDGEDLOCK1'`).Scan(&label); err != nil {
		t.Fatal(err)
	}
	if label != "talk" {
		t.Fatalf("automatic entry label=%q", label)
	}
	// A new entry saved as automatic stores no label.
	created, err := f.s.CreateManualEvent(ctx, EventInput{ID: "auto-new", Title: "synthetic", StartsOn: "2026-10-02", Tag: TagAutomatic})
	if err != nil || created.Tag != nil || created.TagSource != nil {
		t.Fatalf("created=%+v err=%v", created, err)
	}
}
