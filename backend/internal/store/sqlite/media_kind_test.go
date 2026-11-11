package sqlite

import (
	"context"
	"strings"
	"testing"
	"time"

	xsource "github.com/yexca/kano-navi/backend/internal/source/x"
)

// A post's links record what each of its images and its quote's images
// shows, and the board's read reports it; a link without a kind reads as a
// photo.
func TestMediaLinksKeepTheImageKind(t *testing.T) {
	f := newRelatedFixture(t)
	ctx := context.Background()
	post := xsource.Post{ID: "1900000000000000810", AccountHandle: "kano_2525", Type: "daily", Text: "clip", PublishedAt: "2026-10-11T00:00:00Z",
		URL: "https://x.com/kano_2525/status/1900000000000000810", MediaURLs: []string{twimg("still"), twimg("clip")},
		MediaKinds: []string{xsource.MediaPhoto, xsource.MediaVideo}, Raw: map[string]any{},
		Quote: &xsource.Quote{ID: "1900000000000000811", AuthorHandle: "other", Text: "loop", URL: "https://x.com/other/status/1900000000000000811",
			MediaURLs: []string{twimg("loop")}, MediaKinds: []string{xsource.MediaGIF}}}
	if err := f.s.WriteXRecent(ctx, "kano_2525", xsource.Result{Account: "kano_2525", Posts: []xsource.Post{post}}, true); err != nil {
		t.Fatal(err)
	}
	if got := f.text(`SELECT group_concat(role||':'||position||':'||COALESCE(kind,'-'),',') FROM (SELECT * FROM media_links WHERE owner_id=? ORDER BY role,position)`, post.ID); got != "post-image:0:photo,post-image:1:video,quote-image:0:gif" {
		t.Fatalf("links=%s", got)
	}
	if _, err := f.s.DB().Exec(`UPDATE media_links SET kind=NULL WHERE owner_id=? AND position=0 AND role='post-image'`, post.ID); err != nil {
		t.Fatal(err)
	}
	media, err := f.s.queryPostMedia(ctx, post.ID, "post-image")
	if err != nil || len(media) != 2 || media[0].Kind != "photo" || media[1].Kind != "video" {
		t.Fatalf("media=%+v err=%v", media, err)
	}
}

// Startup that adds the kind marks a stored post's video and GIF links from
// its raw_json alone, and leaves the rest as photos.
func TestMediaKindBackfillReadsStoredPayloads(t *testing.T) {
	f := newRelatedFixture(t)
	ctx := context.Background()
	const id = "1900000000000000820"
	raw := `{"tweetID":"` + id + `","mediaURLs":["` + twimg("still") + `","` + twimg("clip") + `"],
		"media_extended":[{"type":"image","url":"` + twimg("still") + `"},{"type":"video","url":"https://video.twimg.com/clip.mp4","thumbnail_url":"` + twimg("clip") + `"}],
		"qrt":{"tweetID":"1900000000000000821","text":"loop","user_screen_name":"other","media_extended":[{"type":"gif","thumbnail_url":"` + twimg("loop") + `"}]}}`
	now := ISOTime(time.Now())
	if _, err := f.s.DB().Exec(`INSERT INTO posts(id,source,account_handle,text,published_at,url,raw_json) VALUES(?,'x','kano_2525','clip','2026-10-11T00:00:00.000Z',?,?)`,
		id, "https://x.com/kano_2525/status/"+id, raw); err != nil {
		t.Fatal(err)
	}
	tx, err := f.s.DB().BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	for i, link := range []struct{ url, role string }{{twimg("still"), "post-image"}, {twimg("clip"), "post-image"}, {twimg("loop"), roleQuoteImage}} {
		position := i
		if link.role == roleQuoteImage {
			position = 0
		}
		if err = replaceMediaLink(ctx, tx, "x", link.url, "post", id, link.role, position, now); err != nil {
			t.Fatal(err)
		}
	}
	if err = backfillMediaKinds(ctx, tx); err != nil {
		t.Fatal(err)
	}
	var got string
	if err = tx.QueryRow(`SELECT group_concat(role||':'||position||':'||COALESCE(kind,'-'),',') FROM (SELECT * FROM media_links WHERE owner_id=? ORDER BY role,position)`, id).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != "post-image:0:-,post-image:1:video,quote-image:0:gif" {
		t.Fatalf("links=%s", got)
	}
	if strings.Contains(got, "photo") {
		t.Fatal("the backfill wrote a photo kind")
	}
}
