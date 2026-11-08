package youtube

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestParseVideoIDAcceptsIDsAndVideoLinks(t *testing.T) {
	for input, want := range map[string]string{
		"AAAAAAAAAAA": "AAAAAAAAAAA",
		" https://www.youtube.com/watch?v=AAAAAAAAAAA&t=9": "AAAAAAAAAAA",
		"youtube.com/watch?v=AAAAAAAAAAA":                  "AAAAAAAAAAA",
		"https://m.youtube.com/watch?v=A-_AAAAAAAA":        "A-_AAAAAAAA",
		"https://youtu.be/AAAAAAAAAAA?si=x":                "AAAAAAAAAAA",
		"https://www.youtube.com/shorts/AAAAAAAAAAA":       "AAAAAAAAAAA",
		"https://www.youtube.com/live/AAAAAAAAAAA":         "AAAAAAAAAAA",
		"https://www.youtube.com/embed/AAAAAAAAAAA":        "AAAAAAAAAAA",
	} {
		if got, ok := ParseVideoID(input); !ok || got != want {
			t.Errorf("ParseVideoID(%q)=%q,%v", input, got, ok)
		}
	}
	for _, input := range []string{
		"", "AAAAAAAAAA", "https://example.com/watch?v=AAAAAAAAAAA", "https://www.youtube.com/channel/UCAAAAAAAAAAAAAAAAAAAAAA",
		"https://www.youtube.com/watch?v=AAAAAAAAAA", "https://www.youtube.com.invalid/watch?v=AAAAAAAAAAA",
		"https://www.youtube.com:8443/watch?v=AAAAAAAAAAA", "ftp://www.youtube.com/watch?v=AAAAAAAAAAA",
	} {
		if got, ok := ParseVideoID(input); ok {
			t.Errorf("ParseVideoID(%q) accepted %q", input, got)
		}
	}
}

func TestReadVideoAcceptsOnlyTheChannelsVideos(t *testing.T) {
	watched, called := []string{}, 0
	c := NewClient(&http.Client{Transport: fakeTransport(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/youtube/v3/videos":
			called++
			return response(200, apiItems(`{"id":"AAAAAAAAAAA","snippet":{"title":"ordinary upload","channelId":"`+syntheticChannel+`","publishedAt":"2026-01-02T00:00:00Z","liveBroadcastContent":"none"},"contentDetails":{"duration":"PT1M"},"status":{"privacyStatus":"public"}}`)), nil
		case "/watch":
			id := r.URL.Query().Get("v")
			watched = append(watched, id)
			channel := syntheticChannel
			if id == "BBBBBBBBBBB" {
				channel = "UCBBBBBBBBBBBBBBBBBBBBBB"
			}
			return response(200, `var ytInitialPlayerResponse={"videoDetails":{"videoId":"`+id+`","channelId":"`+channel+`","title":"watched","lengthSeconds":"60"},"playabilityStatus":{"status":"OK"},"microformat":{"playerMicroformatRenderer":{"publishDate":"2026-01-02T00:00:00+00:00"}}};`), nil
		}
		t.Fatalf("unexpected request %s", r.URL)
		return nil, nil
	})})
	a := NewAdapter(c, Options{ChannelID: syntheticChannel, MaxDetailRequests: 12, APIKey: "synthetic-api-key"})
	a.Now = func() time.Time { return time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC) }
	// With a key the Data API describes it, without a watch page.
	v, _, err := a.ReadVideo(context.Background(), "AAAAAAAAAAA", map[string]Video{})
	if err != nil || v.Title != "ordinary upload" || v.Kind != KindUpload || called != 1 || len(watched) != 0 {
		t.Fatalf("api video=%#v err=%v called=%d watched=%v", v, err, called, watched)
	}
	// Without one its watch page does, and another channel's video is refused.
	a.Options.APIKey = ""
	if v, _, err = a.ReadVideo(context.Background(), "CCCCCCCCCCC", nil); err != nil || v.Title != "watched" || v.DurationSeconds != 60 {
		t.Fatalf("watch video=%#v err=%v", v, err)
	}
	if _, _, err = a.ReadVideo(context.Background(), "BBBBBBBBBBB", nil); err == nil || err.Error() != CodeOtherChannel {
		t.Fatalf("other channel err=%v", err)
	}
	if called != 1 || len(watched) != 2 {
		t.Fatalf("called=%d watched=%v", called, watched)
	}
}
