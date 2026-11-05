package sqlite

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/yexca/kano-navi/backend/internal/testutil"
)

const (
	repostedPostID = "1900000000000000200"
	originalPostID = "1900000000000000201"
	originalAvatar = "https://pbs.twimg.com/profile_images/1900000000000000201/avatar_normal.jpg"
)

func repostTestStore(t *testing.T) (string, *Store) {
	t.Helper()
	root := t.TempDir()
	path := filepath.Join(root, "database", "kano.sqlite")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(testutil.NewSnapshot(t), path); err != nil {
		t.Fatal(err)
	}
	s, err := OpenWorkflowCore(path, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return root, s
}

func TestIngestBambinoRepostProjectsTheOriginalStatus(t *testing.T) {
	root, s := repostTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	published := ISOTime(now.Add(-time.Minute))
	originalURL := "https://x.com/original_synthetic/status/" + originalPostID
	if _, err := s.IngestBambinoPost(ctx, BambinoPost{
		ID:            repostedPostID,
		AccountHandle: "kano_2525",
		Text:          "original wording",
		PublishedAt:   published,
		URL:           "https://x.com/kano_2525/status/" + repostedPostID,
		Repost:        &BambinoRepost{ID: originalPostID, AuthorHandle: "original_synthetic", AuthorName: "Original", AuthorAvatar: originalAvatar, PublishedAt: published, URL: originalURL},
		RawJSON:       `{"ingest_hash":"synthetic"}`,
	}); err != nil {
		t.Fatal(err)
	}
	var stored sql.NullString
	if err := s.DB().QueryRow(`SELECT repost_json FROM posts WHERE id=?`, repostedPostID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if want := `{"id":"` + originalPostID + `","authorHandle":"original_synthetic","authorName":"Original","authorAvatar":"` + originalAvatar + `","publishedAt":"` + published + `","url":"` + originalURL + `"}`; stored.String != want {
		t.Fatalf("repost_json=%s want=%s", stored.String, want)
	}
	// The avatar is a media candidate, cached by the media step like any image.
	if got := linkedMedia(t, s.DB(), repostedPostID, roleRepostAvatar); got != originalAvatar {
		t.Fatalf("repost avatar link=%s", got)
	}

	dashboard, err := s.GetDashboard(ctx, 30, now)
	if err != nil {
		t.Fatal(err)
	}
	var found *Post
	for i := range dashboard.Posts {
		if dashboard.Posts[i].ID == repostedPostID {
			found = &dashboard.Posts[i]
		}
	}
	if found == nil || found.Repost == nil {
		t.Fatalf("dashboard post=%+v", found)
	}
	// The wording belongs to the original author; the handle stays the reposting account.
	if found.AccountHandle == nil || *found.AccountHandle != "kano_2525" || found.Repost.ID != originalPostID ||
		found.Repost.AuthorHandle != "original_synthetic" || found.Repost.AuthorName != "Original" || found.Repost.AuthorAvatar != "" || found.Repost.URL != originalURL {
		t.Fatalf("post=%+v repost=%+v", found, found.Repost)
	}

	// Once the file is downloaded, the projection serves it from the cache.
	content := []byte("synthetic avatar")
	hash := sha256Hex(string(content))
	cache := "x/sha256/" + hash + ".jpg"
	if err = os.MkdirAll(filepath.Join(root, "x", "sha256"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, filepath.FromSlash(cache)), content, 0o644); err != nil {
		t.Fatal(err)
	}
	avatarID, _, err := mediaIdentity(originalAvatar)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := s.DB().Exec(`UPDATE media_assets SET status='ready',sha256=?,cache_path=?,mime_type='image/jpeg',width=200,height=200 WHERE id=?`, hash, cache, avatarID)
	if err != nil {
		t.Fatal(err)
	}
	if affected, _ := updated.RowsAffected(); affected != 1 {
		t.Fatalf("avatar asset %s not updated, affected=%d", avatarID, affected)
	}
	dashboard, err = s.GetDashboard(ctx, 30, now)
	if err != nil {
		t.Fatal(err)
	}
	for _, post := range dashboard.Posts {
		if post.ID == repostedPostID && (post.Repost == nil || post.Repost.AuthorAvatar != "/media/"+avatarID+"?v="+hash) {
			t.Fatalf("cached repost avatar=%+v", post.Repost)
		}
	}
}

func TestRepostBackfillReadsStoredRawJSON(t *testing.T) {
	_, s := repostTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	published := ISOTime(now.Add(-time.Minute))
	originalURL := "https://x.com/original_synthetic/status/" + originalPostID
	insert := func(id, raw string) {
		t.Helper()
		if _, err := s.DB().Exec(`INSERT INTO posts (id,source,account_handle,type,text,published_at,url,raw_json) VALUES (?,'x','kano_2525','daily','original wording',?,?,?)`,
			id, published, "https://x.com/kano_2525/status/"+id, raw); err != nil {
			t.Fatal(err)
		}
	}
	insert(repostedPostID, `{"bambino":{"twitter":{"retweeted_tweet":{"id":"`+originalPostID+`","url":"`+originalURL+`","author_handle":"original_synthetic","author_display_name":"Original","author_avatar_url":"`+originalAvatar+`","published_at":"`+published+`","text":"original wording"}}}}`)
	// A nested status that is not an x.com page of its own id is not a repost.
	insert("1900000000000000202", `{"bambino":{"twitter":{"retweeted_tweet":{"id":"`+originalPostID+`","url":"https://evil.example.invalid/status/`+originalPostID+`","author_handle":"original_synthetic"}}}}`)

	tx, err := s.DB().BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = backfillReposts(ctx, tx); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	var stored sql.NullString
	if err = s.DB().QueryRow(`SELECT repost_json FROM posts WHERE id=?`, repostedPostID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if want := `{"id":"` + originalPostID + `","authorHandle":"original_synthetic","authorName":"Original","authorAvatar":"` + originalAvatar + `","publishedAt":"` + published + `","url":"` + originalURL + `"}`; stored.String != want {
		t.Fatalf("repost_json=%s want=%s", stored.String, want)
	}
	if got := linkedMedia(t, s.DB(), repostedPostID, roleRepostAvatar); got != originalAvatar {
		t.Fatalf("backfilled avatar link=%s", got)
	}
	var foreign sql.NullString
	if err = s.DB().QueryRow(`SELECT repost_json FROM posts WHERE id='1900000000000000202'`).Scan(&foreign); err != nil || foreign.Valid {
		t.Fatalf("foreign repost_json=%v err=%v", foreign, err)
	}

	dashboard, err := s.GetDashboard(ctx, 30, now)
	if err != nil {
		t.Fatal(err)
	}
	for _, post := range dashboard.Posts {
		if post.ID == repostedPostID && (post.Repost == nil || post.Repost.AuthorHandle != "original_synthetic" || post.Repost.AuthorAvatar != "") {
			t.Fatalf("backfilled repost=%+v", post.Repost)
		}
	}
}
