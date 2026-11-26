package schedule

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/yexca/kano-navi/backend/internal/digest"
	"github.com/yexca/kano-navi/backend/internal/jsvalue"
	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
)

// The candidate phase benchmarks run a scan over a synthetic snapshot of
// 10,000 candidate posts, one in four with a ready cached image, each with a
// stored success for its current input, so every post is a cache hit and no
// model is called.
//
//   - Settled: every post's state records its result and stamp, the scan
//     after an upgraded snapshot's first run.
//   - Judged: every post is loaded and judged, as when the extractor version,
//     the keywords or a detection stage change, and as the preview does.
//   - Preview: the queue preview of the same snapshot, judging every post.
//
// Run from backend/:
//
//	go test ./internal/schedule -run '^$' -bench BenchmarkScheduleScanSynthetic10k -benchtime 3x

const syntheticScanPosts = 10_000

func BenchmarkScheduleScanSynthetic10kSettled(b *testing.B) {
	run := openSyntheticScan(b, syntheticScanPosts)
	benchmarkScan(b, run)
}

func BenchmarkScheduleScanSynthetic10kJudged(b *testing.B) {
	run := openSyntheticScan(b, syntheticScanPosts)
	run.ignoreSettled = true
	benchmarkScan(b, run)
}

func BenchmarkScheduleScanSynthetic10kPreview(b *testing.B) {
	run := openSyntheticScan(b, syntheticScanPosts)
	run.ignoreSettled = true
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		preview, err := run.PreviewQueue(ctx)
		if err != nil {
			b.Fatal(err)
		}
		if preview.Cached != syntheticScanPosts || len(preview.Run) != 0 {
			b.Fatalf("preview cached=%d run=%d", preview.Cached, len(preview.Run))
		}
	}
}

func benchmarkScan(b *testing.B, run *Extractor) {
	b.Helper()
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result, err := run.Run(ctx)
		if err != nil {
			b.Fatal(err)
		}
		if countOf(result.Counts, "cached") != syntheticScanPosts || countOf(result.Counts, "attempted") != 0 {
			b.Fatalf("counts %v", result.Counts)
		}
	}
}

// syntheticSourceRecord is a stored source record of about the size a
// fetched status has (1.4 KB on average), most of it nothing the extractor
// reads.
func syntheticSourceRecord(id string, i int) *jsvalue.Object {
	entities := []any{}
	for j := 0; j < 6; j++ {
		entities = append(entities, jsvalue.Obj("start", j*8, "end", j*8+6, "text", fmt.Sprintf("synthetic-tag-%d", j),
			"url", fmt.Sprintf("https://synthetic.example.test/%s/%d", id, j)))
	}
	return jsvalue.Obj("id", id, "text", "synthetic padding "+id, "lang", "ja", "search_text", "synthetic search text "+id,
		"author", jsvalue.Obj("handle", "synthetic_handle", "name", "Synthetic Account", "avatar", "https://synthetic.example.test/avatar.png",
			"description", "a synthetic profile description that pads the record like a real author block does"),
		"metrics", jsvalue.Obj("likes", i, "reposts", i%7, "replies", i%11, "quotes", i%3, "bookmarks", i%5, "views", i*13),
		"entities", entities)
}

// openSyntheticScan writes the synthetic snapshot and runs one scan over it,
// which records every cache hit and its stamp.
func openSyntheticScan(b *testing.B, n int) *Extractor {
	b.Helper()
	store, err := sqlite.OpenInitialized(filepath.Join(b.TempDir(), "database", "kano.sqlite"), true, true)
	if err != nil {
		b.Fatalf("open store: %v", err)
	}
	b.Cleanup(func() { _ = store.Close() })
	db := store.DB()
	ctx := context.Background()
	exec := func(query string, args ...any) {
		if _, err := db.ExecContext(ctx, query, args...); err != nil {
			b.Fatalf("exec %q: %v", query, err)
		}
	}
	exec(`PRAGMA synchronous = OFF`)
	exec(`INSERT INTO app_settings (key, value, updated_at) VALUES ('sync_extraction_lookback_days', '0', ?)`, testTimestamp)
	exec(`BEGIN`)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("synthetic-%05d", i)
		publishedAt := sqlite.ISOTime(base.Add(time.Duration(i) * time.Minute))
		record := syntheticSourceRecord(id, i)
		raw := jsvalue.Stringify(record)
		if i%4 == 0 {
			imageURL := "https://pbs.twimg.com/media/synthetic-" + id + ".png"
			record.Set("media_urls", []any{imageURL})
			raw = jsvalue.Stringify(record)
			content := pngHeader + id
			hash := digest.SHA256Hex(content)
			relative := "x/sha256/" + hash[:2] + "/" + hash + ".png"
			target := filepath.Join(store.DataRoot(), filepath.FromSlash(relative))
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				b.Fatal(err)
			}
			if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
				b.Fatal(err)
			}
			mediaID := mediaIDForSourceURL(imageURL)
			exec(`INSERT INTO media_assets (id, source, source_url, cache_path, mime_type, extension, byte_size,
				sha256, status, created_at, updated_at) VALUES (?, 'x', ?, ?, 'image/png', 'png', ?, ?, 'ready', ?, ?)`,
				mediaID, imageURL, relative, len(content), hash, testTimestamp, testTimestamp)
			exec(`INSERT INTO media_links (media_id, owner_type, owner_id, role, position, created_at, updated_at)
				VALUES (?, 'post', ?, 'post-image', 0, ?, ?)`, mediaID, id, testTimestamp, testTimestamp)
		}
		exec(`INSERT INTO posts (id, source, account_handle, type, label, text, published_at, url, raw_json)
			VALUES (?, 'x', 'synthetic_handle', 'daily', '', ?, ?, ?, ?)`,
			id, fmt.Sprintf("明日 21時から配信 %d", i), publishedAt, "https://x.com/synthetic_handle/status/"+id, raw)
	}
	exec(`COMMIT`)

	run := New(store, Options{
		SecretsKey: testKey,
		HTTPClient: &http.Client{Transport: &fakeProvider{respond: func(recordedRequest) (int, string) {
			return http.StatusInternalServerError, `{}`
		}}},
		Getenv: func(string) string { return "" },
	})
	// A stored success for each candidate's current input.
	scan, err := run.buildQueue(ctx)
	if err != nil {
		b.Fatal(err)
	}
	if len(scan.queue) != n {
		b.Fatalf("candidates=%d, want %d", len(scan.queue), n)
	}
	exec(`BEGIN`)
	for _, item := range scan.queue {
		input, err := run.loadCandidate(ctx, liveReads{db, run.mediaRoot()}, item.post, item.detectionType, false)
		if err != nil {
			b.Fatal(err)
		}
		mode := inputModeOf(item.post, input.media)
		exec(`INSERT INTO schedule_extractions (source, source_item_id, content_fingerprint, extractor_version,
			model, status, result_json, created_at, updated_at) VALUES ('x', ?, ?, ?, 'model-a', 'success', '{}', ?, ?)`,
			item.post.id, fingerprintOf(item.post, item.detectionType, mode, input.media),
			extractorVersionPrefix(item.detectionType, mode)+"synthetic", testTimestamp, testTimestamp)
	}
	exec(`COMMIT`)
	result, err := run.Run(ctx)
	if err != nil {
		b.Fatal(err)
	}
	if countOf(result.Counts, "cached") != n {
		b.Fatalf("setup counts %v", result.Counts)
	}
	return run
}
