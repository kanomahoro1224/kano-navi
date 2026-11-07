package sqlite

import (
	"context"
	"testing"
	"time"
)

// A Bilibili dynamic keeps its own source row and its own account spelling: the
// configured X accounts must not rewrite it, and its media is cached under the
// Bilibili source so the mirror is attributable.
func TestIngestBambinoBilibiliPost(t *testing.T) {
	_, s := repostTestStore(t)
	ctx := context.Background()
	published := ISOTime(time.Now().UTC().Add(-time.Hour))
	const id = "1150000000000000000"
	const image = "https://i0.hdslb.com/bfs/new_dyn/kano.jpg"

	if _, err := s.IngestBambinoPost(ctx, BambinoPost{
		ID:            id,
		Source:        "bilibili",
		AccountHandle: "鹿乃",
		Text:          "今天也元气满满",
		PublishedAt:   published,
		URL:           "https://t.bilibili.com/" + id,
		MediaURLs:     []string{image},
		RawJSON:       `{"ingest_hash":"synthetic"}`,
	}); err != nil {
		t.Fatal(err)
	}

	var source, text string
	var account string
	if err := s.DB().QueryRow(`SELECT source,account_handle,text FROM posts WHERE id=?`, id).Scan(&source, &account, &text); err != nil {
		t.Fatal(err)
	}
	if source != "bilibili" || account != "鹿乃" {
		t.Fatalf("source=%q account=%q", source, account)
	}
	if got := linkedMedia(t, s.DB(), id, "post-image"); got != image {
		t.Fatalf("post image link=%s", got)
	}
	var mediaSource string
	if err := s.DB().QueryRow(`SELECT source FROM media_assets WHERE source_url=?`, image).Scan(&mediaSource); err != nil {
		t.Fatal(err)
	}
	if mediaSource != "bilibili" {
		t.Fatalf("media source=%q", mediaSource)
	}
}

// Forgetting the source must keep behaving like the X-only writer it replaced.
func TestIngestBambinoPostDefaultsToX(t *testing.T) {
	_, s := repostTestStore(t)
	const id = "1900000000000000300"
	if _, err := s.IngestBambinoPost(context.Background(), BambinoPost{
		ID:            id,
		AccountHandle: "kano_2525",
		Text:          "today",
		PublishedAt:   ISOTime(time.Now().UTC().Add(-time.Hour)),
		URL:           "https://twitter.com/kano_2525/status/" + id,
		RawJSON:       `{"ingest_hash":"synthetic"}`,
	}); err != nil {
		t.Fatal(err)
	}
	var source, url string
	if err := s.DB().QueryRow(`SELECT source,url FROM posts WHERE id=?`, id).Scan(&source, &url); err != nil {
		t.Fatal(err)
	}
	if source != "x" || url != "https://x.com/kano_2525/status/"+id {
		t.Fatalf("source=%q url=%q", source, url)
	}
}
