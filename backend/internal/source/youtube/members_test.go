package youtube

import (
	"context"
	"maps"
	"net/http"
	"slices"
	"testing"
)

// membersPage is a members-only playlist page naming one video no other
// answer mentions.
func membersPage() string {
	return apiItems(playlistItem("MMMMMMMMMMM", "2025-10-01T00:00:00Z"))
}

// membersAnswers are a members test's answers: videos.list, the members-only
// playlist page and the members-only playlist's feed. A nil answer fails the
// test when it is asked for.
type membersAnswers struct {
	videos, playlist, feed func() *http.Response
}

// membersClient lists the upload AAAAAAAAAAA through the Data API (or the
// channel feed without a key) and answers everything else as an ordinary run
// does. It counts the members-only playlist and feed requests.
func membersClient(t *testing.T, answers membersAnswers, playlist, feed *int) *Client {
	answer := func(name string, f func() *http.Response) *http.Response {
		if f == nil {
			t.Fatalf("the %s was read", name)
		}
		return f()
	}
	return NewClient(&http.Client{Transport: fakeTransport(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/youtube/v3/playlistItems":
			if r.Header.Get("X-Goog-Api-Key") != syntheticKey || r.URL.Host != "www.googleapis.com" {
				t.Fatalf("key not confined to the header: %s", r.URL)
			}
			switch r.URL.Query().Get("playlistId") {
			case "UU" + syntheticChannel[2:]:
				return response(200, apiItems(playlistItem("AAAAAAAAAAA", "2026-01-02T06:00:00Z"))), nil
			case "UUMO" + syntheticChannel[2:]:
				*playlist++
				return answer("members-only playlist", answers.playlist), nil
			}
		case "/youtube/v3/videos":
			return answer("videos.list", answers.videos), nil
		case "/feeds/videos.xml":
			if r.URL.Query().Get("playlist_id") == "UUMO"+syntheticChannel[2:] {
				*feed++
				return answer("members-only feed", answers.feed), nil
			}
			if r.URL.Query().Get("channel_id") == syntheticChannel {
				return response(200, syntheticFeed([2]string{"AAAAAAAAAAA", ""})), nil
			}
		case "/channel/" + syntheticChannel + "/streams":
			return response(200, ``), nil
		case "/watch":
			return response(200, watchPage(r.URL.Query().Get("v"), `"isLiveContent":false,"lengthSeconds":"60"`, "")), nil
		}
		t.Fatalf("unexpected request %s", r.URL)
		return nil, nil
	})})
}

func publicUpload() *http.Response {
	return response(200, apiItems(`{"id":"AAAAAAAAAAA","snippet":{"title":"upload","liveBroadcastContent":"none"},"contentDetails":{"duration":"PT1M"},"status":{"privacyStatus":"public"}}`))
}

// membersFeed is the members-only feed naming a stored video outside the
// run's snapshot.
func membersFeed() *http.Response {
	return response(200, syntheticFeed([2]string{"OOOOOOOOOOO", ""}))
}

