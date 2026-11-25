package x

import (
	"errors"
	"net/http"
	"testing"
	"time"
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

// Every answer that can tell what a status is says it the same way: the
// FxTwitter timeline's replying_to, the vxtwitter detail's replyingTo,
// retweet and author. An answer without an author or without any reply
// field cannot tell, and says nothing.
func TestStatusKindReadsEveryAnswerAlike(t *testing.T) {
	for name, tc := range map[string]struct {
		payload map[string]any
		want    string
	}{
		"fx top level":   {map[string]any{"author": map[string]any{"screen_name": "Kano_2525"}, "replying_to": nil}, StatusTopLevel},
		"fx thread":      {map[string]any{"author": map[string]any{"screen_name": "kano_2525"}, "replying_to": map[string]any{"screen_name": "kano_2525", "post": "1"}}, StatusSelfReply},
		"fx reply":       {map[string]any{"author": map[string]any{"screen_name": "kano_2525"}, "replying_to": map[string]any{"screen_name": "fan_synthetic"}}, StatusReply},
		"fx reply by id": {map[string]any{"author": map[string]any{"screen_name": "kano_2525"}, "replying_to": map[string]any{"post": "1"}}, StatusReply},
		"vx top level":   {map[string]any{"user_screen_name": "kano_2525", "replyingTo": nil, "replyingToID": nil}, StatusTopLevel},
		"vx repost":      {map[string]any{"user_screen_name": "kano_2525", "retweet": map[string]any{"tweetID": "2"}}, StatusRepost},
		"foreign":        {map[string]any{"user": map[string]any{"screen_name": "fan_synthetic"}}, StatusForeign},
		"no reply field": {map[string]any{"author": map[string]any{"screen_name": "kano_2525"}}, ""},
		"no author":      {map[string]any{"replying_to": nil}, ""},
	} {
		if got := StatusKind(tc.payload, "@kano_2525"); got != tc.want {
			t.Fatalf("%s: %q, want %q", name, got, tc.want)
		}
	}
}

// An X API post says what it is through its replied_to reference: a thread
// reply when the parent is the account's, a reply otherwise or when X does
// not return the parent, and a top-level post without one.
func TestAPIPostsTellTheirKind(t *testing.T) {
	at := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	answer := apiAnswer(t, "", statusFixture{id: "1900000000000000310", text: "top", at: at})
	reply := func(id, parent string) map[string]any {
		return map[string]any{"id": id, "author_id": "74", "created_at": "2026-10-20T00:00:00.000Z", "text": "reply " + id,
			"referenced_tweets": []any{map[string]any{"type": "replied_to", "id": parent}}}
	}
	answer["data"] = append(answer["data"].([]any),
		reply("1900000000000000311", "1900000000000000310"),
		reply("1900000000000000312", "1900000000000000001"),
		reply("1900000000000000313", "1900000000000000002"))
	includes := answer["includes"].(map[string]any)
	includes["tweets"] = []any{
		map[string]any{"id": "1900000000000000310", "author_id": "74", "text": "top"},
		map[string]any{"id": "1900000000000000001", "author_id": "99", "text": "a fan's question"},
	}
	includes["users"] = append(includes["users"].([]any), map[string]any{"id": "99", "username": "fan_synthetic"})
	posts, _, _, err := APIPosts(answer, "kano")
	if err != nil || len(posts) != 4 {
		t.Fatalf("posts=%+v err=%v", posts, err)
	}
	want := []string{StatusTopLevel, StatusSelfReply, StatusReply, StatusReply}
	for i, post := range posts {
		if post.Kind != want[i] || StatusKind(post.Raw, "kano") != want[i] {
			t.Fatalf("%s: kind %q, raw says %q, want %q", post.ID, post.Kind, StatusKind(post.Raw, "kano"), want[i])
		}
	}
}
