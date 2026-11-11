package schedule

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"
)

// The X prompt and schema are unchanged by entry platforms: X platforms
// come from code. These are the sha256 of the strings sent to providers.
func TestXExtractionPromptAndSchemaAreUnchanged(t *testing.T) {
	const (
		instructions = "849c536fd9dd074a7d0bab129df711b3aaa4991178f41537206e35abcbf4e185"
		schema       = "185cd4eb031cc486cb0ec45d825f9263e955ac5f2eee4b4a559023afcff72dba"
	)
	if got := sha256Hex(scheduleInstructions); got != instructions {
		t.Errorf("X instructions changed: %s", got)
	}
	if got := sha256Hex(string(scheduleSchema)); got != schema {
		t.Errorf("X schema changed: %s", got)
	}
}

// The Bilibili schema lists platforms as an array in place of the single
// platform, and is still valid JSON.
func TestBilibiliSchemaAsksForPlatforms(t *testing.T) {
	var parsed map[string]any
	if err := json.Unmarshal([]byte(bilibiliScheduleSchema), &parsed); err != nil {
		t.Fatal(err)
	}
	items := parsed["properties"].(map[string]any)["events"].(map[string]any)["items"].(map[string]any)
	properties := items["properties"].(map[string]any)
	required := items["required"].([]any)
	if _, single := properties["platform"]; single || properties["platforms"].(map[string]any)["type"] != "array" ||
		!slices.Contains(required, any("platforms")) || slices.Contains(required, any("platform")) || !slices.Contains(required, any("timeZone")) {
		t.Fatalf("schema=%s", bilibiliScheduleSchema)
	}
	if !strings.Contains(bilibiliScheduleInstructions, bilibiliSchedulePlatformInstruction) || strings.Contains(bilibiliScheduleInstructions, schedulePlatformInstruction) {
		t.Fatal("Bilibili prompt does not ask for platforms")
	}
}

func platformsResult(entries ...any) *object {
	return obj("classification", "schedule", "events", entries, "confidence", 0.9, "evidence", "e", "reason", nil, "action", "add")
}

// A Bilibili entry keeps the platforms the model read; an invalid value is
// dropped; a stream naming none is on Bilibili by the source default.
func TestBilibiliEntriesListTheirPlatforms(t *testing.T) {
	published, _ := time.Parse(time.RFC3339, "2026-11-07T04:04:48Z")
	entry := func(eventType string, url any, platforms ...any) *object {
		e := bilibiliEntry("杂谈", "2026-11-07", "20:00", eventType, url, nil)
		e.set("platforms", platforms)
		return e
	}
	opus := "https://www.bilibili.com/opus/1"
	got, err := bilibiliProfile.normalizeResult(platformsResult(
		entry("stream", nil),
		entry("stream", nil, "bilibili", "youtube"),
		entry("stream", nil, "made-up", 7),
		entry("event", nil),
		entry("stream", nil, "bilibili"),
		entry("stream", "https://live.bilibili.com/1000001", "youtube"),
		entry("stream", "https://www.youtube.com/watch?v=AAAAAAAAAAA"),
	), "bilibili-1", opus, "今晚8点直播", published, published)
	if err != nil || len(got.events) != 7 {
		t.Fatalf("got=%#v err=%v", got, err)
	}
	want := []struct {
		platforms []string
		inferred  bool
	}{
		{[]string{"bilibili"}, true},
		{[]string{"bilibili", "youtube"}, false},
		{[]string{"bilibili"}, true},
		{[]string{}, false},
		{[]string{"bilibili"}, true},
		{[]string{"youtube", "bilibili"}, false},
		{[]string{"youtube"}, false},
	}
	for i, w := range want {
		e := got.events[i]
		if !slices.Equal(e.platforms, w.platforms) || e.platformsInferred != w.inferred || e.platform != firstOr(w.platforms, "unknown") {
			t.Errorf("entry %d: platforms=%v inferred=%v platform=%s, want %v %v", i, e.platforms, e.platformsInferred, e.platform, w.platforms, w.inferred)
		}
	}
	// The room link never stays as the entry's link.
	if got.events[5].url != opus {
		t.Fatalf("room link kept: %s", got.events[5].url)
	}
}

func firstOr(values []string, fallback string) string {
	if len(values) == 0 {
		return fallback
	}
	return values[0]
}

// X platforms come from code: simulcast wording names both platforms,
// Bilibili-only wording Bilibili, and everything else stays as before.
func TestXEntriesNameASimulcastFromTheWording(t *testing.T) {
	published, _ := time.Parse(time.RFC3339, "2026-11-07T04:04:48Z")
	xPost := "https://x.com/synthetic_handle/status/1"
	stream := func(title string) *object { return bilibiliEntry(title, "2026-11-07", "20:00", "stream", nil, nil) }
	for text, want := range map[string][]string{
		"今夜20時からYouTubeとbilibiliで同時配信！": {"youtube", "bilibili"},
		"今夜20時から同時配信":                   {"youtube", "bilibili"},
		"tonight's simulcast at 20:00":  {"youtube", "bilibili"},
		"今夜20時からbilibili限定配信":           {"bilibili"},
		"今夜20時から配信":                     {},
		"今夜20時からYouTubeで配信":             {},
	} {
		got, err := xProfile.normalizeResult(platformsResult(stream("配信")), "1", xPost, text, published, published)
		if err != nil || !slices.Equal(got.events[0].platforms, want) || got.events[0].platformsInferred {
			t.Errorf("%q => %v err=%v, want %v", text, got.events[0].platforms, err, want)
		}
	}
	// A post of several entries names a simulcast only for an entry whose
	// own wording does.
	got, err := xProfile.normalizeResult(platformsResult(stream("YouTube・bilibili同時配信"), stream("歌枠")), "1", xPost,
		"今週の予定 YouTubeとbilibiliで同時配信あり", published, published)
	if err != nil || !slices.Equal(got.events[0].platforms, []string{"youtube", "bilibili"}) || len(got.events[1].platforms) != 0 {
		t.Fatalf("schedule=%v / %v err=%v", got.events[0].platforms, got.events[1].platforms, err)
	}
	// The model's single platform and a YouTube link stay as before.
	linked := bilibiliEntry("配信", "2026-11-07", "20:00", "stream", "https://www.youtube.com/watch?v=AAAAAAAAAAA", nil)
	modelled := stream("配信")
	modelled.set("platform", "youtube")
	got, err = xProfile.normalizeResult(platformsResult(linked, modelled), "1", xPost, "", published, published)
	if err != nil || !slices.Equal(got.events[0].platforms, []string{"youtube"}) || !slices.Equal(got.events[1].platforms, []string{"youtube"}) {
		t.Fatalf("youtube=%v / %v err=%v", got.events[0].platforms, got.events[1].platforms, err)
	}
}
