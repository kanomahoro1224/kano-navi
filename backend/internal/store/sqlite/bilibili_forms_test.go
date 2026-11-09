package sqlite

import (
	"context"
	"strings"
	"testing"
	"time"
)

var formNow = time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC)

// broadcastEntry inserts a stream entry shown on platforms from start for
// minutes (0 leaves its end unknown), linked to a YouTube video when youtube
// is not empty.
func (f *videoMergeFixture) broadcastEntry(id, platforms, start string, minutes int, youtube string) {
	f.t.Helper()
	at, err := time.Parse(time.RFC3339, start)
	if err != nil {
		f.t.Fatal(err)
	}
	var end any
	if minutes > 0 {
		end = at.Add(time.Duration(minutes) * time.Minute).UTC().Format(time.RFC3339)
	}
	source, item := "x", any(nil)
	if youtube != "" {
		source, item = "youtube", youtube
	}
	f.exec(`INSERT INTO events(id,source,source_item_id,title,starts_on,starts_at,ends_at,event_type,platform,platforms,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,'stream',?,?,?,?)`, id, source, item, "synthetic broadcast "+id, start[:10], start, end,
		strings.Split(strings.Trim(platforms, `[]"`), `"`)[0], platforms, start, start)
}

// form is "kind|kind_source|kind_review" of a video.
func (f *videoMergeFixture) form(id string) string {
	f.t.Helper()
	var out string
	if err := f.s.DB().QueryRow(`SELECT COALESCE(kind,'')||'|'||COALESCE(kind_source,'')||'|'||COALESCE(kind_review,'') FROM videos WHERE id=?`, id).Scan(&out); err != nil {
		f.t.Fatal(err)
	}
	return out
}

func (f *videoMergeFixture) settle() BilibiliFormCounts {
	f.t.Helper()
	counts, err := f.s.SettleBilibiliForms(context.Background(), formNow)
	if err != nil {
		f.t.Fatal(err)
	}
	return counts
}

// Each form rule settles an upload, and an upload no rule covers stays
// unsettled for the model.
func TestBilibiliFormRules(t *testing.T) {
	f := newVideoMergeFixture(t)
	f.exec(`UPDATE videos SET kind='upload'`)
	// A two-hour broadcast on Bilibili that ended at 14:00 UTC on 03-01.
	f.broadcastEntry("cast", `["bilibili"]`, "2026-03-01T12:00:00Z", 120, "")
	// The live room recorded a three-hour broadcast ending at 15:00 on 03-05.
	f.exec(`INSERT INTO sync_state(source,account_id,metadata_json) VALUES('bilibili-live','1','{"lastBroadcast":"2026-03-05T12:00:00Z","lastLiveAt":"2026-03-05T15:00:00Z","endedAt":"2026-03-05T15:05:00Z"}')`)
	for _, u := range []struct {
		id, title, published string
		duration             int
	}{
		{"BV1MARKER001", "【直播回放】synthetic", "2026-02-01T00:00:00Z", 0},
		{"BV1MARKER002", "synthetic 录播", "2026-02-01T00:00:00Z", 200},
		{"BV1ENTRYCAST", "synthetic night", "2026-03-02T10:00:00Z", 6000},
		{"BV1ROOMCAST1", "synthetic room", "2026-03-06T09:00:00Z", 9000},
		{"BV1TOOSHORT1", "synthetic half", "2026-03-02T10:00:00Z", 3000},
		{"BV1TOOLATE01", "synthetic late", "2026-03-04T10:00:00Z", 6000},
		{"BV1SHORTCLIP", "synthetic clip", "2026-02-01T00:00:00Z", 300},
		{"BV1UNKNOWN01", "synthetic unknown", "2026-02-01T00:00:00Z", 0},
	} {
		f.bilibili(u.id, u.title, u.published, u.duration)
	}
	f.exec(`UPDATE videos SET kind='upload' WHERE source='bilibili'`)
	// An operator's form and a stored one are never touched.
	f.bilibili("BV1OPERATOR1", "【直播回放】operator", "2026-02-01T00:00:00Z", 0)
	f.exec(`UPDATE videos SET kind='upload',manual_kind='upload' WHERE id='BV1OPERATOR1'`)
	counts := f.settle()
	if counts.Settled != 5 {
		t.Fatalf("counts=%+v", counts)
	}
	for id, want := range map[string]string{
		"BV1MARKER001": "stream|rule|",
		"BV1MARKER002": "stream|rule|",
		"BV1ENTRYCAST": "stream|rule|",
		"BV1ROOMCAST1": "stream|rule|",
		"BV1TOOSHORT1": "upload||", // 50 of 120 minutes is less than half
		"BV1TOOLATE01": "upload||", // more than 48 hours after the end
		"BV1SHORTCLIP": "upload|rule|",
		"BV1UNKNOWN01": "upload||",
		"BV1OPERATOR1": "upload||",
	} {
		if got := f.form(id); got != want {
			t.Errorf("%s form=%q, want %q", id, got, want)
		}
	}
	if again := f.settle(); again.Settled != 0 || again.Considered != 3 {
		t.Fatalf("second run=%+v", again)
	}
}

