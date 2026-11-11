package adminapi

import (
	"encoding/json"
	"net/http"

	"github.com/yexca/kano-navi/backend/internal/httpx"
	"github.com/yexca/kano-navi/backend/internal/jobs"
	"github.com/yexca/kano-navi/backend/internal/source/history"
	ytsource "github.com/yexca/kano-navi/backend/internal/source/youtube"
)

// addYoutubeVideo accepts { input } with a YouTube video ID or link, reads
// that video of the configured channel now and stores it as a manual fetch
// stores a listed and checked video: the listing row, then the detail
// check through the recent step's upsert, so manual locks and deletion
// tombstones hold. A stored video is read again. It costs one Data API call
// with a key, or one watch page without, and starts the media step for the
// thumbnail.
func (a *api) addYoutubeVideo(w http.ResponseWriter, r *http.Request) {
	if a.Youtube == nil || a.skipped("youtube") {
		httpx.WriteError(w, httpx.Invalid("the YouTube source is disabled for this process"))
		return
	}
	body, err := httpx.Body(w, r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	for key := range body {
		if key != "input" {
			httpx.WriteError(w, httpx.Invalid("%s is not supported", key))
			return
		}
	}
	var input string
	_ = json.Unmarshal(body["input"], &input)
	id, ok := ytsource.ParseVideoID(input)
	if !ok {
		httpx.WriteError(w, httpx.Invalid("enter a YouTube video ID or link"))
		return
	}
	ctx := r.Context()
	stored, err := a.Store.StoredYoutubeVideos(ctx, []string{id})
	if err != nil {
		httpx.WriteCode(w, http.StatusServiceUnavailable, "database_unavailable")
		return
	}
	checked, localized, err := a.Youtube.ReadVideo(ctx, id, stored)
	if err != nil {
		writeYoutubeReadError(w, err)
		return
	}
	listed := ytsource.Video{ID: checked.ID, Title: checked.Title, PublishedAt: checked.PublishedAt, URL: checked.URL, ThumbnailURL: checked.ThumbnailURL, Kind: ytsource.KindUnknown}
	if _, err = a.Store.WriteHistoryPage(ctx, history.Page{
		Source: history.SourceYoutube, Account: a.Youtube.Options.ChannelID,
		Videos: []ytsource.Video{listed}, Described: []ytsource.Video{checked}, Localized: localized,
	}); err != nil {
		httpx.WriteCode(w, http.StatusServiceUnavailable, "database_unavailable")
		return
	}
	video, err := a.Store.AdminVideoByID(ctx, id)
	if err != nil {
		writeVideoError(w, err)
		return
	}
	response := map[string]any{"video": video, "started": false, "run": nil}
	if a.Runs != nil && !a.skipped("media") {
		if result, startErr := a.Runs.Start(ctx, jobs.KindWorkflow, "admin", jobs.Options{Steps: []string{"media"}}); startErr == nil {
			response["started"], response["run"] = result.Accepted, result.Job
		}
	}
	httpx.WriteJSON(w, http.StatusOK, response)
}

// writeYoutubeReadError answers a failed single read with its controlled
// code, never an upstream body.
func writeYoutubeReadError(w http.ResponseWriter, err error) {
	code := "youtube_unavailable"
	if e, ok := err.(*ytsource.Error); ok {
		code = e.Code
	}
	switch code {
	case ytsource.CodeOtherChannel:
		httpx.WriteError(w, httpx.Invalid("this video was not uploaded by the configured YouTube channel"))
	case ytsource.CodePrivate, ytsource.CodeMembers, ytsource.CodeLoginRequired, "youtube_watch_details_incomplete":
		httpx.WriteJSON(w, http.StatusNotFound, map[string]any{"error": "video_not_found", "code": code, "message": "YouTube does not show this video publicly (" + code + ")"})
	default:
		httpx.WriteJSON(w, http.StatusBadGateway, map[string]any{"error": code, "message": "YouTube did not answer (" + code + ")"})
	}
}
