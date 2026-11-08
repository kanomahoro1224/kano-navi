package sqlite

import (
	"database/sql/driver"
	"path/filepath"
	"testing"
)

func TestScheduleInputStampCoversTheExtractionInput(t *testing.T) {
	store, err := OpenInitialized(filepath.Join(t.TempDir(), "database", "kano.sqlite"), true, true)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	db := store.DB()
	var current *testing.T
	exec := func(query string, args ...any) {
		t := current
		t.Helper()
		if _, err := db.Exec(query, args...); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
	}
	const at = "2026-05-01T00:00:00.000Z"
	reset := func() {
		exec(`DELETE FROM media_links`)
		exec(`DELETE FROM media_assets`)
		exec(`DELETE FROM posts`)
		exec(`INSERT INTO posts (id, source, account_handle, type, label, text, published_at, url, likes, media_url, raw_json, quote_json)
			VALUES ('post-1', 'x', 'synthetic_handle', 'daily', 'label', '明日 21時', ?, 'https://x.com/synthetic_handle/status/post-1', 1,
			'https://media.example.test/a.png', '{"likes":1,"mediaURLs":["https://media.example.test/a.png"]}',
			'{"authorHandle":"synthetic_other","authorName":"Other","text":"引用","publishedAt":"2026-04-30T00:00:00.000Z"}')`, at)
		for i, id := range []string{"media-a", "media-b"} {
			exec(`INSERT INTO media_assets (id, source, source_url, cache_path, mime_type, sha256, status, created_at, updated_at)
				VALUES (?, 'x', ?, ?, 'image/png', ?, 'ready', ?, ?)`, id, "https://media.example.test/"+id, "x/"+id+".png", "hash-"+id, at, at)
			exec(`INSERT INTO media_links (media_id, owner_type, owner_id, role, position, created_at, updated_at)
				VALUES (?, 'post', 'post-1', 'post-image', ?, ?, ?)`, id, i, at, at)
		}
	}
	stamp := func(detectionType, version string) string {
		t := current
		t.Helper()
		var value string
		if err := db.QueryRow(`SELECT `+ScheduleInputStampSQL("p")+` FROM posts p WHERE p.id = 'post-1'`, detectionType, version).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	current = t
	reset()
	base := stamp("message", "synthetic-v1")
	if len(base) != 64 || stamp("message", "synthetic-v1") != base {
		t.Fatalf("stamp %q is not a stable hash", base)
	}
	if stamp("board", "synthetic-v1") == base || stamp("message", "synthetic-v2") == base {
		t.Fatal("the detection type or version leaves the stamp alone")
	}
	for name, change := range map[string]string{
		"source":         `UPDATE posts SET source = 'bilibili'`,
		"text":           `UPDATE posts SET text = '明日 22時'`,
		"empty text":     `UPDATE posts SET text = ''`,
		"published at":   `UPDATE posts SET published_at = '2026-05-02T00:00:00.000Z'`,
		"url":            `UPDATE posts SET url = 'https://x.com/synthetic_handle/status/post-2'`,
		"media url":      `UPDATE posts SET media_url = NULL`,
		"declared list":  `UPDATE posts SET raw_json = json_set(raw_json, '$.mediaURLs', json('["https://media.example.test/b.png"]'))`,
		"media_urls":     `UPDATE posts SET raw_json = json_set(raw_json, '$.media_urls', json('[]'))`,
		"mediaUrls":      `UPDATE posts SET raw_json = json_set(raw_json, '$.mediaUrls', 'one')`,
		"later spelling": `UPDATE posts SET raw_json = '{"likes":1,"mediaURLs":["https://media.example.test/a.png"],"mediaURLs":[]}'`,
		"invalid raw":    `UPDATE posts SET raw_json = '{"mediaURLs":'`,
		"raw array":      `UPDATE posts SET raw_json = '["https://media.example.test/a.png"]'`,
		"no raw":         `UPDATE posts SET raw_json = NULL`,
		"quote author":   `UPDATE posts SET quote_json = json_set(quote_json, '$.authorHandle', 'synthetic_third')`,
		"quote name":     `UPDATE posts SET quote_json = json_set(quote_json, '$.authorName', 'Third')`,
		"quote text":     `UPDATE posts SET quote_json = json_set(quote_json, '$.text', '別の引用')`,
		"unavailable":    `UPDATE posts SET quote_json = json_set(quote_json, '$.unavailable', json('true'))`,
		"invalid quote":  `UPDATE posts SET quote_json = '{"text":'`,
		"hidden quote":   `UPDATE posts SET quote_hidden_at = '2026-05-01T00:00:00.000Z'`,
		"image order":    `UPDATE media_links SET position = 5 WHERE media_id = 'media-a'`,
		"image removed":  `DELETE FROM media_links WHERE media_id = 'media-b'`,
		"image status":   `UPDATE media_assets SET status = 'pending' WHERE id = 'media-a'`,
		"image hash":     `UPDATE media_assets SET sha256 = 'hash-other' WHERE id = 'media-a'`,
		"image type":     `UPDATE media_assets SET mime_type = 'image/jpeg' WHERE id = 'media-a'`,
		"image path":     `UPDATE media_assets SET cache_path = 'x/other.png' WHERE id = 'media-a'`,
		"asset missing":  `DELETE FROM media_assets WHERE id = 'media-a'`,
		"quote image":    `UPDATE media_links SET role = 'quote-image' WHERE media_id = 'media-b'`,
	} {
		t.Run(name, func(t *testing.T) {
			current = t
			reset()
			exec(change)
			if stamp("message", "synthetic-v1") == base {
				t.Fatal("the stamp did not change")
			}
		})
	}
	// What the extractor does not read leaves the stamp alone.
	for name, change := range map[string]string{
		"label":            `UPDATE posts SET label = 'other label'`,
		"counters":         `UPDATE posts SET likes = 99, raw_json = json_set(raw_json, '$.likes', 99)`,
		"raw member order": `UPDATE posts SET raw_json = '{"mediaURLs":["https://media.example.test/a.png"],"likes":1}'`,
		"quote extras":     `UPDATE posts SET quote_json = json_set(quote_json, '$.publishedAt', '2026-04-29T00:00:00.000Z')`,
		// The extractor's decoder matches keys without case.
		"quote key case":   `UPDATE posts SET quote_json = '{"authorhandle":"synthetic_other","AuthorName":"Other","text":"引用"}'`,
		"hidden post":      `UPDATE posts SET hidden_at = '2026-05-01T00:00:00.000Z'`,
		"asset source url": `UPDATE media_assets SET source_url = 'https://media.example.test/elsewhere' WHERE id = 'media-a'`,
	} {
		t.Run(name, func(t *testing.T) {
			current = t
			reset()
			exec(change)
			if stamp("message", "synthetic-v1") != base {
				t.Fatal("the stamp changed")
			}
		})
	}
}

func TestScheduleInputStampKeepsArgumentsApart(t *testing.T) {
	args := func(values ...driver.Value) []driver.Value {
		out := make([]driver.Value, scheduleStampArgs)
		copy(out, values)
		return out
	}
	stamp := func(values []driver.Value) string {
		t.Helper()
		value, err := scheduleInputStamp(values)
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	seen := map[string]string{}
	for name, values := range map[string][]driver.Value{
		"ab|c":      args("ab", "c"),
		"a|bc":      args("a", "bc"),
		"empty":     args("", ""),
		"null":      args(nil, nil),
		"bytes":     args([]byte("x"), "c"),
		"string":    args("y", "c"),
		"integer":   args(int64(1), "c"),
		"float":     args(float64(1), "c"),
		"raw null":  args("message", "v", "x", "t", nil, nil, nil, "null"),
		"raw array": args("message", "v", "x", "t", nil, nil, nil, "[]"),
	} {
		got := stamp(values)
		if other, dup := seen[got]; dup {
			t.Errorf("%s and %s share a stamp", name, other)
		}
		seen[got] = name
	}
	if _, err := scheduleInputStamp(args()[:3]); err == nil {
		t.Error("a short argument list was accepted")
	}
}
