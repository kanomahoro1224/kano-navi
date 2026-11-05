package httpapi

import (
	"errors"
	"net/http"

	"github.com/yexca/kano-navi/backend/internal/publicview"
	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
)

const archivePageSize = 30

// archiveNumber reads a positive page or page size; anything else falls back.
func archiveNumber(r *http.Request, key string, fallback int) int {
	value, ok := parseInt10(r.URL.Query().Get(key))
	if !ok || value < 1 {
		return fallback
	}
	if value > 1<<20 {
		return 1 << 20
	}
	return int(value)
}

func writeArchiveError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, sqlite.ErrInvalidCalendarRange):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_range"}, "")
	case errors.Is(err, sqlite.ErrInvalidAccount):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_account"}, "")
	case errors.Is(err, sqlite.ErrInvalidVideoCategory):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_category"}, "")
	default:
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database_unavailable"}, "")
	}
}

func archivePayload[T any](page *sqlite.ArchivePage[T], project func(T) map[string]any) map[string]any {
	items := make([]any, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, project(item))
	}
	return map[string]any{
		"items": items, "page": page.Page, "pageSize": page.PageSize, "total": page.Total,
		"totalPages": page.TotalPages, "hasNext": page.HasNext, "revision": page.Revision,
		"adjacent": map[string]any{"previous": page.Previous, "next": page.Next},
	}
}

// posts serves the post archive: stored posts of an inclusive Japan-date
// range, newest first, optionally for one account.
func (h *Handler) posts(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	page, err := h.store.GetPostArchive(r.Context(), query.Get("from"), query.Get("to"), query.Get("account"),
		archiveNumber(r, "page", 1), archiveNumber(r, "pageSize", archivePageSize))
	if err != nil {
		writeArchiveError(w, err)
		return
	}
	payload := archivePayload(page, publicview.PublicPost)
	payload["accountCounts"] = page.AccountCounts
	writeRevalidatedJSON(w, r, payload, nil)
}

// postMonths serves the post archive's month index: every Japan month that
// holds a shown post, oldest first, with its per-account counts.
func (h *Handler) postMonths(w http.ResponseWriter, r *http.Request) {
	months, revision, err := h.store.GetPostMonths(r.Context())
	writeArchiveMonths(w, r, months, revision, err, "accountCounts")
}

// videoMonths serves the video timeline's month index: every Japan month that
// holds a public video, oldest first, with its per-category counts.
func (h *Handler) videoMonths(w http.ResponseWriter, r *http.Request) {
	months, revision, err := h.store.GetVideoMonths(r.Context())
	writeArchiveMonths(w, r, months, revision, err, "categoryCounts")
}

func writeArchiveMonths(w http.ResponseWriter, r *http.Request, months []sqlite.ArchiveMonth, revision int64, err error, countsKey string) {
	if err != nil {
		writeArchiveError(w, err)
		return
	}
	items := make([]any, 0, len(months))
	for _, month := range months {
		items = append(items, map[string]any{"month": month.Month, "total": month.Total, countsKey: month.Counts})
	}
	writeRevalidatedJSON(w, r, map[string]any{"months": items, "revision": revision}, nil)
}

// videos serves the video archive, newest first, optionally of one category:
// with from and to, the stored videos whose scheduled or published time falls
// in that inclusive Japan-date range; without them, the timeline of every
// dated video, or of those on or before the inclusive Japan date until.
func (h *Handler) videos(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	pageNumber, pageSize := archiveNumber(r, "page", 1), archiveNumber(r, "pageSize", archivePageSize)
	var page *sqlite.ArchivePage[sqlite.Video]
	var err error
	if query.Has("from") || query.Has("to") {
		if query.Has("until") {
			err = sqlite.ErrInvalidCalendarRange
		} else {
			page, err = h.store.GetVideoArchive(r.Context(), query.Get("from"), query.Get("to"), query.Get("category"), pageNumber, pageSize, h.now())
		}
	} else {
		page, err = h.store.GetVideoTimeline(r.Context(), query.Get("until"), query.Get("category"), pageNumber, pageSize, h.now())
	}
	if err != nil {
		writeArchiveError(w, err)
		return
	}
	payload := archivePayload(page, publicview.PublicVideo)
	payload["categoryCounts"] = page.CategoryCounts
	// The body includes the time-derived fields (isUpcoming, liveStatus), so
	// its ETag changes when they expire as well as with stored data.
	writeRevalidatedJSON(w, r, payload, nil)
}
