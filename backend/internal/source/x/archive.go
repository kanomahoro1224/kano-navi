package x

import (
	"encoding/json"
	"errors"
	"html"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// An account export is the JSON array a browser exporter writes for a
// profile's posts: one record per status with the exporter's own fields
// (screen_name, in_reply_to, retweeted_status, counts) and X's status record
// in metadata, whose legacy fields carry the wording, links and media.

// MaxArchiveBytes bounds one account export file.
const MaxArchiveBytes = 256 << 20

// ArchiveOrigin marks an imported export in a post's raw_json.
const ArchiveOrigin = "x-export"

// ErrArchiveInvalid reports an export that is not a list of statuses.
var ErrArchiveInvalid = errors.New("x_archive_invalid")

type archiveRecord struct {
	ID              string          `json:"id"`
	ScreenName      string          `json:"screen_name"`
	Name            string          `json:"name"`
	InReplyTo       json.RawMessage `json:"in_reply_to"`
	RetweetedStatus json.RawMessage `json:"retweeted_status"`
	FavoriteCount   int             `json:"favorite_count"`
	RetweetCount    int             `json:"retweet_count"`
	ReplyCount      int             `json:"reply_count"`
	ViewsCount      *int            `json:"views_count"`
	Metadata        graphTweet      `json:"metadata"`
}

// graphTweet is X's status record: the export's metadata and the quoted
// status inside it share this shape.
type graphTweet struct {
	TypeName string `json:"__typename"`
	RestID   string `json:"rest_id"`
	Core     struct {
		UserResults struct {
			Result struct {
				Avatar struct {
					ImageURL string `json:"image_url"`
				} `json:"avatar"`
				Core struct {
					Name       string `json:"name"`
					ScreenName string `json:"screen_name"`
				} `json:"core"`
			} `json:"result"`
		} `json:"user_results"`
	} `json:"core"`
	Legacy struct {
		CreatedAt        string          `json:"created_at"`
		FullText         string          `json:"full_text"`
		Entities         archiveEntities `json:"entities"`
		ExtendedEntities archiveEntities `json:"extended_entities"`
	} `json:"legacy"`
	NoteTweet struct {
		NoteTweetResults struct {
			Result struct {
				Text      string          `json:"text"`
				EntitySet archiveEntities `json:"entity_set"`
			} `json:"result"`
		} `json:"note_tweet_results"`
	} `json:"note_tweet"`
	QuotedStatusResult struct {
		Result json.RawMessage `json:"result"`
	} `json:"quoted_status_result"`
	// Tweet is the status a TweetWithVisibilityResults wraps.
	Tweet *graphTweet `json:"tweet"`
}

type archiveEntities struct {
	URLs []struct {
		URL      string `json:"url"`
		Expanded string `json:"expanded_url"`
	} `json:"urls"`
	Media []struct {
		URL      string `json:"url"`
		MediaURL string `json:"media_url_https"`
		Type     string `json:"type"`
	} `json:"media"`
}

// ArchiveStats counts an export's records by what they are.
type ArchiveStats struct {
	Records, Posts, Replies, Reposts, OtherAccount, Invalid int
}

// ParseArchive reads one account's export and returns its own top-level
// posts, normalized like a detail answer: no replies, no reposts and no
// other account's statuses. capturedAt names the export in raw_json.
func ParseArchive(r io.Reader, account, capturedAt string) ([]Post, ArchiveStats, error) {
	body, err := io.ReadAll(io.LimitReader(r, MaxArchiveBytes+1))
	if err != nil || len(body) > MaxArchiveBytes {
		return nil, ArchiveStats{}, ErrArchiveInvalid
	}
	var records []archiveRecord
	if err = json.Unmarshal(body, &records); err != nil {
		return nil, ArchiveStats{}, ErrArchiveInvalid
	}
	stats := ArchiveStats{Records: len(records)}
	posts := make([]Post, 0, len(records))
	for _, record := range records {
		switch {
		case !strings.EqualFold(record.ScreenName, account):
			stats.OtherAccount++
			continue
		case present(record.InReplyTo):
			stats.Replies++
			continue
		case present(record.RetweetedStatus):
			stats.Reposts++
			continue
		}
		post := archivePost(record, account, capturedAt)
		if post == nil {
			stats.Invalid++
			continue
		}
		posts = append(posts, *post)
		stats.Posts++
	}
	return posts, stats, nil
}

func present(raw json.RawMessage) bool {
	value := strings.TrimSpace(string(raw))
	return value != "" && value != "null"
}

func archivePost(record archiveRecord, account, capturedAt string) *Post {
	tweet := record.Metadata.status()
	if tweet == nil || record.ID == "" || (tweet.RestID != "" && tweet.RestID != record.ID) {
		return nil
	}
	quoted := tweet.quoted()
	quotedID := ""
	if quoted != nil {
		quotedID = quoted.RestID
	}
	payload := tweet.payload(record.ID, account, record.Name, quotedID)
	if payload == nil {
		return nil
	}
	payload["likes"] = record.FavoriteCount
	payload["retweets"] = record.RetweetCount
	payload["replies"] = record.ReplyCount
	if record.ViewsCount != nil {
		payload["views"] = *record.ViewsCount
	}
	if quoted != nil {
		if qrt := quoted.payload(quoted.RestID, quoted.handle(), quoted.name(), ""); qrt != nil {
			if avatar := quoted.Core.UserResults.Result.Avatar.ImageURL; avatar != "" {
				qrt["user_profile_image_url"] = avatar
			}
			payload["qrt"] = qrt
		}
	}
	payload["archive"] = map[string]any{"origin": ArchiveOrigin, "capturedAt": capturedAt}
	if !statusIDPattern.MatchString(record.ID) {
		return nil
	}
	post := Normalize(payload, record.ID, account)
	if post != nil {
		// ParseArchive keeps only the account's own top-level posts.
		post.Kind = StatusTopLevel
	}
	return post
}

// status unwraps a status record shown with visibility results.
func (t *graphTweet) status() *graphTweet {
	if t == nil {
		return nil
	}
	if t.TypeName == "TweetWithVisibilityResults" {
		return t.Tweet.status()
	}
	if t.Legacy.CreatedAt == "" {
		return nil
	}
	return t
}

func (t *graphTweet) quoted() *graphTweet {
	if !present(t.QuotedStatusResult.Result) {
		return nil
	}
	var quoted graphTweet
	if json.Unmarshal(t.QuotedStatusResult.Result, &quoted) != nil {
		return nil
	}
	status := quoted.status()
	if status == nil || !statusIDPattern.MatchString(status.RestID) || status.handle() == "" {
		return nil
	}
	return status
}

func (t *graphTweet) handle() string {
	handle := t.Core.UserResults.Result.Core.ScreenName
	if !handlePattern.MatchString(handle) {
		return ""
	}
	return handle
}

func (t *graphTweet) name() string { return t.Core.UserResults.Result.Core.Name }

// payload is the status in vxtwitter's detail shape, which Normalize and
// QuoteOf read: its wording with t.co links expanded and media links removed,
// as is the link to the status it quotes (quotedID), which shows as its card;
// its time; and its images (a video's or GIF's thumbnail) with their kinds.
func (t *graphTweet) payload(id, handle, name, quotedID string) map[string]any {
	published, err := time.Parse(time.RubyDate, t.Legacy.CreatedAt)
	if err != nil || id == "" {
		return nil
	}
	entities := t.Legacy.Entities
	text := t.Legacy.FullText
	if note := t.NoteTweet.NoteTweetResults.Result; strings.TrimSpace(note.Text) != "" {
		text = note.Text
		entities.URLs = append(note.EntitySet.URLs, entities.URLs...)
	}
	media := t.Legacy.ExtendedEntities.Media
	if len(media) == 0 {
		media = entities.Media
	}
	urls, kinds := []any{}, []any{}
	for _, item := range media {
		text = strings.ReplaceAll(text, item.URL, "")
		if item.MediaURL == "" {
			continue
		}
		urls = append(urls, item.MediaURL)
		kinds = append(kinds, mediaKind(item.Type))
	}
	for _, link := range entities.URLs {
		switch {
		case link.URL == "" || link.Expanded == "":
		case quotedID != "" && statusIDIn(link.Expanded) == quotedID:
			text = strings.ReplaceAll(text, link.URL, "")
		default:
			text = strings.ReplaceAll(text, link.URL, link.Expanded)
		}
	}
	text = strings.TrimSpace(trailingSpace.ReplaceAllString(html.UnescapeString(text), "\n"))
	return map[string]any{
		"tweetID":          id,
		"text":             text,
		"date_epoch":       float64(published.Unix()),
		"user_screen_name": handle,
		"user_name":        name,
		"tweetURL":         "https://x.com/" + handle + "/status/" + id,
		"mediaURLs":        urls,
		"mediaKinds":       kinds,
	}
}

// trailingSpace is the space a removed media link leaves at a line's end.
var trailingSpace = regexp.MustCompile(`[ \t]+\n`)

// ArchiveMediaFile reads an export media file name,
// "<handle>_<status ID>_<type>_<position from 1>_<date>.<ext>", into the
// status and the zero-based position of its image. Only photos qualify: a
// video or GIF file is not the thumbnail the post keeps.
func ArchiveMediaFile(name string) (status string, position int, ok bool) {
	match := archiveMediaName.FindStringSubmatch(name)
	if match == nil {
		return "", 0, false
	}
	n, err := strconv.Atoi(match[2])
	if err != nil || n < 1 || n > 4 {
		return "", 0, false
	}
	return match[1], n - 1, true
}

var archiveMediaName = regexp.MustCompile(`^[A-Za-z0-9_]{1,30}_(\d{10,22})_photo_(\d)_\d{8}\.(?:jpe?g|png|webp|gif)$`)
