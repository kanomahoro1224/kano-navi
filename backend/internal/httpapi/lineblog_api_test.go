package httpapi

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/yexca/kano-navi/backend/internal/lineblog"
)

// syntheticLineblog is a two-article archive; the newer one has enough
// comment text to cross the compression threshold.
func syntheticLineblog(t *testing.T) {
	t.Helper()
	comments := make([]map[string]any, 0, 12)
	for number := 12; number >= 1; number-- {
		comments = append(comments, map[string]any{
			"number": number, "author": "Reader", "posted": "2019-01-02T03:04:00.000Z",
			"body": strings.Repeat("synthetic ", 12), "rating": nil,
		})
	}
	comments[0]["rating"] = map[string]any{"icon": "star", "value": 5}
	content, err := json.Marshal(map[string]any{
		"updatedAt": "2019-01-02", "originalBlogUrl": "https://blog.example.test/",
		"posts": []any{
			map[string]any{"id": "200", "published": "2019-01-01T00:00:00.000Z", "title": map[string]any{"ja": "新", "zh-CN": nil, "en": "New"}, "thumbnail": nil, "commentCount": 12},
			map[string]any{"id": "100", "published": "2018-12-01T00:00:00.000Z", "title": map[string]any{"ja": "旧", "zh-CN": "旧", "en": nil}, "thumbnail": nil, "commentCount": 0},
		},
		"months": []any{map[string]any{"month": "2019-01", "total": 1}, map[string]any{"month": "2018-12", "total": 1}},
		"articles": []any{
			map[string]any{"id": "200", "previousId": "100", "nextId": nil, "comments": comments, "post": map[string]any{
				"id": "200", "url": "https://blog.example.test/archives/200.html", "archiveUrl": "https://archive.example.test/200",
				"blocks": []any{
					map[string]any{"type": "paragraph", "content": []any{map[string]any{"type": "text", "text": "本文"}},
						"translations": map[string]any{"zh-CN": nil, "en": []any{[]any{map[string]any{"type": "text", "text": "Body"}}}}},
					map[string]any{"type": "rule"},
				},
			}},
			map[string]any{"id": "100", "previousId": nil, "nextId": "200", "comments": []any{}, "post": map[string]any{
				"id": "100", "blocks": []any{},
			}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	archive := lineblog.Parse(content)
	previous := lineblogArchiveData
	lineblogArchiveData = func() lineblog.Archive { return archive }
	t.Cleanup(func() { lineblogArchiveData = previous })
}

func TestLineblogReadsServeThePackagedShapes(t *testing.T) {
	syntheticLineblog(t)
	store, _ := openFixture(t)
	handler := NewHandler(store)

	index := request(t, handler, http.MethodGet, "/api/lineblog")
	if index.Code != http.StatusOK || index.Header().Get("Cache-Control") != "no-cache" || !strings.HasPrefix(index.Header().Get("ETag"), `W/"`) ||
		index.Header().Get("Vary") != "Accept-Encoding" || index.Header().Get("Content-Type") != "application/json; charset=utf-8" {
		t.Fatalf("index: status=%d headers=%v", index.Code, index.Header())
	}
	payload := decodeObject(t, index)
	posts := payload["posts"].([]any)
	if payload["updatedAt"] != "2019-01-02" || payload["originalBlogUrl"] != "https://blog.example.test/" || len(posts) != 2 ||
		posts[0].(map[string]any)["id"] != "200" || len(payload["months"].([]any)) != 2 {
		t.Fatalf("index=%v", payload)
	}
	for _, key := range []string{"revision", "articles"} {
		if _, ok := payload[key]; ok {
			t.Fatalf("index carries %q: %v", key, payload)
		}
	}

	post := request(t, handler, http.MethodGet, "/api/lineblog/posts/200")
	if post.Code != http.StatusOK || post.Header().Get("Cache-Control") != "no-cache" || post.Header().Get("ETag") == index.Header().Get("ETag") {
		t.Fatalf("post: status=%d headers=%v", post.Code, post.Header())
	}
	article := decodeObject(t, post)
	body := article["post"].(map[string]any)
	blocks := body["blocks"].([]any)
	translations := blocks[0].(map[string]any)["translations"].(map[string]any)
	if article["updatedAt"] != "2019-01-02" || article["previousId"] != "100" || article["nextId"] != nil || body["id"] != "200" ||
		len(blocks) != 2 || translations["zh-CN"] != nil || len(translations["en"].([]any)) != 1 {
		t.Fatalf("post=%v", article)
	}
	if _, ok := article["comments"]; ok {
		t.Fatalf("the article leaves its comments to their own read: %v", article)
	}

	comments := decodeObject(t, request(t, handler, http.MethodGet, "/api/lineblog/posts/200/comments"))
	list := comments["comments"].([]any)
	first := list[0].(map[string]any)
	if comments["updatedAt"] != "2019-01-02" || len(list) != 12 || first["number"] != float64(12) ||
		first["rating"].(map[string]any)["icon"] != "star" || list[1].(map[string]any)["rating"] != nil {
		t.Fatalf("comments=%v", comments)
	}

	// A known article without comments has an empty list, not a 404.
	none := request(t, handler, http.MethodGet, "/api/lineblog/posts/100/comments")
	if none.Code != http.StatusOK || len(decodeObject(t, none)["comments"].([]any)) != 0 {
		t.Fatalf("empty comments: status=%d body=%s", none.Code, none.Body)
	}
	oldest := decodeObject(t, request(t, handler, http.MethodGet, "/api/lineblog/posts/100"))
	if oldest["previousId"] != nil || oldest["nextId"] != "200" {
		t.Fatalf("oldest=%v", oldest)
	}
}

func TestLineblogPostPathsRejectUnknownIDsAndOtherShapes(t *testing.T) {
	syntheticLineblog(t)
	store, _ := openFixture(t)
	handler := NewHandler(store)
	for _, path := range []string{
		"/api/lineblog/posts/300", "/api/lineblog/posts/300/comments", "/api/lineblog/posts/abc",
		"/api/lineblog/posts/..", "/api/lineblog/posts/0200x", "/api/lineblog/posts/-1",
		"/api/lineblog/posts/1234567890123", "/api/lineblog/posts/%20200",
	} {
		recorder := request(t, handler, http.MethodGet, path)
		if recorder.Code != http.StatusNotFound || decodeObject(t, recorder)["error"] != "not_found" {
			t.Fatalf("%s: status=%d body=%s", path, recorder.Code, recorder.Body)
		}
	}
	for _, path := range []string{
		"/api/lineblog/posts/", "/api/lineblog/posts", "/api/lineblog/posts/200/", "/api/lineblog/posts/200/comments/",
		"/api/lineblog/posts/200/other", "/api/lineblog/posts/200/comments/1", "/api/lineblog/posts//200",
		"/api/lineblog/posts/../../200", "/api/lineblog/posts/..%2F..%2Fetc", "/api/lineblog/", "/api/lineblog/months",
	} {
		recorder := request(t, handler, http.MethodGet, path)
		if recorder.Code != http.StatusNotFound || decodeObject(t, recorder)["error"] != "api_not_found" {
			t.Fatalf("%s: status=%d body=%s", path, recorder.Code, recorder.Body)
		}
	}
	for _, path := range []string{"/api/lineblog", "/api/lineblog/posts/200", "/api/lineblog/posts/200/comments", "/api/lineblog/posts/300"} {
		for _, method := range []string{http.MethodHead, http.MethodPost, http.MethodDelete} {
			recorder := request(t, handler, method, path)
			if recorder.Code != http.StatusMethodNotAllowed || recorder.Header().Get("Allow") != http.MethodGet {
				t.Fatalf("%s %s: status=%d headers=%v", method, path, recorder.Code, recorder.Header())
			}
		}
	}
}

func TestLineblogReadsRevalidateAcrossEncodings(t *testing.T) {
	syntheticLineblog(t)
	store, _ := openFixture(t)
	server := httptest.NewServer(Compress(NewHandler(store)))
	defer server.Close()
	client := &http.Client{Transport: &http.Transport{DisableCompression: true}}
	get := func(path, encoding, ifNoneMatch string) (*http.Response, []byte) {
		t.Helper()
		req, err := http.NewRequest(http.MethodGet, server.URL+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		if encoding != "" {
			req.Header.Set("Accept-Encoding", encoding)
		}
		if ifNoneMatch != "" {
			req.Header.Set("If-None-Match", ifNoneMatch)
		}
		response, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		body, _ := io.ReadAll(response.Body)
		return response, body
	}

	identity, plain := get("/api/lineblog/posts/200/comments", "", "")
	etag := identity.Header.Get("ETag")
	if identity.StatusCode != http.StatusOK || identity.Header.Get("Content-Encoding") != "" || len(plain) < compressThreshold {
		t.Fatalf("identity: status=%d headers=%v size=%d", identity.StatusCode, identity.Header, len(plain))
	}
	gzipped, compressed := get("/api/lineblog/posts/200/comments", "gzip", "")
	if gzipped.Header.Get("Content-Encoding") != "gzip" || gzipped.Header.Get("ETag") != etag || gzipped.Header.Get("Vary") != "Accept-Encoding" {
		t.Fatalf("gzip: headers=%v", gzipped.Header)
	}
	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		t.Fatal(err)
	}
	decoded, _ := io.ReadAll(reader)
	if !bytes.Equal(decoded, plain) {
		t.Fatal("the gzip body differs from the identity body")
	}
	for _, candidate := range []string{etag, strings.TrimPrefix(etag, "W/"), `"other", ` + etag} {
		unchanged, body := get("/api/lineblog/posts/200/comments", "gzip", candidate)
		if unchanged.StatusCode != http.StatusNotModified || len(body) != 0 || unchanged.Header.Get("ETag") != etag {
			t.Fatalf("If-None-Match %s: status=%d headers=%v", candidate, unchanged.StatusCode, unchanged.Header)
		}
	}
	for _, path := range []string{"/api/lineblog", "/api/lineblog/posts/200"} {
		first, _ := get(path, "", "")
		again, body := get(path, "", first.Header.Get("ETag"))
		if first.StatusCode != http.StatusOK || again.StatusCode != http.StatusNotModified || len(body) != 0 {
			t.Fatalf("%s: status=%d then %d", path, first.StatusCode, again.StatusCode)
		}
	}
	// Errors stay uncompressed.
	missing, body := get("/api/lineblog/posts/300", "gzip", "")
	if missing.StatusCode != http.StatusNotFound || missing.Header.Get("Content-Encoding") != "" || !strings.Contains(string(body), "not_found") {
		t.Fatalf("missing: status=%d headers=%v", missing.StatusCode, missing.Header)
	}
}

func TestLineblogPackagedArchiveIsServed(t *testing.T) {
	store, _ := openFixture(t)
	handler := NewHandler(store)
	index := request(t, handler, http.MethodGet, "/api/lineblog")
	var payload struct {
		UpdatedAt string `json:"updatedAt"`
		Posts     []struct {
			ID string `json:"id"`
		} `json:"posts"`
	}
	if err := json.Unmarshal(index.Body.Bytes(), &payload); err != nil || payload.UpdatedAt == "" || len(payload.Posts) == 0 {
		t.Fatalf("index: status=%d err=%v", index.Code, err)
	}
	newest := payload.Posts[0].ID
	for _, path := range []string{"/api/lineblog/posts/" + newest, "/api/lineblog/posts/" + newest + "/comments"} {
		if recorder := request(t, handler, http.MethodGet, path); recorder.Code != http.StatusOK {
			t.Fatalf("%s: status=%d", path, recorder.Code)
		}
	}
}
