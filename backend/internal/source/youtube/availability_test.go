package youtube

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"testing"
)

func TestWatchPageTellsWhoCanWatch(t *testing.T) {
	const id = "AAAAAAAAAAA"
	for name, tc := range map[string]struct{ body, want string }{
		"public":   {watchPage(id, `"lengthSeconds":"60"`, ""), AvailabilityPublic},
		"unlisted": {`<script>var ytInitialPlayerResponse = {"videoDetails":{"videoId":"` + id + `","title":"synthetic","lengthSeconds":"60"},"playabilityStatus":{"status":"OK"},"microformat":{"playerMicroformatRenderer":{"isUnlisted":true}}};</script>`, AvailabilityUnlisted},
		// A members-only stream archive still names its start, so it passes.
		"members archive": {`<script>var ytInitialPlayerResponse = {"videoDetails":{"videoId":"` + id + `","title":"synthetic"},"playabilityStatus":{"status":"UNPLAYABLE","reason":"Join this channel to get access to members-only content"},"microformat":{"playerMicroformatRenderer":{"liveBroadcastDetails":{"isLiveNow":false,"startTimestamp":"2026-01-02T09:00:00+00:00","endTimestamp":"2026-01-02T11:00:00+00:00"}}}};</script>`, AvailabilityMembers},
	} {
		v, err := ParseWatch([]byte(tc.body), id)
		if err != nil || v.Availability != tc.want {
			t.Fatalf("%s: availability=%q err=%v", name, v.Availability, err)
		}
	}
}

func TestFailedAvailabilityNeedsAnAnsweredPage(t *testing.T) {
	unknownSignIn := &Error{CodeLoginRequired}
	for _, tc := range []struct {
		name   string
		err    error
		absent bool
		want   string
	}{
		{"private page", &Error{CodePrivate}, false, AvailabilityPrivate},
		{"members page", &Error{CodeMembers}, false, AvailabilityMembers},
		{"left out, unknown sign-in reason", unknownSignIn, true, AvailabilityPrivate},
		{"left out, bot check", errBotCheck, true, ""},
		{"left out, unavailable page", &Error{"youtube_watch_details_incomplete"}, true, AvailabilityRemoved},
		{"answered, incomplete page", &Error{"youtube_watch_details_incomplete"}, false, ""},
		{"left out, no answer", &Error{"youtube_upstream_5xx"}, true, ""},
		{"left out, transport", errors.New("synthetic"), true, ""},
		{"answered, unknown sign-in reason", unknownSignIn, false, ""},
	} {
		if got := FailedAvailability(tc.err, tc.absent); got != tc.want {
			t.Fatalf("%s: %q", tc.name, got)
		}
	}
}

func TestDataAPIStatusKeepsAMembersVerdict(t *testing.T) {
	for _, tc := range []struct{ privacy, stored, want string }{
		{"public", "", AvailabilityPublic},
		{"unlisted", AvailabilityPublic, AvailabilityUnlisted},
		{"public", AvailabilityMembers, AvailabilityMembers},
		{"private", AvailabilityMembers, AvailabilityPrivate},
		{"", AvailabilityPublic, ""},
	} {
		if got := apiAvailability(tc.privacy, tc.stored); got != tc.want {
			t.Fatalf("%+v: %q", tc, got)
		}
	}
}

func TestRecentStepRecordsWhoCanWatch(t *testing.T) {
	var calls [][]string
	var watched []string
	ids := []string{"AAAAAAAAAAA", "BBBBBBBBBBB", "CCCCCCCCCCC"}
	c := detailClient(t, func(int, []string) *http.Response {
		// Only AAAAAAAAAAA is answered; the others are left out.
		return response(200, apiItems(`{"id":"AAAAAAAAAAA","snippet":{"title":"upload","liveBroadcastContent":"none"},"contentDetails":{"duration":"PT1M"},"status":{"privacyStatus":"unlisted"}}`))
	}, func(id string) *http.Response {
		if id == "BBBBBBBBBBB" {
			return response(200, `<script>var ytInitialPlayerResponse = {"playabilityStatus":{"status":"ERROR","reason":"Video unavailable"}};</script>`)
		}
		return response(200, signInPlayer(`"reason":"synthetic"`))
	}, &calls, &watched)
	a := NewAdapter(c, Options{ChannelID: syntheticChannel, MaxDetailRequests: 12, APIKey: "synthetic-api-key"})
	a.Now = syntheticNow
	result, err := a.FetchRecent(context.Background(), storedState(ids), 7)
	if err != nil {
		t.Fatal(err)
	}
	// Neither page is an error, and the sign-in page of a video the API left
	// out does not stop the later page.
	if !slices.Equal(watched, ids[1:]) || len(result.Errors) != 0 {
		t.Fatalf("watched=%v errors=%v", watched, result.Errors)
	}
	if result.Availability["BBBBBBBBBBB"] != AvailabilityRemoved || result.Availability["CCCCCCCCCCC"] != AvailabilityPrivate || len(result.Availability) != 2 {
		t.Fatalf("availability=%v", result.Availability)
	}
	if len(result.Videos) != 1 || result.Videos[0].Availability != AvailabilityUnlisted {
		t.Fatalf("videos=%#v", result.Videos)
	}
}
