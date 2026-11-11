package adminapi

import (
	"net/http"

	"github.com/yexca/kano-navi/backend/internal/httpx"
	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
)

// worksCovers reports how the Works page's upload covers stand: the counts
// and every upload still without a cover, with its videos' states. It reads
// the snapshot only; the media step makes the lookups and downloads.
// skipped says SKIP_MEDIA keeps that step from running in this process.
func (a *api) worksCovers(w http.ResponseWriter, r *http.Request) {
	if a.Works == nil {
		httpx.WriteCode(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	now := a.Now()
	report, err := a.Store.WorkCoverReport(r.Context(), *a.Works, now)
	if err != nil {
		httpx.WriteCode(w, http.StatusInternalServerError, "works_covers_failed")
		return
	}
	uploads := make([]map[string]any, 0, len(report.Uploads))
	for _, upload := range report.Uploads {
		videos := make([]map[string]any, 0, len(upload.Videos))
		for _, video := range upload.Videos {
			videos = append(videos, map[string]any{
				"platform": video.Platform, "id": video.ID, "members": video.Members,
				"state": video.State, "code": video.Code, "retryAt": video.RetryAt,
			})
		}
		uploads = append(uploads, map[string]any{"id": upload.ID, "date": upload.Date, "state": upload.State, "videos": videos})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"total": report.Total, "covered": report.Covered, "due": report.Due,
		"waiting": report.Waiting, "uncovered": report.Uncovered,
		"uploads": uploads, "skipped": a.skipped("media"),
		"checkedAt": sqlite.ISOTime(now),
	})
}