// A live replay merges into the YouTube stream archive linked to the same
// schedule entry; an upload whose form is unsettled or upload does not, and
// an operator's decision outranks the rule.
func TestBilibiliReplaysMergeIntoTheSimulcastArchive(t *testing.T) {
	f := newVideoMergeFixture(t)
	f.youtube("STREAMARCH1", "【歌枠】synthetic", "2026-03-01T12:00:00Z", "2026-03-01T12:00:00Z", 7200)
	f.youtube("STREAMARCH2", "【雑談】synthetic", "2026-03-03T12:00:00Z", "2026-03-03T12:00:00Z", 3600)
	f.exec(`UPDATE videos SET kind='stream' WHERE source='youtube'`)
	// A simulcast: the YouTube reservation shown on both platforms, its end
	// read from the archive's length.
	f.broadcastEntry("youtube-STREAMARCH1", `["youtube","bilibili"]`, "2026-03-01T12:00:00Z", 0, "STREAMARCH1")
	f.broadcastEntry("youtube-STREAMARCH2", `["youtube","bilibili"]`, "2026-03-03T12:00:00Z", 40, "STREAMARCH2")
	f.bilibili("BV1REPLAY001", "【直播回放】synthetic", "2026-03-02T08:00:00Z", 7000)
	// Shorter than thirty minutes: no rule settles it.
	f.bilibili("BV1UNSETTLED", "synthetic", "2026-03-03T20:00:00Z", 1500)
	f.exec(`UPDATE videos SET kind='upload' WHERE source='bilibili'`)
	f.settle()
	f.reconcile()
	f.expect(map[string]string{
		"BV1REPLAY001": "STREAMARCH1|rule|rule:broadcast",
		"BV1UNSETTLED": "||",
	})
	// The model settles the second replay; the next run merges it.
	if _, err := f.s.RecordBilibiliForm(context.Background(), "BV1UNSETTLED", BilibiliFormStream, nil, formNow); err != nil {
		t.Fatal(err)
	}
	f.reconcile()
	f.expect(map[string]string{"BV1UNSETTLED": "STREAMARCH2|rule|rule:broadcast"})
	suggestion, err := f.s.VideoMergeSuggestions(context.Background(), "BV1REPLAY001")
	if err != nil || len(suggestion) != 1 || suggestion[0].Status != "merged" {
		t.Fatalf("suggestion=%+v err=%v", suggestion, err)
	}
	// An operator's undo keeps the replay apart, and the rule never redoes it.
	if _, err = f.s.UnmergeVideo(context.Background(), "BV1REPLAY001", formNow); err != nil {
		t.Fatal(err)
	}
	f.reconcile()
	f.expect(map[string]string{"BV1REPLAY001": "|manual|unmerged_from:STREAMARCH1"})
}