func membersRun(t *testing.T, answers membersAnswers, key string) (Result, int, int) {
	t.Helper()
	playlist, feed := 0, 0
	a := NewAdapter(membersClient(t, answers, &playlist, &feed), Options{ChannelID: syntheticChannel, MaxDetailRequests: 1, APIKey: key})
	a.Now = syntheticNow
	result, err := a.FetchUploads(context.Background(), State{}, List{Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	return result, playlist, feed
}

func TestMembersPlaylistMarksEveryVideoItNames(t *testing.T) {
	// The page names the checked upload, a stored video outside the run's
	// snapshot and one never stored; the writer skips the last. The feed is
	// not needed.
	result, playlist, feed := membersRun(t, membersAnswers{videos: publicUpload, playlist: func() *http.Response {
		return response(200, apiItems(playlistItem("AAAAAAAAAAA", "2026-01-02T06:00:00Z"), playlistItem("OOOOOOOOOOO", "2025-08-28T13:00:00Z"), playlistItem("MMMMMMMMMMM", "2025-10-01T00:00:00Z")))
	}}, "synthetic-api-key")
	// The uploads, members-only playlist pages and one videos.list call.
	if playlist != 1 || feed != 0 || result.APIRequests != 3 || len(result.Errors) != 0 || result.Members != 3 || result.MembersFrom != ListingAPI {
		t.Fatalf("result=%#v playlist=%d feed=%d", result, playlist, feed)
	}
	want := map[string]string{"AAAAAAAAAAA": AvailabilityMembers, "OOOOOOOOOOO": AvailabilityMembers, "MMMMMMMMMMM": AvailabilityMembers}
	if !maps.Equal(result.Availability, want) {
		t.Fatalf("availability=%v", result.Availability)
	}
	// The API calls the checked upload public; the playlist overrides it, so
	// its entry is a members-only one from this run on.
	checked := 0
	for _, v := range result.Videos {
		if v.ID == "AAAAAAAAAAA" && !v.FeedOnly {
			checked++
			if v.Availability != AvailabilityMembers {
				t.Fatalf("video=%#v", v)
			}
		}
	}
	if checked != 1 {
		t.Fatalf("videos=%#v", result.Videos)
	}
}

func TestAFailedMembersPlaylistFallsBackToItsFeed(t *testing.T) {
	for _, tc := range []struct {
		name, code string
		answer     *http.Response
	}{
		{"no members-only playlist", "youtube_api_http_error", response(404, `{"error":{"code":404}}`)},
		{"refused key", "youtube_api_forbidden", response(403, "")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, playlist, feed := membersRun(t, membersAnswers{videos: publicUpload, playlist: func() *http.Response { return tc.answer }, feed: membersFeed}, "synthetic-api-key")
			if playlist != 1 || feed != 1 || result.Inspected != 1 || !slices.Equal(result.Errors, []string{tc.code}) {
				t.Fatalf("result=%#v playlist=%d feed=%d", result, playlist, feed)
			}
			if !maps.Equal(result.Availability, map[string]string{"OOOOOOOOOOO": AvailabilityMembers}) {
				t.Fatalf("availability=%v", result.Availability)
			}
		})
	}
}

func TestAFailedDataAPICallLeavesTheMembersListToTheFeed(t *testing.T) {
	// The quota runs out at videos.list, so the members-only page would fail
	// too and is not asked for; the feed answers instead.
	result, playlist, feed := membersRun(t, membersAnswers{videos: func() *http.Response { return response(403, "") }, feed: membersFeed}, "synthetic-api-key")
	if playlist != 0 || feed != 1 || !slices.Equal(result.Errors, []string{"youtube_api_forbidden"}) || result.MembersFrom != ListingFeed {
		t.Fatalf("result=%#v playlist=%d feed=%d", result, playlist, feed)
	}
	if !maps.Equal(result.Availability, map[string]string{"OOOOOOOOOOO": AvailabilityMembers}) {
		t.Fatalf("availability=%v", result.Availability)
	}
}

func TestWithoutAKeyTheMembersFeedNamesTheMembersOnlyVideos(t *testing.T) {
	result, playlist, feed := membersRun(t, membersAnswers{feed: membersFeed}, "")
	if playlist != 0 || feed != 1 || result.APIRequests != 0 || len(result.Errors) != 0 || result.Listing != ListingFeed {
		t.Fatalf("result=%#v playlist=%d feed=%d", result, playlist, feed)
	}
	if !maps.Equal(result.Availability, map[string]string{"OOOOOOOOOOO": AvailabilityMembers}) {
		t.Fatalf("availability=%v", result.Availability)
	}
}

func TestMembersFeedFailureCostsOnlyItsVerdicts(t *testing.T) {
	result, _, feed := membersRun(t, membersAnswers{videos: publicUpload, playlist: func() *http.Response { return response(404, "") }, feed: func() *http.Response { return response(404, "") }}, "synthetic-api-key")
	if feed != 1 || result.Inspected != 1 || len(result.Availability) != 0 || !slices.Equal(result.Errors, []string{"youtube_api_http_error", "youtube_feed_http_error"}) {
		t.Fatalf("result=%#v feed=%d", result, feed)
	}
}

func TestAFailedChannelFeedSkipsTheMembersFeed(t *testing.T) {
	// Without a key the channel feed is the listing; its outage holds for the
	// members-only feed on the same endpoint, so that is not asked for.
	c := feedClient(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/feeds/videos.xml":
			if r.URL.Query().Get("playlist_id") != "" {
				t.Fatalf("the members-only feed was read after the channel feed failed")
			}
			return response(404, ""), nil
		case "/channel/" + syntheticChannel + "/streams", "/channel/" + syntheticChannel + "/videos":
			return response(200, ``), nil
		}
		t.Fatalf("unexpected request %s", r.URL)
		return nil, nil
	})
	a := NewAdapter(c, Options{ChannelID: syntheticChannel, MaxDetailRequests: 1})
	a.Now = syntheticNow
	result, _ := a.FetchUploads(context.Background(), State{}, List{Limit: 50})
	if len(result.Availability) != 0 || !slices.Equal(result.Errors, []string{"youtube_feed_http_error"}) {
		t.Fatalf("result=%#v", result)
	}
}

func TestMembersListsRejectAnInvalidChannel(t *testing.T) {
	c := NewClient(&http.Client{Transport: fakeTransport(func(*http.Request) (*http.Response, error) {
		t.Fatal("invalid channel made a request")
		return nil, nil
	})})
	if _, err := c.MembersPlaylist(context.Background(), syntheticKey, "UUAAAAAAAAAAAAAAAAAAAAAA"); err == nil || err.Error() != "youtube_invalid_channel" {
		t.Fatalf("playlist err=%v", err)
	}
	if _, err := c.MembersFeed(context.Background(), "UUAAAAAAAAAAAAAAAAAAAAAA"); err == nil || err.Error() != "youtube_invalid_channel" {
		t.Fatalf("feed err=%v", err)
	}
}
