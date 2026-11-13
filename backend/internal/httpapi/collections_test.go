package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPreparedJSONServesCompactBodyWithStableValidator(t *testing.T) {
	calls := 0
	body := &preparedJSON{source: func() []byte {
		calls++
		return []byte("{\n  \"updatedAt\": \"2026-01-02\",\n  \"items\": [\n    \"synthetic\"\n  ]\n}\n")
	}}
	first := httptest.NewRecorder()
	body.serve(first, httptest.NewRequest(http.MethodGet, "/api/synthetic", nil))
	etag := first.Header().Get("ETag")
	if first.Code != http.StatusOK || first.Body.String() != `{"updatedAt":"2026-01-02","items":["synthetic"]}` ||
		first.Header().Get("Cache-Control") != "no-cache" || !strings.HasPrefix(etag, `W/"`) ||
		first.Header().Get("Content-Type") != "application/json; charset=utf-8" {
		t.Fatalf("status=%d headers=%v body=%s", first.Code, first.Header(), first.Body)
	}
	if etag != revalidatedETag(first.Body.Bytes()) {
		t.Fatalf("the validator is not the public reads' one over the served body: %s", etag)
	}

	again := httptest.NewRequest(http.MethodGet, "/api/synthetic", nil)
	again.Header.Set("If-None-Match", `"other", `+strings.TrimPrefix(etag, "W/"))
	unchanged := httptest.NewRecorder()
	body.serve(unchanged, again)
	if unchanged.Code != http.StatusNotModified || unchanged.Body.Len() != 0 || unchanged.Header().Get("ETag") != etag {
		t.Fatalf("revalidation status=%d headers=%v", unchanged.Code, unchanged.Header())
	}
	if calls != 1 {
		t.Fatalf("the body was prepared %d times", calls)
	}

	broken := &preparedJSON{source: func() []byte { return []byte("{not json") }}
	failed := httptest.NewRecorder()
	broken.serve(failed, httptest.NewRequest(http.MethodGet, "/api/synthetic", nil))
	if failed.Code != http.StatusInternalServerError || failed.Header().Get("ETag") != "" || !strings.Contains(failed.Body.String(), "internal_error") {
		t.Fatalf("broken body status=%d headers=%v body=%s", failed.Code, failed.Header(), failed.Body)
	}
}

// The collections are GET reads of the build's own data: they revalidate and
// gzip like the other public reads, refuse other methods, and need no
// snapshot rows.
func TestCollectionRoutesServeTheShippedBodies(t *testing.T) {
	handler := Compress(archiveHandler(t, ""))
	for path, keys := range map[string][]string{
		"/api/journey":           {"updatedAt", "branches", "milestones", "galleryOnly"},
		"/api/works":             {"updatedAt", "summary", "personal", "participation"},
		"/api/works/upload-list": {"updatedAt", "uploads"},
	} {
		feedEncodings(t, handler, path)
		recorder := request(t, handler, http.MethodGet, path)
		payload := decodeObject(t, recorder)
		if len(payload) != len(keys) {
			t.Fatalf("%s keys=%v", path, payload)
		}
		for _, key := range keys {
			if _, ok := payload[key]; !ok {
				t.Fatalf("%s lacks %s", path, key)
			}
		}
		for _, method := range []string{http.MethodHead, http.MethodPost} {
			refused := request(t, handler, method, path)
			if refused.Code != http.StatusMethodNotAllowed || refused.Header().Get("Allow") != http.MethodGet {
				t.Fatalf("%s %s status=%d headers=%v", method, path, refused.Code, refused.Header())
			}
		}
	}
	for _, path := range []string{"/api/journey/", "/api/journey/first-covers", "/api/works/", "/api/works/upload-list/1"} {
		missing := request(t, handler, http.MethodGet, path)
		if missing.Code != http.StatusNotFound || !strings.Contains(missing.Body.String(), "api_not_found") {
			t.Fatalf("%s status=%d body=%s", path, missing.Code, missing.Body)
		}
	}
}