// A replay merges only into the archive of the broadcast it replays: a
// Bilibili-only broadcast's replay stays on its own rather than reaching an
// earlier simulcast's archive, and a live room's record stands for the
// schedule entry it overlaps.
func TestBilibiliReplaysMergeOnlyIntoTheirOwnBroadcast(t *testing.T) {
	f := newVideoMergeFixture(t)
	f.youtube("STREAMARCHA", "【歌枠】synthetic", "2026-03-01T12:00:00Z", "2026-03-01T12:00:00Z", 7200)
	f.youtube("STREAMARCHC", "【雑談】synthetic", "2026-03-05T12:00:00Z", "2026-03-05T12:00:00Z", 10800)
	f.youtube("STREAMARCHD", "【朗読】synthetic", "2026-03-07T12:00:00Z", "2026-03-07T12:00:00Z", 7200)
	f.exec(`UPDATE videos SET kind='stream' WHERE source='youtube'`)
	// A simulcast, then a Bilibili-only broadcast the next day.
	f.broadcastEntry("youtube-STREAMARCHA", `["youtube","bilibili"]`, "2026-03-01T12:00:00Z", 0, "STREAMARCHA")
	f.broadcastEntry("bilibili-only", `["bilibili"]`, "2026-03-02T12:00:00Z", 120, "")
	// Simulcasts whose end is read from the archive; the room recorded the
	// first one ending ten minutes after its archive, and then a broadcast
	// with no schedule entry the day after the second.
	f.broadcastEntry("youtube-STREAMARCHC", `["youtube","bilibili"]`, "2026-03-05T12:00:00Z", 0, "STREAMARCHC")
	f.broadcastEntry("youtube-STREAMARCHD", `["youtube","bilibili"]`, "2026-03-07T12:00:00Z", 0, "STREAMARCHD")
	f.exec(`INSERT INTO sync_state(source,account_id,metadata_json) VALUES('bilibili-live','1','{"lastBroadcast":"2026-03-08T12:00:00Z","lastLiveAt":"2026-03-08T14:00:00Z","endedAt":"2026-03-08T14:05:00Z","previousBroadcast":"2026-03-05T12:00:00Z","previousEndedAt":"2026-03-05T15:10:00Z"}')`)
	f.bilibili("BV1REPLAYAAA", "【直播回放】synthetic simulcast", "2026-03-01T16:00:00Z", 7000)
	f.bilibili("BV1REPLAYBBB", "【直播回放】synthetic bilibili only", "2026-03-02T15:00:00Z", 7000)
	f.bilibili("BV1REPLAYCCC", "【直播回放】synthetic room", "2026-03-06T09:00:00Z", 10500)
	f.bilibili("BV1REPLAYDDD", "【直播回放】synthetic room only", "2026-03-08T15:00:00Z", 7000)
	f.exec(`UPDATE videos SET kind='upload' WHERE source='bilibili'`)
	f.settle()
	f.reconcile()
	f.expect(map[string]string{
		"BV1REPLAYAAA": "STREAMARCHA|rule|rule:broadcast",
		"BV1REPLAYBBB": "||",
		"BV1REPLAYCCC": "STREAMARCHC|rule|rule:broadcast",
		"BV1REPLAYDDD": "||",
	})
	for _, id := range []string{"BV1REPLAYBBB", "BV1REPLAYDDD"} {
		if got := f.form(id); got != "stream|rule|" {
			t.Errorf("%s form=%q", id, got)
		}
	}
}

