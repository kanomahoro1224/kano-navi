package sqlite

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yexca/kano-navi/backend/internal/testutil"
)

func TestAdminPostsPreviewOnlyLocalImages(t *testing.T) {
	// The store's data root is the parent of the database directory, so the
	// snapshot moves under <root>/database next to the cached media tree.
	root := t.TempDir()
	path := filepath.Join(root, "database", "kano.sqlite")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(testutil.NewSnapshot(t), path); err != nil {
		t.Fatal(err)
	}
	content := []byte("synthetic post image")
	hash := sha256Hex(string(content))
	cache := "x/sha256/" + hash + ".jpg"
	if err := os.MkdirAll(filepath.Join(root, "x", "sha256"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(cache)), content, 0o644); err != nil {
		t.Fatal(err)
	}
	image := func(name string) string { return "https://media.example.test/" + name + ".jpg" }
	for _, post := range []struct{ id, account, media, published string }{
		{"linked", "kano_2525", image("first"), "2026-10-13T00:00:00.000Z"},
		{"pending", "kano_2525", image("pending"), "2026-10-12T00:00:00.000Z"},
		{"text", "_Kanotic", "", "2026-10-11T00:00:00.000Z"},
	} {
		var media any
		if post.media != "" {
			media = post.media
		}
		testutil.Exec(t, path, `INSERT INTO posts (id,source,account_handle,type,text,published_at,url,media_url,raw_json) VALUES (?,'x',?,'post','text',?,?,?,'{}')`, post.id, post.account, post.published, "https://x.example.invalid/"+post.id, media)
	}
	for position, name := range []string{"first", "second"} {
		status, cachePath := "ready", any(cache)
		if name == "second" {
			status, cachePath = "pending", nil
		}
		testutil.Exec(t, path, `INSERT INTO media_assets(id,source,source_url,status,sha256,cache_path,created_at,updated_at) VALUES(?,'x',?,?,?,?,'2026-10-10T00:00:00Z','2026-10-10T00:00:00Z')`, sha256Hex(image(name)), image(name), status, hash, cachePath)
		testutil.Exec(t, path, `INSERT INTO media_links(media_id,owner_type,owner_id,role,position,created_at,updated_at) VALUES(?,'post','linked','post-image',?,'2026-10-10T00:00:00Z','2026-10-10T00:00:00Z')`, sha256Hex(image(name)), position)
	}
	store, err := OpenReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	page, err := store.ListAdminPosts(context.Background(), AdminPostFilter{})
	if err != nil {
		t.Fatal(err)
	}
	// Each image is "status url"; only a ready cached file has a URL.
	want := map[string]struct {
		status string
		images []string
	}{
		"linked":  {"ready", []string{"ready /media/" + sha256Hex(image("first")) + "?v=" + hash, "pending "}},
		"pending": {"pending", []string{"pending "}},
		"text":    {"none", []string{}},
	}
	if len(page.Items) != len(want) {
		t.Fatalf("items = %d, want %d", len(page.Items), len(want))
	}
	for _, post := range page.Items {
		w := want[post.ID]
		images := []string{}
		for _, media := range post.Media {
			images = append(images, media.Status+" "+stringValue(media.URL))
		}
		if post.MediaStatus != w.status || strings.Join(images, "|") != strings.Join(w.images, "|") {
			t.Fatalf("%s media = %s %q, want %s %q", post.ID, post.MediaStatus, images, w.status, w.images)
		}
	}

	// The account filter ignores case and a leading @.
	page, err = store.ListAdminPosts(context.Background(), AdminPostFilter{Account: "@KANO_2525"})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 2 || page.Items[0].ID != "linked" || page.Items[1].ID != "pending" {
		t.Fatalf("account filter = %d %+v", page.Total, page.Items)
	}
}

func TestAdminPostsShowFailureBackoff(t *testing.T) {
	path := testutil.NewSnapshot(t)
	for _, id := range []string{"waiting", "stopped", "requested", "fresh"} {
		testutil.Exec(t, path, `INSERT INTO posts (id,source,type,text,published_at,url) VALUES (?,'x','post','text','2026-10-11T00:00:00.000Z',?)`, id, "https://x.example.invalid/"+id)
	}
	testutil.Exec(t, path, `INSERT INTO post_llm_states (post_id,status,failure_count,failure_fingerprint,next_attempt_at,reprocess_requested,updated_at) VALUES
		('waiting','failed',2,'synthetic-input','2026-10-12T06:00:00.000Z',0,'2026-10-12T00:00:00.000Z'),
		('stopped','failed',4,'synthetic-input',NULL,0,'2026-10-12T00:00:00.000Z'),
		('requested','failed',4,'synthetic-input',NULL,1,'2026-10-12T00:00:00.000Z'),
		('fresh','failed',0,NULL,NULL,0,'2026-10-12T00:00:00.000Z')`)
	store, err := OpenReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	page, err := store.ListAdminPosts(context.Background(), AdminPostFilter{})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, item := range page.Items {
		retryAt := "-"
		if item.LLMRetryAt != nil {
			retryAt = *item.LLMRetryAt
		}
		got[item.ID] = retryAt + "|" + map[bool]string{true: "stopped", false: "retrying"}[item.LLMRetryStopped]
	}
	// A queued operator request runs at once, so it shows no backoff.
	for id, want := range map[string]string{
		"waiting":   "2026-10-12T06:00:00.000Z|retrying",
		"stopped":   "-|stopped",
		"requested": "-|retrying",
		"fresh":     "-|retrying",
	} {
		if got[id] != want {
			t.Errorf("%s = %q, want %q", id, got[id], want)
		}
	}
}

