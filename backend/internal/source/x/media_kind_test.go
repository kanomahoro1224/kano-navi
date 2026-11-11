package x

import (
	"encoding/json"
	"reflect"
	"testing"
)

func decodedJSON(t *testing.T, body string) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatal(err)
	}
	return payload
}

// Each path records what an image shows beside its URL: the detail answer's
// media_extended, the timeline's media and the X API's media type, for the
// post's own images and a quoted status's alike.
func TestMediaKindsFollowEverySourcePath(t *testing.T) {
	want := []string{MediaPhoto, MediaVideo, MediaGIF}
	wantURLs := []string{"https://pbs.twimg.com/media/still.jpg", "https://pbs.twimg.com/ext_tw_video_thumb/1/clip.jpg", "https://pbs.twimg.com/tweet_video_thumb/loop.jpg"}

	detail := decodedJSON(t, `{"tweetID":"1900000000000000800","text":"media","date_epoch":1792540800,"user_screen_name":"kano",
		"mediaURLs":["https://pbs.twimg.com/media/still.jpg","https://video.twimg.com/clip.mp4","https://video.twimg.com/loop.mp4"],
		"media_extended":[{"type":"image","url":"https://pbs.twimg.com/media/still.jpg"},
			{"type":"video","url":"https://video.twimg.com/clip.mp4","thumbnail_url":"https://pbs.twimg.com/ext_tw_video_thumb/1/clip.jpg"},
			{"type":"gif","url":"https://video.twimg.com/loop.mp4","thumbnail_url":"https://pbs.twimg.com/tweet_video_thumb/loop.jpg"}],
		"qrt":{"tweetID":"1900000000000000801","text":"quoted","date_epoch":1792537200,"user_screen_name":"other",
			"media_extended":[{"type":"video","url":"https://video.twimg.com/q.mp4","thumbnail_url":"https://pbs.twimg.com/ext_tw_video_thumb/2/q.jpg"}]}}`)
	post := Normalize(detail, "1900000000000000800", "kano")
	if post == nil || !reflect.DeepEqual(post.MediaURLs, wantURLs) || !reflect.DeepEqual(post.MediaKinds, want) {
		t.Fatalf("detail post=%+v", post)
	}
	if q := post.Quote; q == nil || !reflect.DeepEqual(q.MediaKinds, []string{MediaVideo}) || q.MediaURLs[0] != "https://pbs.twimg.com/ext_tw_video_thumb/2/q.jpg" {
		t.Fatalf("detail quote=%+v", post.Quote)
	}
	// The stored payload reads the same, and an untyped list reads as photos.
	if urls, kinds := OwnMedia(detail); !reflect.DeepEqual(urls, wantURLs) || !reflect.DeepEqual(kinds, want) {
		t.Fatalf("stored urls=%v kinds=%v", urls, kinds)
	}
	if _, kinds := OwnMedia(map[string]any{"mediaURLs": []any{"https://pbs.twimg.com/media/a.jpg"}}); !reflect.DeepEqual(kinds, []string{MediaPhoto}) {
		t.Fatalf("untyped kinds=%v", kinds)
	}

	page, err := parseTimelineStatuses([]byte(`{"code":200,"results":[{"id":"1900000000000000802","text":"media","created_timestamp":1792540800,
		"author":{"screen_name":"kano","name":"Kano"},
		"media":{"all":[{"type":"photo","url":"https://pbs.twimg.com/media/still.jpg?name=orig"},
			{"type":"video","url":"https://video.twimg.com/clip.mp4","thumbnail_url":"https://pbs.twimg.com/ext_tw_video_thumb/1/clip.jpg"},
			{"type":"gif","url":"https://video.twimg.com/loop.mp4","thumbnail_url":"https://pbs.twimg.com/tweet_video_thumb/loop.jpg"}]}}],
		"cursor":{"bottom":""}}`), "kano")
	if err != nil || len(page.Statuses) != 1 || page.Statuses[0].Post == nil {
		t.Fatalf("timeline page=%+v err=%v", page, err)
	}
	timeline := page.Statuses[0].Post
	if !reflect.DeepEqual(timeline.MediaURLs, wantURLs) || !reflect.DeepEqual(timeline.MediaKinds, want) {
		t.Fatalf("timeline urls=%v kinds=%v", timeline.MediaURLs, timeline.MediaKinds)
	}
	// Read again from its stored raw_json, the timeline post keeps its kinds.
	if again := Normalize(timeline.Raw, timeline.ID, "kano"); !reflect.DeepEqual(again.MediaKinds, want) {
		t.Fatalf("stored timeline kinds=%v", again.MediaKinds)
	}

	answer := decodedJSON(t, `{"data":[{"id":"1900000000000000803","author_id":"1","created_at":"2026-10-20T00:00:00.000Z","text":"media",
		"attachments":{"media_keys":["m1","m2","m3"]}}],
		"includes":{"users":[{"id":"1","username":"kano","name":"Kano"}],
			"media":[{"media_key":"m1","type":"photo","url":"https://pbs.twimg.com/media/still.jpg"},
				{"media_key":"m2","type":"video","preview_image_url":"https://pbs.twimg.com/ext_tw_video_thumb/1/clip.jpg"},
				{"media_key":"m3","type":"animated_gif","preview_image_url":"https://pbs.twimg.com/tweet_video_thumb/loop.jpg"}]},
		"meta":{"result_count":1}}`)
	posts, _, _, err := APIPosts(answer, "kano")
	if err != nil || len(posts) != 1 || !reflect.DeepEqual(posts[0].MediaURLs, wantURLs) || !reflect.DeepEqual(posts[0].MediaKinds, want) {
		t.Fatalf("api posts=%+v err=%v", posts, err)
	}
	if kinds := posts[0].Raw["mediaKinds"]; !reflect.DeepEqual(kinds, []any{MediaPhoto, MediaVideo, MediaGIF}) {
		t.Fatalf("api raw kinds=%v", kinds)
	}
}
