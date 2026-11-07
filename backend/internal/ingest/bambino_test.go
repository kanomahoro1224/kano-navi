package ingest

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func baseEvent() eventRequest {
	return eventRequest{
		ID:          "1850000000000000000",
		Platform:    "twitter",
		Kind:        "post",
		PublishedAt: "2026-10-31T11:59:03Z",
		URL:         "https://twitter.com/kano_2525/status/1850000000000000000",
		Author: authorRequest{
			Handle:      "kano_2525",
			DisplayName: "鹿乃",
		},
		Text: "今日も配信します！",
		Media: []mediaRequest{
			{Type: "photo", URL: "https://pbs.twimg.com/media/A.jpg"},
		},
		Metrics: metricRequest{Likes: 12},
	}
}

func TestNormalizeQuote(t *testing.T) {
	event := baseEvent()
	event.Kind = "quote"
	event.Quote = &nestedRequest{
		ID:          "1849999999999999999",
		URL:         "https://x.com/other/status/1849999999999999999",
		Author:      authorRequest{Handle: "other", DisplayName: "Other"},
		PublishedAt: "2026-10-30T09:00:00Z",
		Text:        "引用正文",
		Media: []mediaRequest{
			{Type: "photo", URL: "https://pbs.twimg.com/media/Q.jpg"},
		},
	}

	post, hash, reject := normalize(event, time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC))
	if reject != nil {
		t.Fatalf("reject = %+v", reject)
	}
	if post.ID != event.ID {
		t.Fatalf("id = %q", post.ID)
	}
	if post.URL != strings.ReplaceAll(event.URL, "twitter.com", "x.com") {
		t.Fatalf("url = %q", post.URL)
	}
	if post.Quote == nil || post.Quote.ID != event.Quote.ID || post.Quote.AuthorHandle != "other" {
		t.Fatalf("quote = %+v", post.Quote)
	}
	if post.Likes != 12 || len(post.MediaURLs) != 1 {
		t.Fatalf("post = %+v", post)
	}
	if hash == "" {
		t.Fatal("content hash is empty")
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(post.RawJSON), &raw); err != nil {
		t.Fatalf("raw_json = %v", err)
	}
	if raw["ingest_hash"] != hash {
		t.Fatalf("ingest_hash = %v, want %s", raw["ingest_hash"], hash)
	}
	if !strings.Contains(raw["search_text"].(string), "引用正文") {
		t.Fatalf("search_text = %q", raw["search_text"])
	}
}

func TestNormalizeVideo(t *testing.T) {
	event := eventRequest{
		ID:              "BV1CmHw6eEGE",
		Platform:        "bilibili",
		Kind:            "video",
		PublishedAt:     "2026-10-24T06:25:00Z",
		Title:           "【雑談】テスト",
		ThumbnailURL:    "http://i0.hdslb.com/bfs/archive/cover.jpg",
		DurationSeconds: 289,
	}

	video, hash, reject := normalizeVideo(event, time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC))
	if reject != nil {
		t.Fatalf("reject = %+v", reject)
	}
	if video.Source != "bilibili" || video.Kind != "upload" {
		t.Fatalf("source/kind = %q/%q", video.Source, video.Kind)
	}
	if video.URL != "https://www.bilibili.com/video/"+event.ID {
		t.Fatalf("url = %q", video.URL)
	}
	if video.ThumbnailURL != "https://i0.hdslb.com/bfs/archive/cover.jpg" {
		t.Fatalf("thumbnail = %q", video.ThumbnailURL)
	}
	if video.DurationSeconds != 289 || hash == "" {
		t.Fatalf("video = %+v hash=%q", video, hash)
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(video.RawJSON), &raw); err != nil {
		t.Fatalf("raw_json = %v", err)
	}
	if raw["ingest_hash"] != hash || raw["search_text"] != event.Title {
		t.Fatalf("raw = %v", raw)
	}
	if urls, ok := raw["media_urls"].([]any); !ok || len(urls) != 1 {
		t.Fatalf("media_urls = %v", raw["media_urls"])
	}
}

func TestNormalizeYoutubeVideo(t *testing.T) {
	event := eventRequest{
		ID:           "AAAAAAAAAAA",
		Platform:     "youtube",
		Kind:         "video",
		PublishedAt:  "2026-10-24T06:25:00Z",
		Title:        "歌枠テスト",
		ThumbnailURL: "https://i.ytimg.com/vi/AAAAAAAAAAA/hqdefault.jpg",
	}

	video, hash, reject := normalizeVideo(event, time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC))
	if reject != nil {
		t.Fatalf("reject = %+v", reject)
	}
	if video.Source != "youtube" || video.Kind != "upload" {
		t.Fatalf("source/kind = %q/%q", video.Source, video.Kind)
	}
	if video.URL != "https://www.youtube.com/watch?v="+event.ID || hash == "" {
		t.Fatalf("video = %+v", video)
	}
	if video.IsShort == nil || *video.IsShort {
		t.Fatalf("is_short = %v", video.IsShort)
	}
}