// A rule that applies only once its broadcast is known replaces the form
// the model settled before, and the replay then merges; a model's form the
// rule agrees with, and an operator's, stay as they are.
func TestBilibiliRulesReplaceAnEarlierModelForm(t *testing.T) {
	f := newVideoMergeFixture(t)
	ctx := context.Background()
	f.bilibili("BV1LATECAST1", "synthetic night", "2026-03-02T10:00:00Z", 6000)
	f.bilibili("BV1AGREEING1", "synthetic clip", "2026-03-02T10:00:00Z", 300)
	f.bilibili("BV1OPERATOR2", "synthetic operator", "2026-03-02T10:00:00Z", 6000)
	f.exec(`UPDATE videos SET kind='upload' WHERE source='bilibili'`)
	f.exec(`UPDATE videos SET kind_source='llm' WHERE id='BV1AGREEING1'`)
	f.exec(`UPDATE videos SET manual_kind='upload' WHERE id='BV1OPERATOR2'`)
	if counts := f.settle(); counts.Settled != 0 {
		t.Fatalf("before the broadcast=%+v", counts)
	}
	if ok, err := f.s.RecordBilibiliForm(ctx, "BV1LATECAST1", BilibiliFormUpload, nil, formNow); err != nil || !ok {
		t.Fatalf("model ok=%v err=%v", ok, err)
	}
	// The broadcast and its archive are read afterwards.
	f.youtube("STREAMLATE1", "【歌枠】synthetic", "2026-03-01T12:00:00Z", "2026-03-01T12:00:00Z", 7200)
	f.exec(`UPDATE videos SET kind='stream' WHERE source='youtube'`)
	f.broadcastEntry("youtube-STREAMLATE1", `["youtube","bilibili"]`, "2026-03-01T12:00:00Z", 120, "STREAMLATE1")
	if counts := f.settle(); counts.Settled != 1 || counts.Considered != 0 {
		t.Fatalf("after the broadcast=%+v", counts)
	}
	for id, want := range map[string]string{
		"BV1LATECAST1": "stream|rule|",
		"BV1AGREEING1": "upload|llm|",
		"BV1OPERATOR2": "upload||",
	} {
		if got := f.form(id); got != want {
			t.Errorf("%s form=%q, want %q", id, got, want)
		}
	}
	f.reconcile()
	f.expect(map[string]string{"BV1LATECAST1": "STREAMLATE1|rule|rule:broadcast", "BV1OPERATOR2": "||"})
	if again := f.settle(); again.Settled != 0 {
		t.Fatalf("second run=%+v", again)
	}
}

