package x

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
)

// Status outcomes: what a detail answer says a stored status is, for an
// account whose earlier import may have filed replies and other authors'
// statuses as its own posts.
const (
	// StatusTopLevel is the account's own post that replies to nobody.
	StatusTopLevel = "top_level"
	// StatusSelfReply continues one of the account's own threads.
	StatusSelfReply = "self_reply"
	// StatusReply answers another account.
	StatusReply = "reply"
	// StatusForeign is another author's status.
	StatusForeign = "foreign"
	// StatusRepost is the account's repost under its own ID.
	StatusRepost = "repost"
	// StatusGone no longer exists, or its author's posts cannot be read.
	StatusGone = "gone"
)

// ErrStatusUnclassified is a detail answer that names another status or
// cannot be read; the status should be requested again.
var ErrStatusUnclassified = errors.New("x_status_unclassified")

// ClassifyDetail reads the answer of Client.Detail for the account's status
// id. A request error is returned as it is, except that a gone status is an
// outcome; an answer that names no status, or another one, is
// ErrStatusUnclassified.
func ClassifyDetail(body []byte, requestErr error, account, id string) (string, error) {
	if requestErr != nil {
		if isGone(requestErr) {
			return StatusGone, nil
		}
		return "", requestErr
	}
	var payload map[string]any
	if json.Unmarshal(body, &payload) != nil {
		// vxtwitter answers a status it cannot read (deleted, or its
		// author's posts are private or suspended) with this page.
		if bytes.Contains(body, []byte("Failed to scan your link")) {
			return StatusGone, nil
		}
		return "", ErrStatusUnclassified
	}
	if firstString(payload, "tweetID", "tweetId", "id") != id {
		return "", ErrStatusUnclassified
	}
	author := firstString(object(payload["user"]), "screen_name", "screenName")
	if author == "" {
		author = firstString(object(payload["author"]), "screen_name", "screenName")
	}
	if author == "" {
		author = stringValue(payload["user_screen_name"])
	}
	author = strings.TrimPrefix(author, "@")
	switch {
	case author == "":
		return "", ErrStatusUnclassified
	case !strings.EqualFold(author, account):
		return StatusForeign, nil
	case len(object(payload["retweet"])) > 0:
		return StatusRepost, nil
	}
	to := replyingTo(payload)
	switch {
	case to == "":
		return StatusTopLevel, nil
	case strings.EqualFold(to, account):
		return StatusSelfReply, nil
	}
	return StatusReply, nil
}

// replyingTo is the handle a status answers: vxtwitter's replyingTo, or
// FxTwitter's replying_to as a handle or an object naming one. A reply whose
// handle is unknown but whose replied-to status is named answers "?".
func replyingTo(payload map[string]any) string {
	for _, key := range []string{"replyingTo", "replying_to"} {
		switch value := payload[key].(type) {
		case string:
			if handle := strings.TrimPrefix(strings.TrimSpace(value), "@"); handle != "" {
				return handle
			}
		case map[string]any:
			if handle := firstString(value, "screen_name", "screenName", "handle"); handle != "" {
				return strings.TrimPrefix(handle, "@")
			}
		}
	}
	if firstString(payload, "replyingToID", "replying_to_status") != "" {
		return "?"
	}
	return ""
}