func TestNormalizeYoutubeShortsAndLive(t *testing.T) {
	event := eventRequest{
		ID:              "BBBBBBBBBBB",
		Platform:        "youtube",
		Kind:            "shorts",
		PublishedAt:     "2026-10-24T06:25:00Z",
		Title:           "short",
		ThumbnailURL:    "https://i.ytimg.com/vi/BBBBBBBBBBB/hqdefault.jpg",
		DurationSeconds: 30,
	}
	short, _, reject := normalizeVideo(event, time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC))
	if reject != nil {
		t.Fatalf("shorts reject = %+v", reject)
	}
	if short.IsShort == nil || !*short.IsShort || short.IsUpcoming || short.LiveStatus != "" {
		t.Fatalf("shorts = %+v", short)
	}

	event.ID = "CCCCCCCCCCC"
	event.Kind = "live_schedule"
	event.ScheduledAt = "2026-10-25T09:00:00Z"
	schedule, _, reject := normalizeVideo(event, time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC))
	if reject != nil {
		t.Fatalf("schedule reject = %+v", reject)
	}
	if !schedule.IsUpcoming || schedule.LiveStatus != "upcoming" || schedule.ScheduledAt != "2026-10-25T09:00:00.000Z" {
		t.Fatalf("schedule = %+v", schedule)
	}

	event.Kind = "live_end"
	ended, _, reject := normalizeVideo(event, time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC))
	if reject != nil {
		t.Fatalf("end reject = %+v", reject)
	}
	if ended.IsUpcoming || ended.LiveStatus != "ended" {
		t.Fatalf("end = %+v", ended)
	}
}

func TestNormalizeVideoRejectsBadIDAndFuture(t *testing.T) {
	event := eventRequest{
		ID:          "not-a-bv",
		Platform:    "bilibili",
		Kind:        "video",
		PublishedAt: "2026-10-24T06:25:00Z",
		Title:       "x",
	}
	if _, _, reject := normalizeVideo(event, time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)); reject == nil || reject.Code != "invalid_event" {
		t.Fatalf("id reject = %+v", reject)
	}
	event.ID = "BV1CmHw6eEGE"
	event.PublishedAt = "2026-11-02T00:00:00Z"
	if _, _, reject := normalizeVideo(event, time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)); reject == nil || reject.Code != "future_timestamp" {
		t.Fatalf("future reject = %+v", reject)
	}
	// Bilibili only reports uploads; a live kind has no videos row here.
	event.PublishedAt = "2026-10-24T06:25:00Z"
	event.Kind = "live_start"
	if _, _, reject := normalizeVideo(event, time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)); reject == nil || reject.Code != "invalid_event" {
		t.Fatalf("kind reject = %+v", reject)
	}
	// A YouTube id is never a BV id and vice versa.
	event = eventRequest{ID: "AAAAAAAAAAA", Platform: "bilibili", Kind: "video", PublishedAt: "2026-10-24T06:25:00Z", Title: "x"}
	if _, _, reject := normalizeVideo(event, time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)); reject == nil || reject.Code != "invalid_event" {
		t.Fatalf("cross-platform id reject = %+v", reject)
	}
}

func TestNormalizeRepostUsesOriginalTextAndMedia(t *testing.T) {
	event := baseEvent()
	event.Kind = "repost"
	event.Text = "RT @other: 原推"
	event.Media = nil
	event.Retweet = &nestedRequest{
		ID:          "1849999999999999999",
		URL:         "https://x.com/other/status/1849999999999999999",
		Author:      authorRequest{Handle: "other", DisplayName: "Other", AvatarURL: "https://pbs.twimg.com/profile_images/1/avatar_normal.jpg"},
		PublishedAt: "2026-10-30T09:00:00Z",
		Text:        "原推正文",
		Media: []mediaRequest{
			{Type: "photo", URL: "https://pbs.twimg.com/media/RT.jpg"},
		},
	}

	post, _, reject := normalize(event, time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC))
	if reject != nil {
		t.Fatalf("reject = %+v", reject)
	}
	if post.Text != "原推正文" {
		t.Fatalf("text = %q", post.Text)
	}
	if len(post.MediaURLs) != 1 || post.MediaURLs[0] != "https://pbs.twimg.com/media/RT.jpg" {
		t.Fatalf("media = %+v", post.MediaURLs)
	}
	if post.Quote != nil {
		t.Fatalf("repost must not create a quote: %+v", post.Quote)
	}
	if post.Repost == nil || post.Repost.ID != event.Retweet.ID || post.Repost.AuthorHandle != "other" || post.Repost.AuthorName != "Other" || post.Repost.AuthorAvatar != event.Retweet.Author.AvatarURL || post.Repost.URL != event.Retweet.URL {
		t.Fatalf("repost = %+v", post.Repost)
	}
}

func bilibiliEvent() eventRequest {
	return eventRequest{
		ID:          "1150000000000000000",
		Platform:    "bilibili",
		Kind:        "post",
		PublishedAt: "2026-10-31T11:00:00Z",
		URL:         "https://t.bilibili.com/1150000000000000000",
		Author: authorRequest{
			Handle:      "鹿乃",
			ExternalID:  "316381099",
			DisplayName: "鹿乃",
		},
		Text: "今天也元气满满",
		Media: []mediaRequest{
			{Type: "photo", URL: "https://i0.hdslb.com/bfs/new_dyn/abc.jpg"},
		},
	}
}

