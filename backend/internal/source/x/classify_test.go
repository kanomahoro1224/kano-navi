package x

import (
	"errors"
	"net/http"
	"testing"
)

// A detail answer tells an own top-level post from a thread reply, a reply
// to someone else, another author's status and the account's repost; a
// status the source cannot read is gone.
func TestClassifyDetailReadsWhatAStoredStatusIs(t *testing.T) {
	const id = "1900000000000000901"
	for name, tc := range map[string]struct {
		body string
		err  error
		want string
	}{
		"top level":   {body: `{"tweetID":"` + id + `","user_screen_name":"kano_2525","replyingTo":null}`, want: StatusTopLevel},
		"thread":      {body: `{"tweetID":"` + id + `","user_screen_name":"kano_2525","replyingTo":"Kano_2525","replyingToID":"1"}`, want: StatusSelfReply},
		"reply":       {body: `{"tweetID":"` + id + `","user_screen_name":"kano_2525","replyingTo":"someone_synthetic"}`, want: StatusReply},
		"reply by id": {body: `{"tweetID":"` + id + `","user_screen_name":"kano_2525","replyingToID":"1"}`, want: StatusReply},
		"fx reply":    {body: `{"id":"` + id + `","author":{"screen_name":"kano_2525"},"replying_to":{"screen_name":"someone_synthetic"}}`, want: StatusReply},
		"foreign":     {body: `{"tweetID":"` + id + `","user_screen_name":"fan_synthetic"}`, want: StatusForeign},
		"repost":      {body: `{"tweetID":"` + id + `","user_screen_name":"kano_2525","retweet":{"tweetID":"2"}}`, want: StatusRepost},
		"gone page":   {body: `<html><meta content="Failed to scan your link! This may be due to a deleted tweet"></html>`, want: StatusGone},
		"not found":   {err: &Error{Code: ErrHTTP, Status: http.StatusNotFound}, want: StatusGone},
	} {
		got, err := ClassifyDetail([]byte(tc.body), tc.err, "kano_2525", id)
		if err != nil || got != tc.want {
			t.Fatalf("%s: %q %v", name, got, err)
		}
	}
}

func TestClassifyDetailAsksAgainForAnswersItCannotUse(t *testing.T) {
	const id = "1900000000000000901"
	limited := &Error{Code: ErrRateLimited, Status: http.StatusTooManyRequests}
	if _, err := ClassifyDetail(nil, limited, "kano_2525", id); !errors.Is(err, limited) {
		t.Fatalf("rate limit: %v", err)
	}
	for name, body := range map[string]string{
		"other status": `{"tweetID":"1900000000000000902","user_screen_name":"kano_2525"}`,
		"no author":    `{"tweetID":"` + id + `"}`,
		"other page":   `<html>maintenance</html>`,
	} {
		if _, err := ClassifyDetail([]byte(body), nil, "kano_2525", id); !errors.Is(err, ErrStatusUnclassified) {
			t.Fatalf("%s: %v", name, err)
		}
	}
}
