package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/yexca/kano-navi/backend/internal/lineblog"
)

// lineblogArchiveData is the packaged LINE BLOG archive; tests replace it.
var lineblogArchiveData = lineblog.Packaged

const lineblogPostsPrefix = "/api/lineblog/posts/"

// lineblogRoute splits /api/lineblog/posts/{id} and its /comments. Any other
// path below the prefix, with an empty, further or unknown segment, is not a
// route.
func lineblogRoute(path string) (id string, comments bool, ok bool) {
	rest, found := strings.CutPrefix(path, lineblogPostsPrefix)
	if !found {
		return "", false, false
	}
	id, tail, nested := strings.Cut(rest, "/")
	switch {
	case id == "":
		return "", false, false
	case !nested:
		return id, false, true
	case tail == "comments":
		return id, true, true
	}
	return "", false, false
}

// lineblogIndex serves the archive's article list and months.
func (h *Handler) lineblogIndex(w http.ResponseWriter, r *http.Request) {
	h.writeArchiveJSON(w, r, lineblogArchiveData().Index)
}

// writeArchiveJSON answers a packaged LINE BLOG read with its media on the
// configured CDN, if any; the ETag is computed over the body as sent.
func (h *Handler) writeArchiveJSON(w http.ResponseWriter, r *http.Request, body json.RawMessage) {
	rewritten, err := h.publicJSON(body)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"}, "")
		return
	}
	writeRevalidatedJSON(w, r, json.RawMessage(rewritten), nil)
}

// lineblogPost serves one archived article or its comments.
func (h *Handler) lineblogPost(w http.ResponseWriter, r *http.Request) {
	id, comments, ok := lineblogRoute(r.URL.Path)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "api_not_found"}, "")
		return
	}
	w.Header().Set("Vary", "Accept-Encoding")
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, http.MethodGet)
		return
	}
	article, known := lineblog.Article{}, false
	if lineblog.ValidID(id) {
		article, known = lineblogArchiveData().Article(id)
	}
	if !known {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"}, "")
		return
	}
	if comments {
		h.writeArchiveJSON(w, r, article.Comments)
		return
	}
	h.writeArchiveJSON(w, r, article.Post)
}