// A Bilibili dynamic shares the posts table with X but keeps its own source, so
// the archive can tell them apart and no X-only rewriting touches it.
func TestNormalizeBilibiliPost(t *testing.T) {
	event := bilibiliEvent()
	event.URL = ""

	post, hash, reject := normalize(event, time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC))
	if reject != nil {
		t.Fatalf("reject = %+v", reject)
	}
	if post.Source != "bilibili" {
		t.Fatalf("source = %q", post.Source)
	}
	if post.URL != "https://t.bilibili.com/"+event.ID {
		t.Fatalf("url = %q", post.URL)
	}
	if post.AccountHandle != "鹿乃" {
		t.Fatalf("account = %q", post.AccountHandle)
	}
	if len(post.MediaURLs) != 1 || hash == "" {
		t.Fatalf("post = %+v", post)
	}
}

func TestNormalizeBilibiliRepost(t *testing.T) {
	event := bilibiliEvent()
	event.Kind = "repost"
	event.Text = "转发动态"
	event.Media = nil
	event.Retweet = &nestedRequest{
		ID:          "1149999999999999999",
		URL:         "https://t.bilibili.com/1149999999999999999",
		Author:      authorRequest{Handle: "别人", DisplayName: "别人"},
		PublishedAt: "2026-10-30T09:00:00Z",
		Text:        "原动态正文",
		Media: []mediaRequest{
			{Type: "photo", URL: "https://i0.hdslb.com/bfs/new_dyn/rt.jpg"},
		},
	}

	post, _, reject := normalize(event, time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC))
	if reject != nil {
		t.Fatalf("reject = %+v", reject)
	}
	if post.Text != "原动态正文" || post.Repost == nil || post.Repost.ID != event.Retweet.ID {
		t.Fatalf("repost = %+v text = %q", post.Repost, post.Text)
	}
	if post.Repost.URL != event.Retweet.URL {
		t.Fatalf("repost url = %q", post.Repost.URL)
	}
	if len(post.MediaURLs) != 1 || !strings.Contains(post.MediaURLs[0], "rt.jpg") {
		t.Fatalf("media = %+v", post.MediaURLs)
	}
}

// A BV id belongs to the videos table; letting it through here would put a
// video in the feed.
func TestNormalizeBilibiliRejectsNonDynamicID(t *testing.T) {
	event := bilibiliEvent()
	event.ID = "BV1xx411c7mD"
	if _, _, reject := normalize(event, time.Now()); reject == nil || reject.Code != "invalid_event" {
		t.Fatalf("reject = %+v", reject)
	}
}

func TestNormalizeRejectsFutureAndUnknownPlatform(t *testing.T) {
	event := baseEvent()
	event.PublishedAt = "2026-11-02T00:00:00Z"
	if _, _, reject := normalize(event, time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)); reject == nil || reject.Code != "future_timestamp" {
		t.Fatalf("future reject = %+v", reject)
	}
	event = baseEvent()
	event.Platform = "twitch"
	if _, _, reject := normalize(event, time.Now()); reject == nil || reject.Code != "unsupported_platform" {
		t.Fatalf("platform reject = %+v", reject)
	}
}

func TestVerifySignature(t *testing.T) {
	module := &Module{
		secret:    []byte("shared-secret"),
		tolerance: 5 * time.Minute,
		now:       func() time.Time { return time.Unix(1_790_000_000, 0) },
	}
	body := []byte(`{"schema":1}`)
	request := httptest.NewRequest(http.MethodPost, "/internal/ingest/bambino", nil)
	request.Header.Set("X-Bambino-Timestamp", "1790000000")
	request.Header.Set("X-Bambino-Signature", "sha256="+signatureFor("shared-secret", "1790000000", body))
	if err := module.verify(request, body); err != nil {
		t.Fatalf("valid signature rejected: %v", err)
	}

	request.Header.Set("X-Bambino-Signature", "sha256=deadbeef")
	if err := module.verify(request, body); err == nil {
		t.Fatal("invalid signature accepted")
	}
}

func signatureFor(secret, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp))
	mac.Write([]byte("\n"))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func TestServeHTTPRejectsBadSignatureBeforeStorage(t *testing.T) {
	module := &Module{
		secret:    []byte("shared-secret"),
		tolerance: 5 * time.Minute,
		now:       func() time.Time { return time.Unix(1_790_000_000, 0) },
	}
	request := httptest.NewRequest(http.MethodPost, "/internal/ingest/bambino", strings.NewReader(`{"schema":1,"events":[]}`))
	request.Header.Set("X-Bambino-Timestamp", "1790000000")
	request.Header.Set("X-Bambino-Signature", "sha256=deadbeef")
	recorder := httptest.NewRecorder()
	module.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", recorder.Code)
	}
}