func TestAdminPostsSplitExtractionAndTranslationFailures(t *testing.T) {
	ctx := context.Background()
	store, err := OpenInitialized(filepath.Join(t.TempDir(), "database", "kano.sqlite"), true, true)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := store.DB().Exec(query, args...); err != nil {
			t.Fatal(err)
		}
	}
	stamp := "2026-10-11T00:00:00.000Z"
	for _, id := range []string{"extract", "translate", "both", "stale", "dismissed"} {
		exec(`INSERT INTO posts (id,source,type,text,published_at,url) VALUES (?,'x','post',?,?,?)`, id, "wording "+id, stamp, "https://x.example.invalid/"+id)
	}
	exec(`INSERT INTO post_llm_states (post_id,status,failure_count,failure_fingerprint,next_attempt_at,updated_at) VALUES
		('extract','failed',1,'synthetic-input','2026-10-12T00:00:00.000Z',?),
		('both','failed',1,'synthetic-input','2026-10-12T00:00:00.000Z',?),
		('dismissed','dismissed',2,'synthetic-input',NULL,?)`, stamp, stamp, stamp)
	// A failure of wording that has since changed is not a current failure.
	for id, wording := range map[string]string{"translate": "wording translate", "both": "wording both", "stale": "older wording"} {
		exec(`INSERT INTO translation_states (owner_type,owner_id,field,source_hash,status,last_error,updated_at) VALUES ('post',?,'text',?,'failed','LLM output was truncated',?)`, id, TranslationHash(wording), stamp)
	}

	counts, err := store.AdminCounts(ctx, "2026-10-11")
	if err != nil {
		t.Fatal(err)
	}
	if counts.FailedPosts != 3 || counts.FailedExtractions != 2 || counts.FailedTranslations != 2 {
		t.Fatalf("counts=%+v", counts)
	}
	ids := func(filter AdminPostFilter) string {
		t.Helper()
		page, err := store.ListAdminPosts(ctx, filter)
		if err != nil {
			t.Fatal(err)
		}
		out := []string{}
		for _, item := range page.Items {
			out = append(out, item.ID)
		}
		return strings.Join(out, ",")
	}
	if got := ids(AdminPostFilter{Translation: "failed"}); got != "both,translate" {
		t.Fatalf("translation failures=%s", got)
	}
	if got := ids(AdminPostFilter{LLM: "failed"}); got != "both,extract" {
		t.Fatalf("extraction failures=%s", got)
	}
	page, err := store.ListAdminPosts(ctx, AdminPostFilter{LLM: "dismissed"})
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("dismissed=%v err=%v", page.Items, err)
	}
	// A dismissed failure is not waiting for a retry.
	if item := page.Items[0]; item.LLMRetryStopped || item.LLMRetryAt != nil {
		t.Fatalf("dismissed post shows a backoff: %+v", item)
	}
}

func TestAdminPostsReportTheirScheduleImage(t *testing.T) {
	ctx := context.Background()
	store, err := OpenInitialized(filepath.Join(t.TempDir(), "database", "kano.sqlite"), true, true)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := store.DB().Exec(query, args...); err != nil {
			t.Fatal(err)
		}
	}
	stamp := "2026-10-11T00:00:00.000Z"
	image := func(name string) string { return "https://media.example.test/" + name + ".jpg" }
	exec(`INSERT INTO posts (id,source,type,text,published_at,url) VALUES ('board','x','post','text',?,'https://x.example.invalid/board'),('plain','x','post','text',?,'https://x.example.invalid/plain')`, stamp, stamp)
	for position, name := range []string{"first", "second"} {
		exec(`INSERT INTO media_assets(id,source,source_url,status,created_at,updated_at) VALUES(?,'x',?,'pending',?,?)`, sha256Hex(image(name)), image(name), stamp, stamp)
		exec(`INSERT INTO media_links(media_id,owner_type,owner_id,role,position,created_at,updated_at) VALUES(?,'post','board','post-image',?,?,?)`, sha256Hex(image(name)), position, stamp, stamp)
	}
	exec(`INSERT INTO assets(id,kind,url,source_url,updated_at) VALUES('schedule-x-board','schedule',?,'https://x.example.invalid/board',?)`, image("second"), stamp)
	exec(`INSERT INTO schedule_asset_reviews(asset_id,llm_status,manual_status,updated_at) VALUES('schedule-x-board','uncertain','unreviewed',?)`, stamp)

	record := func(id string) *AdminPostSchedule {
		t.Helper()
		page, err := store.ListAdminPosts(ctx, AdminPostFilter{})
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range page.Items {
			if item.ID == id {
				return item.ScheduleImage
			}
		}
		t.Fatalf("%s not listed", id)
		return nil
	}
	if plain := record("plain"); plain != nil {
		t.Fatalf("a post without a schedule asset reports one: %+v", plain)
	}
	board := record("board")
	if board == nil || board.Index == nil || *board.Index != 1 || board.Picked || board.LLMStatus != "uncertain" || board.ManualStatus != "unreviewed" {
		t.Fatalf("unexpected record: %+v", board)
	}
	if _, err = store.PickPostScheduleImage(ctx, SchedulePick{PostID: "board", Index: 0, Reason: "first image"}, stamp); err != nil {
		t.Fatal(err)
	}
	board = record("board")
	if board.Index == nil || *board.Index != 0 || !board.Picked || board.ManualStatus != "schedule" || board.LLMStatus != "pending" {
		t.Fatalf("unexpected picked record: %+v", board)
	}
	if _, err = store.PickPostScheduleImage(ctx, SchedulePick{PostID: "plain", Index: 0, Reason: "x"}, stamp); !errors.Is(err, ErrSchedulePickImage) {
		t.Fatalf("a post without images: %v", err)
	}
}
