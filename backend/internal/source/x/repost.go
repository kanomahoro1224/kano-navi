package x

import "strings"

// Repost is an own account's repost of another status. X lists a repost with
// the original's ID and publication time, so it has neither of its own. ID is
// the reposted status; Author its author's handle, empty when unknown;
// Status the reposted status as the listing carries it, mapped like a quote,
// or nil when the listing carries none. The board shows no reposts: the link
// and the status are stored apart from posts.
type Repost struct {
	ID     string
	Author string
	Status *Quote
}

// repostedBy is the handle FxTwitter names in a timeline item's reposted_by:
// an author object, or a bare handle.
func repostedBy(status map[string]any) string {
	value := status["reposted_by"]
	handle := stringValue(value)
	if by := object(value); len(by) > 0 {
		handle = firstString(by, "screen_name", "screenName")
	}
	return strings.TrimPrefix(handle, "@")
}

// timelineRepost reads a timeline item as the account's repost: an item of
// another author that names the account in reposted_by.
func timelineRepost(status map[string]any, account string) (Repost, bool) {
	id := stringValue(status["id"])
	if !statusIDPattern.MatchString(id) || !strings.EqualFold(repostedBy(status), account) {
		return Repost{}, false
	}
	repost := Repost{ID: id}
	if handle := stringValue(object(status["author"])["screen_name"]); handlePattern.MatchString(handle) {
		repost.Author = handle
	}
	if quoted := quoteFrom(status); quoted != nil && quoted.ID == id && !quoted.Unavailable {
		repost.Status, repost.Author = quoted, quoted.AuthorHandle
	}
	return repost, true
}

// APIReposts lists the account's reposts in an X API v2 posts answer: its
// posts that reference a status as retweeted, with the status when the
// answer's includes carry it.
func APIReposts(payload map[string]any, account string) []Repost {
	includes := object(payload["includes"])
	users := map[string]map[string]any{}
	for _, item := range list(includes["users"]) {
		if user := object(item); stringValue(user["id"]) != "" {
			users[stringValue(user["id"])] = user
		}
	}
	media := map[string]map[string]any{}
	for _, item := range list(includes["media"]) {
		if m := object(item); stringValue(m["media_key"]) != "" {
			media[stringValue(m["media_key"])] = m
		}
	}
	referenced := map[string]map[string]any{}
	for _, key := range []string{"tweets", "posts"} {
		for _, item := range list(includes[key]) {
			if status := object(item); stringValue(status["id"]) != "" {
				referenced[stringValue(status["id"])] = status
			}
		}
	}
	out := []Repost{}
	seen := map[string]bool{}
	for _, item := range list(payload["data"]) {
		entry := object(item)
		handle := stringValue(users[stringValue(entry["author_id"])]["username"])
		if handle != "" && !strings.EqualFold(handle, account) {
			continue
		}
		id := referenceOf(entry, "retweeted")
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		repost := Repost{ID: id}
		if status, found := referenced[id]; found {
			if author := stringValue(users[stringValue(status["author_id"])]["username"]); handlePattern.MatchString(author) {
				repost.Author = author
			}
			if quoted := quoteFrom(apiQuote(status, users, media)); quoted != nil && quoted.ID == id && !quoted.Unavailable {
				repost.Status, repost.Author = quoted, quoted.AuthorHandle
			}
		}
		out = append(out, repost)
	}
	return out
}