// The Review queue lists the uploads the model was uncertain about or gave
// up on, with the broadcast the replay rule read, until the operator sets a
// form; settled, merged and removed uploads stay out.
func TestBilibiliFormReviewQueue(t *testing.T) {
	f := newVideoMergeFixture(t)
	ctx := context.Background()
	f.broadcastEntry("cast", `["bilibili"]`, "2026-03-01T12:00:00Z", 120, "")
	for _, id := range []string{"BV1UNCERTAIN", "BV1FAILED001", "BV1MERGED001", "BV1REMOVED01", "BV1SETTLED01"} {
		f.bilibili(id, "synthetic "+id, "2026-03-02T00:00:00Z", 1500)
	}
	f.youtube("TARGETVIDEO", "synthetic target", "2026-02-01T00:00:00Z", "", 0)
	f.exec(`UPDATE videos SET kind='upload'`)
	if _, err := f.s.RecordBilibiliForm(ctx, "BV1UNCERTAIN", BilibiliFormUncertain, nil, formNow); err != nil {
		t.Fatal(err)
	}
	for range bilibiliFormMaxFailures {
		if err := f.s.RecordBilibiliFormFailure(ctx, "BV1FAILED001", formNow); err != nil {
			t.Fatal(err)
		}
	}
	f.exec(`UPDATE videos SET kind_review='uncertain' WHERE id IN ('BV1MERGED001','BV1REMOVED01')`)
	f.exec(`UPDATE videos SET merged_into='TARGETVIDEO',merge_source='manual' WHERE id='BV1MERGED001'`)
	f.exec(`UPDATE videos SET manual_availability='removed' WHERE id='BV1REMOVED01'`)
	if _, err := f.s.RecordBilibiliForm(ctx, "BV1SETTLED01", BilibiliFormUpload, nil, formNow); err != nil {
		t.Fatal(err)
	}
	items, err := f.s.BilibiliFormReviews(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, item := range items {
		cast := ""
		if item.Broadcast != nil {
			cast = item.Broadcast.Source + ":" + *item.Broadcast.EventID
		}
		got = append(got, item.Video.ID+"/"+item.Review+"/"+cast+"/"+boolText(item.Video.Public))
	}
	if strings.Join(got, ",") != "BV1FAILED001/failed/entry:cast/false,BV1UNCERTAIN/uncertain/entry:cast/false" {
		t.Fatalf("reviews=%v", got)
	}
	counts, err := f.s.AdminCounts(ctx, "2026-03-10")
	if err != nil || counts.BilibiliForms != 2 {
		t.Fatalf("counts=%+v err=%v", counts, err)
	}
	// The operator's answer settles the form and the upload becomes public.
	if _, err = f.s.SetVideoOverrides(ctx, "BV1UNCERTAIN", VideoOverrides{SetKind: true, Kind: new("stream")}, formNow); err != nil {
		t.Fatal(err)
	}
	video, err := f.s.AdminVideoByID(ctx, "BV1UNCERTAIN")
	if err != nil || !video.Public || video.FormSource == nil || *video.FormSource != FormSourceManual || video.FormReview != nil {
		t.Fatalf("video=%+v err=%v", video, err)
	}
	// A Bilibili upload has no premieres.
	if _, err = f.s.SetVideoOverrides(ctx, "BV1FAILED001", VideoOverrides{SetKind: true, Kind: new("premiere")}, formNow); err != ErrVideoInvalid {
		t.Fatalf("premiere err=%v", err)
	}
	if counts, err = f.s.AdminCounts(ctx, "2026-03-10"); err != nil || counts.BilibiliForms != 1 {
		t.Fatalf("after counts=%+v err=%v", counts, err)
	}
}

// The admin list narrows any task to one platform, and with Bilibili alone
// it lists merged uploads as rows of their own.
func TestAdminVideoSourceFilter(t *testing.T) {
	f := newVideoMergeFixture(t)
	ctx := context.Background()
	f.youtube("YOUTUBEVID1", "synthetic", "2026-03-01T00:00:00Z", "", 0)
	f.bilibili("BV1MERGED001", "synthetic merged", "2026-03-01T00:00:00Z", 0)
	f.bilibili("BV1ONLYUPLD1", "synthetic only", "2026-03-02T00:00:00Z", 0)
	f.exec(`UPDATE videos SET merged_into='YOUTUBEVID1',merge_source='manual' WHERE id='BV1MERGED001'`)
	list := func(filter AdminVideoFilter) string {
		t.Helper()
		page, err := f.s.ListAdminVideos(ctx, filter)
		if err != nil {
			t.Fatal(err)
		}
		ids := []string{}
		for _, item := range page.Items {
			ids = append(ids, item.ID)
		}
		return strings.Join(ids, ",")
	}
	for filter, want := range map[AdminVideoFilter]string{
		{Merge: "visible"}:                     "BV1ONLYUPLD1,YOUTUBEVID1",
		{Merge: "visible", Source: "youtube"}:  "YOUTUBEVID1",
		{Merge: "visible", Source: "bilibili"}: "BV1ONLYUPLD1",
		{Source: "bilibili"}:                   "BV1ONLYUPLD1,BV1MERGED001",
		{Source: "bilibili", Merge: "merged"}:  "BV1MERGED001",
	} {
		if got := list(filter); got != want {
			t.Errorf("%+v = %q, want %q", filter, got, want)
		}
	}
}

func boolText(value bool) string {
	if value {
		return "true"
	}
	return "false"
}
