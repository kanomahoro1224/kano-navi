package sqlite

import (
	"context"
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
// out, and a Bilibili upload, never shown, is not asked about.
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

	candidates, err := f.s.ListVideoTagCandidates(context.Background(), "2026-10-20", 20)
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
