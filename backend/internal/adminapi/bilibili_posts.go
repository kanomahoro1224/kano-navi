package adminapi

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/yexca/kano-navi/backend/internal/httpx"
	"github.com/yexca/kano-navi/backend/internal/jobs"
	bilisource "github.com/yexca/kano-navi/backend/internal/source/bilibili"
	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
)

// addBilibiliPost accepts { input } with a bilibili.com/opus or
// t.bilibili.com dynamic link and stores that dynamic of the configured
// account, read from its opus page. It costs one Bilibili request (two when
// the device identity is stale), respects the captcha backoff and starts the
// media step for its images.
func (a *api) addBilibiliPost(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Body(w, r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	var input string
	_ = json.Unmarshal(body["input"], &input)
	id, ok := bilisource.ParseDynamicID(input)
	if !ok {
		httpx.WriteError(w, httpx.Invalid("enter a bilibili.com/opus or t.bilibili.com dynamic link"))
		return
	}
	if a.Bilibili == nil || a.skipped("bilibili") {
		httpx.WriteError(w, httpx.Invalid("the Bilibili source is disabled for this process"))
		return
	}
	ctx := r.Context()
	account := a.Bilibili.Options.Account
	state, err := a.Store.BilibiliState(ctx, account)
	if err != nil {
		httpx.WriteCode(w, http.StatusServiceUnavailable, "database_unavailable")
		return
	}
	force, err := forceFlag(body)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	dynamic, next, err := a.Bilibili.FetchDynamic(ctx, state, id, force)
	if err != nil {
		_ = a.Store.SaveBilibiliState(ctx, account, next)
		switch bilisource.ErrorCode(err) {
		case "bilibili_not_found":
			httpx.WriteJSON(w, http.StatusNotFound, map[string]any{"error": "post_not_found", "message": "Bilibili does not show this dynamic"})
		case "bilibili_other_account":
			httpx.WriteError(w, httpx.Invalid("this dynamic was not posted by the configured Bilibili account"))
		case "bilibili_unsupported_dynamic":
			httpx.WriteJSON(w, http.StatusBadRequest, map[string]any{"error": "bilibili_unsupported_dynamic", "message": "this dynamic is not a text or picture post"})
		default:
			writeBilibiliError(w, err, next)
		}
		return
	}
	if _, err = a.Store.WriteBilibiliDynamic(ctx, account, dynamic, next); err != nil {
		httpx.WriteCode(w, http.StatusServiceUnavailable, "database_unavailable")
		return
	}
	post, err := a.Store.AdminPostByID(ctx, dynamic.PostID())
	if err != nil {
		httpx.WriteCode(w, http.StatusServiceUnavailable, "database_unavailable")
		return
	}
	response := map[string]any{"post": post, "started": false, "run": nil}
	if a.Runs != nil && !a.skipped("media") {
		if result, startErr := a.Runs.Start(ctx, jobs.KindWorkflow, "admin", jobs.Options{Steps: []string{"media"}}); startErr == nil {
			response["started"], response["run"] = result.Accepted, result.Job
		}
	}
	httpx.WriteJSON(w, http.StatusOK, response)
}

// bilibiliLiveProbe is the Bilibili part of the live-status probe as the
// console sees it: its settings and bounds, whether this process checks the
// room, and the plan.
func (a *api) bilibiliLiveProbe(ctx context.Context, settings sqlite.LiveProbeSettings) (map[string]any, error) {
	out := map[string]any{
		"enabled": settings.BilibiliEnabled, "intervalMinutes": settings.BilibiliIntervalMinutes,
		"minIntervalMinutes": sqlite.BilibiliLiveMinMinutes, "maxIntervalMinutes": sqlite.BilibiliLiveMaxMinutes,
		"available": false, "nextCheckAt": nil, "targets": []any{},
	}
	if a.Bilibili == nil {
		return out, nil
	}
	available := !a.skipped("bilibili")
	if a.LiveProbe != nil {
		available = available && a.LiveProbe()["available"] == true
	}
	plan, err := a.Store.BilibiliLivePlan(ctx, a.Bilibili.Options.Account, a.Now(), settings)
	if err != nil {
		return nil, err
	}
	targets := make([]map[string]any, 0, len(plan.Targets))
	for _, target := range plan.Targets {
		targets = append(targets, map[string]any{
			"eventId": target.EventID, "title": target.Title, "startsAt": sqlite.ISOTime(target.StartsAt),
			"phase": target.Phase, "dueAt": sqlite.ISOTime(target.DueAt),
		})
	}
	out["available"], out["targets"] = available, targets
	if available && settings.BilibiliEnabled {
		next := plan.DueAt
		if next.Before(a.Now()) {
			next = a.Now()
		}
		out["nextCheckAt"] = sqlite.ISOTime(next)
	}
	return out, nil
}
