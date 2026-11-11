package youtube

import (
	"context"
	"net/url"
	"strings"
)

// CodeOtherChannel answers an operator's single read of a video another
// channel uploaded.
const CodeOtherChannel = "youtube_other_channel"

// videoHosts are the youtube.com addresses a video link may use; youtu.be
// short links name the ID in their path.
var videoHosts = map[string]bool{"youtube.com": true, "www.youtube.com": true, "m.youtube.com": true}

// ParseVideoID accepts a video ID or a YouTube video link (a watch page, a
// youtu.be short link, or a /shorts/, /live/ or /embed/ page) and returns
// the ID.
func ParseVideoID(input string) (string, bool) {
	input = strings.TrimSpace(input)
	if videoID.MatchString(input) {
		return input, true
	}
	if !strings.Contains(input, "://") {
		input = "https://" + input
	}
	u, err := url.Parse(input)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.Port() != "" {
		return "", false
	}
	host := strings.ToLower(u.Hostname())
	path := strings.Trim(u.Path, "/")
	id := ""
	switch {
	case host == "youtu.be":
		id = path
	case !videoHosts[host]:
		return "", false
	case path == "watch":
		id = u.Query().Get("v")
	default:
		for _, prefix := range []string{"shorts/", "live/", "embed/"} {
			if rest, found := strings.CutPrefix(path, prefix); found {
				id = rest
			}
		}
	}
	if !videoID.MatchString(id) {
		return "", false
	}
	return id, true
}

// ReadVideo reads one video an operator names, for the console's single
// add: through the Data API under the recent step's rules when a key is
// configured (one videos.list call, plus the newest page of each format
// playlist for an ended broadcast that needs a verdict), and through its
// watch page otherwise or when the API leaves it undescribed. stored holds
// its stored row, if any. Only a video of the configured channel is
// accepted (CodeOtherChannel). It returns the checked video and the
// localized titles the API answered.
func (a *Adapter) ReadVideo(ctx context.Context, id string, stored map[string]Video) (Video, map[string]map[string]string, error) {
	if !videoID.MatchString(id) {
		return Video{}, nil, &Error{"youtube_invalid_video"}
	}
	old, found := stored[id]
	if !found {
		old = Video{ID: id, URL: "https://www.youtube.com/watch?v=" + id, Kind: KindUnknown}
	}
	var v Video
	var localized map[string]map[string]string
	if a.Options.APIKey != "" {
		described := a.DescribeVideos(ctx, []Video{old}, stored, nil)
		if ctx.Err() != nil {
			return Video{}, nil, ctx.Err()
		}
		for _, answered := range described.Videos {
			if answered.ID == id && !answered.Unclassified {
				v, localized = answered, described.Localized
			}
		}
	}
	if v.ID == "" {
		checked, err := a.watch(ctx, id)
		if err != nil {
			return Video{}, nil, err
		}
		v = completeDetails(checked, old, a.Now())
	}
	switch {
	case v.ChannelID == "":
		return Video{}, nil, &Error{"youtube_watch_details_incomplete"}
	case v.ChannelID != a.Options.ChannelID:
		return Video{}, nil, &Error{CodeOtherChannel}
	}
	return v, localized, nil
}
