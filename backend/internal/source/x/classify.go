package x

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
)

// Status outcomes: what an answer says a stored status is for the account it
// is stored under. posts.upstream_kind records them (StatusGone aside), and
// a review of an earlier import's posts reports them.
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
// ErrStatusUnclassified. It reads the answer as StatusKind does, and an
// answer of the account without reply fields replies to nobody.
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
	if firstString(payload, "tweetID", "tweetId", "id") != id || payloadAuthor(payload) == "" {
		return "", ErrStatusUnclassified
	}
	if kind := StatusKind(payload, account); kind != "" {
		return kind, nil
	}
	return StatusTopLevel, nil
}

// StatusKind reads what a status answer (a detail answer, a timeline entry,
// or an X API post as APIPosts maps it) says the status is for the account
// it is stored under: StatusForeign for another author's status,
// StatusRepost for the account's repost, and otherwise StatusTopLevel,
// StatusSelfReply or StatusReply by the account its reply fields name. It is
// "" when the answer cannot tell: it names no author, or carries none of the
// reply fields (vxtwitter's replyingTo and replyingToID, FxTwitter's
// replying_to and replying_to_status), not even as null.
func StatusKind(payload map[string]any, account string) string {
	author := payloadAuthor(payload)
	switch {
	case author == "":
		return ""
	case !strings.EqualFold(author, strings.TrimPrefix(account, "@")):
		return StatusForeign
	case len(object(payload["retweet"])) > 0:
		return StatusRepost
	case !hasReplyFields(payload):
		return ""
	}
	to := replyingTo(payload)
	switch {
	case to == "":
		return StatusTopLevel
	case strings.EqualFold(to, strings.TrimPrefix(account, "@")):
		return StatusSelfReply
	}
	return StatusReply
}

// payloadAuthor is the handle an answer names as the status's author.
func payloadAuthor(payload map[string]any) string {
	author := firstString(object(payload["user"]), "screen_name", "screenName")
	if author == "" {
		author = firstString(object(payload["author"]), "screen_name", "screenName")
	}
	if author == "" {
		author = stringValue(payload["user_screen_name"])
	}
	return strings.TrimPrefix(author, "@")
}

var replyFields = []string{"replyingTo", "replying_to", "replyingToID", "replying_to_status"}

// hasReplyFields reports whether an answer carries any reply field, null
// included: a null field says the status replies to nobody.
func hasReplyFields(payload map[string]any) bool {
	for _, key := range replyFields {
		if _, ok := payload[key]; ok {
			return true
		}
	}
	return false
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
			if len(value) > 0 {
				return "?"
			}
		}
	}
	if firstString(payload, "replyingToID", "replying_to_status") != "" {
		return "?"
	}
	return ""
}
