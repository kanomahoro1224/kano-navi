package x

import (
	"errors"
	"strings"
	"testing"
)

// syntheticExport is an account export in the exporter's shape, with X's
// status record in metadata. Every handle, ID and address is synthetic.
const syntheticExport = `[
  {
    "id": "1900000000000000901", "screen_name": "kano_2525", "name": "Kano",
    "in_reply_to": null, "retweeted_status": null, "quoted_status": "1900000000000000999",
    "favorite_count": 12, "retweet_count": 3, "reply_count": 2, "views_count": 400,
    "metadata": {
      "__typename": "Tweet", "rest_id": "1900000000000000901",
      "legacy": {
        "created_at": "Wed Nov 18 09:18:01 +0000 2026",
        "full_text": "Tonight &amp; tomorrow\nhttps://t.co/link1 https://t.co/quote1 https://t.co/photo1",
        "entities": {"urls": [
          {"url": "https://t.co/link1", "expanded_url": "https://example.com/watch"},
          {"url": "https://t.co/quote1", "expanded_url": "https://x.com/someone_synthetic/status/1900000000000000999"}
        ]},
        "extended_entities": {"media": [
          {"url": "https://t.co/photo1", "media_url_https": "https://pbs.twimg.com/media/SyntheticA.jpg", "type": "photo"},
          {"url": "https://t.co/photo1", "media_url_https": "https://pbs.twimg.com/ext_tw_video_thumb/1/pu/img/SyntheticB.jpg", "type": "video"}
        ]}
      },
      "quoted_status_result": {"result": {
        "__typename": "Tweet", "rest_id": "1900000000000000999",
        "core": {"user_results": {"result": {"core": {"name": "Someone", "screen_name": "someone_synthetic"}}}},
        "legacy": {"created_at": "Tue Nov 17 01:00:00 +0000 2026", "full_text": "quoted words", "entities": {}}
      }}
    }
  },
  {
    "id": "1900000000000000902", "screen_name": "kano_2525", "name": "Kano",
    "in_reply_to": {"id": "1900000000000000001"}, "retweeted_status": null,
    "metadata": {"__typename": "Tweet", "rest_id": "1900000000000000902", "legacy": {"created_at": "Wed Nov 18 09:18:01 +0000 2026", "full_text": "a reply"}}
  },
  {
    "id": "1900000000000000903", "screen_name": "kano_2525", "name": "Kano",
    "in_reply_to": null, "retweeted_status": {"id": "1900000000000000002"},
    "metadata": {"__typename": "Tweet", "rest_id": "1900000000000000903", "legacy": {"created_at": "Wed Nov 18 09:18:01 +0000 2026", "full_text": "RT"}}
  },
  {
    "id": "1900000000000000904", "screen_name": "other_synthetic", "name": "Other",
    "metadata": {"__typename": "Tweet", "rest_id": "1900000000000000904", "legacy": {"created_at": "Wed Nov 18 09:18:01 +0000 2026", "full_text": "not ours"}}
  },
  {
    "id": "1900000000000000905", "screen_name": "kano_2525", "name": "Kano",
    "metadata": {"__typename": "TweetWithVisibilityResults", "tweet": {
      "rest_id": "1900000000000000905",
      "legacy": {"created_at": "Mon Nov 16 00:00:00 +0000 2026", "full_text": "short…", "entities": {}},
      "note_tweet": {"note_tweet_results": {"result": {"text": "a long post https://t.co/long1", "entity_set": {"urls": [{"url": "https://t.co/long1", "expanded_url": "https://example.com/long"}]}}}}
    }}
  },
  {"id": "1900000000000000906", "screen_name": "kano_2525", "metadata": {"rest_id": "1900000000000000906", "legacy": {}}}
]`

// An export yields the account's own top-level posts in a detail answer's
// form: expanded links, no media or quote links, the post's images with
// their kinds, its counts and the status it quotes.
func TestParseArchiveReadsOwnTopLevelPosts(t *testing.T) {
	posts, stats, err := ParseArchive(strings.NewReader(syntheticExport), "kano_2525", "2026-11-18T00:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	if stats != (ArchiveStats{Records: 6, Posts: 2, Replies: 1, Reposts: 1, OtherAccount: 1, Invalid: 1}) {
		t.Fatalf("stats=%+v", stats)
	}
	post := posts[0]
	if post.ID != "1900000000000000901" || post.Text != "Tonight & tomorrow\nhttps://example.com/watch" {
		t.Fatalf("post=%q %q", post.ID, post.Text)
	}
	if post.PublishedAt != "2026-11-18T09:18:01Z" || post.URL != "https://x.com/kano_2525/status/1900000000000000901" {
		t.Fatalf("time=%s url=%s", post.PublishedAt, post.URL)
	}
	if post.Likes != 12 || post.Reposts != 3 || post.Replies != 2 {
		t.Fatalf("counts=%d %d %d", post.Likes, post.Reposts, post.Replies)
	}
	if strings.Join(post.MediaURLs, " ") != "https://pbs.twimg.com/media/SyntheticA.jpg https://pbs.twimg.com/ext_tw_video_thumb/1/pu/img/SyntheticB.jpg" ||
		strings.Join(post.MediaKinds, " ") != MediaPhoto+" "+MediaVideo {
		t.Fatalf("media=%v kinds=%v", post.MediaURLs, post.MediaKinds)
	}
	quote := post.Quote
	if quote == nil || quote.ID != "1900000000000000999" || quote.AuthorHandle != "someone_synthetic" || quote.Text != "quoted words" ||
		quote.URL != "https://x.com/someone_synthetic/status/1900000000000000999" || quote.Unavailable {
		t.Fatalf("quote=%+v", quote)
	}
	if origin := post.Raw["archive"].(map[string]any)["origin"]; origin != ArchiveOrigin {
		t.Fatalf("raw archive=%v", post.Raw["archive"])
	}
	if long := posts[1]; long.Text != "a long post https://example.com/long" || long.Quote != nil {
		t.Fatalf("long post=%q quote=%v", long.Text, long.Quote)
	}
}

func TestParseArchiveRejectsWhatIsNotAnExport(t *testing.T) {
	for _, body := range []string{`{"id":"1"}`, `not json`, ``} {
		if _, _, err := ParseArchive(strings.NewReader(body), "kano_2525", ""); !errors.Is(err, ErrArchiveInvalid) {
			t.Fatalf("%q: err=%v", body, err)
		}
	}
}

func TestArchiveMediaFileNamesAPhotoAndItsPosition(t *testing.T) {
	for name, want := range map[string]struct {
		id       string
		position int
		ok       bool
	}{
		"kano_2525_1900000000000000901_photo_2_20261118.jpg": {"1900000000000000901", 1, true},
		"MKLNtic_1900000000000000901_photo_1_20220902.png":   {"1900000000000000901", 0, true},
		"kano_2525_1900000000000000901_video_1_20261118.mp4": {},
		"kano_2525_1900000000000000901_photo_5_20261118.jpg": {},
		"../kano_2525_1900000000000000901_photo_1_20261118":  {},
	} {
		id, position, ok := ArchiveMediaFile(name)
		if id != want.id || position != want.position || ok != want.ok {
			t.Fatalf("%s: %s %d %v", name, id, position, ok)
		}
	}
}
