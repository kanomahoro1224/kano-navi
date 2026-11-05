package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// repostedPostsSQL selects stored X posts whose raw_json names a reposted
// status but that have no repost projection yet. Only rows without
// repost_json are read, so the step is idempotent.
const repostedPostsSQL = `SELECT id,raw_json FROM posts
 WHERE source='x' AND repost_json IS NULL AND json_valid(raw_json) AND (
  json_type(raw_json,'$.bambino.twitter.retweeted_tweet')='object'
  OR json_type(raw_json,'$.bambino.retweet')='object')`

// backfillReposts gives posts stored before the repost projection their
// original status from the raw_json each kept. A post whose nested status is
// not an x.com status of its own id keeps no projection, which reads as an
// ordinary post rather than a repost attributed to the wrong account.
func backfillReposts(ctx context.Context, tx *sql.Tx) error {
	type row struct{ id, raw string }
	rows, err := tx.QueryContext(ctx, repostedPostsSQL)
	if err != nil {
		return ErrSchema
	}
	found := []row{}
	for rows.Next() {
		var r row
		if err = rows.Scan(&r.id, &r.raw); err != nil {
			rows.Close()
			return ErrSchema
		}
		found = append(found, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return ErrSchema
	}
	fail := func() error { return fmt.Errorf("%w: repost backfill failed", ErrSchema) }
	now := workflowTime(time.Now())
	changed := false
	for _, r := range found {
		repost := repostFromRaw(r.raw)
		if repost == nil {
			continue
		}
		body, marshalErr := json.Marshal(repost)
		if marshalErr != nil {
			return fail()
		}
		if _, err = tx.ExecContext(ctx, `UPDATE posts SET repost_json=? WHERE id=?`, string(body), r.id); err != nil {
			return fail()
		}
		if repost.AuthorAvatar != "" {
			if err = replaceMediaLink(ctx, tx, "x", repost.AuthorAvatar, "post", r.id, roleRepostAvatar, 0, now); err != nil {
				return fail()
			}
		}
		changed = true
	}
	if changed {
		if err = bumpRevision(ctx, tx, now); err != nil {
			return fail()
		}
	}
	return nil
}

// repostFromRaw reads the reposted status out of a stored raw_json. It
// accepts both the collector's twitter metadata shape and an ingest event's
// nested retweet, and drops anything that is not an x.com status URL.
func repostFromRaw(raw string) *PostRepost {
	var payload map[string]any
	if json.Unmarshal([]byte(raw), &payload) != nil {
		return nil
	}
	bambino, _ := payload["bambino"].(map[string]any)
	if twitter, ok := bambino["twitter"].(map[string]any); ok {
		if nested, ok := twitter["retweeted_tweet"].(map[string]any); ok {
			return newRepost(textField(nested, "id"), textField(nested, "author_handle"), textField(nested, "author_display_name"), textField(nested, "author_avatar_url"), textField(nested, "published_at"), textField(nested, "url"))
		}
	}
	if nested, ok := bambino["retweet"].(map[string]any); ok {
		author, _ := nested["author"].(map[string]any)
		return newRepost(textField(nested, "id"), textField(author, "handle"), textField(author, "display_name"), textField(author, "avatar_url"), textField(nested, "published_at"), textField(nested, "url"))
	}
	return nil
}

func newRepost(id, handle, name, avatar, publishedAt, url string) *PostRepost {
	id = strings.TrimSpace(id)
	handle = strings.TrimPrefix(strings.TrimSpace(handle), "@")
	url = strings.TrimSpace(url)
	if !quoteIDPattern.MatchString(id) {
		return nil
	}
	if url == "" && handle != "" {
		url = "https://x.com/" + handle + "/status/" + id
	}
	if url == "" || !quoteLinksTo(url, id) {
		return nil
	}
	return &PostRepost{ID: id, AuthorHandle: handle, AuthorName: strings.TrimSpace(name), AuthorAvatar: validRepostAvatar(avatar), PublishedAt: strings.TrimSpace(publishedAt), URL: url}
}

func textField(m map[string]any, key string) string {
	value, _ := m[key].(string)
	return value
}
