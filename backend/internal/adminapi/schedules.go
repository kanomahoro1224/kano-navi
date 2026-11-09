package adminapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/yexca/kano-navi/backend/internal/httpx"
	"github.com/yexca/kano-navi/backend/internal/jobs"
	"github.com/yexca/kano-navi/backend/internal/source/history"
	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
)

// --- source schedules -------------------------------------------------------

// bilibiliFloor is BILIBILI_MIN_INTERVAL_MINUTES as the process reads it, or
// its default when this process has no Bilibili reader.
func (a *api) bilibiliFloor() int {
	if a.Bilibili == nil {
		return 180
	}
	return int(a.Bilibili.Options.MinInterval / time.Minute)
}

// sourceSchedules lists each source's timer for the console, with its bounds
// and whether SKIP_* disables it in this process.
func (a *api) sourceSchedules(ctx context.Context) ([]map[string]any, error) {
	schedules, err := a.Store.SourceSchedules(ctx, a.bilibiliFloor())
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(schedules))
	for _, schedule := range schedules {
		entry, err := a.scheduleEntry(ctx, schedule)
		if err != nil {
			return nil, err
		}
		out = append(out, entry)
	}
	return out, nil
}

func (a *api) scheduleEntry(ctx context.Context, schedule sqlite.SourceSchedule) (map[string]any, error) {
	low, high := sqlite.SourceScheduleBounds(schedule.Source, a.bilibiliFloor())
	entry := map[string]any{
		"source": schedule.Source, "enabled": schedule.Enabled, "intervalMinutes": schedule.IntervalMinutes,
		"minIntervalMinutes": low, "maxIntervalMinutes": high, "sync": schedule.Sync,
		"nextRunAt": schedule.NextRunAt, "lastRun": schedule.LastRun, "skipped": a.skipped(schedule.Source),
	}
	if a.skipped(schedule.Source) {
		entry["reason"] = "environment"
	}
	chain, err := a.chainEntry(ctx, schedule)
	if err != nil {
		return nil, err
	}
	entry["chain"] = chain
	return entry, nil
}

// chainEntry describes a schedule's method chain: each method in
// order with whether it is enabled, whether this process can run it and the
// variable it needs. X adds its API gate and cap, today's usage and each
// account's timeline failure streak. Bilibili's one method is fixed.
func (a *api) chainEntry(ctx context.Context, schedule sqlite.SourceSchedule) (map[string]any, error) {
	methods := []map[string]any{}
	for _, method := range schedule.Chain.Methods {
		item := map[string]any{"id": method.ID, "enabled": method.Enabled, "available": true}
		if method.ID == history.MethodAPI || method.ID == history.MethodYTDLP {
			if described, ok := history.Lookup(schedule.Source, method.ID); ok {
				if described.Requires != "" {
					item["requires"] = described.Requires
				}
			}
			item["available"] = a.methodAvailable(schedule.Source, method.ID)
		}
		methods = append(methods, item)
	}
	out := map[string]any{"methods": methods, "fixed": !slices.Contains(sqlite.ChainSources, schedule.Source)}
	if schedule.Source == "x" {
		state, err := a.Store.XAPIStatus(ctx, a.XHandles, a.Now())
		if err != nil {
			return nil, err
		}
		out["apiGate"], out["apiDailyCap"] = schedule.Chain.APIGate, schedule.Chain.APIDailyCap
		out["apiUsage"], out["timelineFailures"] = state.Usage, state.TimelineFailures
	}
	return out, nil
}

// updateSourceSchedule accepts any subset of { enabled, intervalMinutes,
// sync, chain }; omitted fields keep their value.
func (a *api) updateSourceSchedule(w http.ResponseWriter, r *http.Request) {
	source := r.PathValue("source")
	if !slices.Contains(sqlite.ScheduleSources, source) {
		httpx.WriteCode(w, http.StatusNotFound, "not_found")
		return
	}
	body, err := httpx.Body(w, r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	floor := a.bilibiliFloor()
	schedule, err := a.Store.SourceScheduleFor(r.Context(), source, floor)
	if err != nil {
		httpx.WriteCode(w, http.StatusServiceUnavailable, "database_unavailable")
		return
	}
	low, high := sqlite.SourceScheduleBounds(source, floor)
	for key, raw := range body {
		switch key {
		case "enabled":
			if json.Unmarshal(raw, &schedule.Enabled) != nil {
				httpx.WriteError(w, httpx.Invalid("enabled must be a boolean"))
				return
			}
		case "intervalMinutes":
			minutes, rangeErr := integer(raw, "intervalMinutes", low, high)
			if rangeErr != nil {
				httpx.WriteError(w, rangeErr)
				return
			}
			schedule.IntervalMinutes = minutes
		case "sync":
			if schedule.Sync == nil {
				httpx.WriteError(w, httpx.Invalid("%s has no sync setting", source))
				return
			}
			setting, syncErr := sourceSync(raw)
			if syncErr != nil {
				httpx.WriteError(w, syncErr)
				return
			}
			if !setting.ValidFor(source) {
				httpx.WriteError(w, httpx.Invalid("%s reads incrementally only", source))
				return
			}
			schedule.Sync = &setting
		case "chain":
			chain, chainErr := sourceChain(source, raw, schedule.Chain)
			if chainErr != nil {
				httpx.WriteError(w, chainErr)
				return
			}
			schedule.Chain = chain
		default:
			httpx.WriteError(w, httpx.Invalid("%s is not a source schedule field", key))
			return
		}
	}
	saved, err := a.Store.SaveSourceSchedule(r.Context(), schedule, floor, a.Now())
	if errors.Is(err, sqlite.ErrWorkflowValidation) {
		httpx.WriteError(w, httpx.Invalid("source schedule is invalid"))
		return
	}
	if err != nil {
		workflowError(w, err, "not_found")
		return
	}
	entry, err := a.scheduleEntry(r.Context(), saved)
	if err != nil {
		httpx.WriteCode(w, http.StatusServiceUnavailable, "database_unavailable")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, entry)
}

// sourceChain reads { methods: [{ id, enabled }], apiGate, apiDailyCap } over
// the current chain: methods, when given, list every method of the source
// once with at least one enabled; the X API gate and cap apply to X only.
func sourceChain(source string, raw json.RawMessage, current sqlite.SourceChain) (sqlite.SourceChain, error) {
	if !slices.Contains(sqlite.ChainSources, source) {
		return current, httpx.Invalid("the %s chain is fixed", source)
	}
	var body map[string]json.RawMessage
	if json.Unmarshal(raw, &body) != nil || body == nil {
		return current, httpx.Invalid("chain must be an object")
	}
	chain := current
	chain.Methods = slices.Clone(current.Methods)
	for key, value := range body {
		switch {
		case key == "methods":
			var methods []map[string]json.RawMessage
			if json.Unmarshal(value, &methods) != nil {
				return current, httpx.Invalid("chain methods must be a list of { id, enabled }")
			}
			chain.Methods = []sqlite.ChainMethod{}
			seen := map[string]bool{}
			for _, item := range methods {
				var method sqlite.ChainMethod
				if len(item) != 2 || json.Unmarshal(item["id"], &method.ID) != nil || json.Unmarshal(item["enabled"], &method.Enabled) != nil {
					return current, httpx.Invalid("chain methods must be a list of { id, enabled }")
				}
				if !slices.Contains(sqlite.ChainMethodIDs(source), method.ID) {
					return current, httpx.Invalid("%s is not a method of the %s chain", method.ID, source)
				}
				if seen[method.ID] {
					return current, httpx.Invalid("the chain lists %s twice", method.ID)
				}
				seen[method.ID] = true
				chain.Methods = append(chain.Methods, method)
			}
			if len(seen) != len(sqlite.ChainMethodIDs(source)) {
				return current, httpx.Invalid("the chain must list every method: %s", strings.Join(sqlite.ChainMethodIDs(source), ", "))
			}
			if len(chain.Enabled()) == 0 {
				return current, httpx.Invalid("the chain must enable at least one method")
			}
		case key == "apiGate" && source == "x":
			gate, err := integer(value, "apiGate", 0, sqlite.MaxXAPIGate)
			if err != nil {
				return current, err
			}
			chain.APIGate = gate
		case key == "apiDailyCap" && source == "x":
			limit, err := integer(value, "apiDailyCap", 1, sqlite.MaxXAPIDailyCap)
			if err != nil {
				return current, err
			}
			chain.APIDailyCap = limit
		default:
			return current, httpx.Invalid("%s is not a field of the %s chain", key, source)
		}
	}
	return chain, nil
}

// timedQueue is the work the timers have lined up for the single-flight
// queue: each enabled source schedule this process runs and the live-status
// probe when it has a next run, in the order they reach the queue. Entries
// due at or before now wait for the queue; the scheduler submits due sources
// earliest first, then by source, as DueSourceSchedule reads them.
func (a *api) timedQueue(schedules []sqlite.SourceSchedule, probeNext any) []map[string]any {
	type entry struct {
		at   time.Time
		key  string
		item map[string]any
	}
	now := a.Now()
	entries := []entry{}
	add := func(next string, key string, item map[string]any) {
		at, err := time.Parse(time.RFC3339, next)
		if err != nil {
			return
		}
		item["nextRunAt"], item["due"] = next, !at.After(now)
		entries = append(entries, entry{at, key, item})
	}
	for _, schedule := range schedules {
		if schedule.Enabled && !a.skipped(schedule.Source) && schedule.NextRunAt != nil {
			add(*schedule.NextRunAt, schedule.Source, map[string]any{"kind": "source", "source": schedule.Source})
		}
	}
	if next, ok := probeNext.(string); ok {
		add(next, "~live-probe", map[string]any{"kind": "live-probe"})
	}
	slices.SortStableFunc(entries, func(x, y entry) int {
		if c := x.at.Compare(y.at); c != 0 {
			return c
		}
		return strings.Compare(x.key, y.key)
	})
	out := make([]map[string]any, len(entries))
	for i, e := range entries {
		out[i] = e.item
	}
	return out
}

// startSourceSchedule starts a source's timer now: the schedule becomes due
// and the scheduler is asked to submit it at once. While a run holds the
// queue the timer waits as the next due source, and its claim plans the
// following run as for any timed run.
func (a *api) startSourceSchedule(w http.ResponseWriter, r *http.Request) {
	source := r.PathValue("source")
	if !slices.Contains(sqlite.ScheduleSources, source) {
		httpx.WriteCode(w, http.StatusNotFound, "not_found")
		return
	}
	if a.skipped(source) {
		httpx.WriteError(w, httpx.Invalid("the %s source is disabled for this process", source))
		return
	}
	if a.Scheduler == nil || a.Scheduler()["enabled"] != true {
		httpx.WriteError(w, httpx.Invalid("timers do not run in this process"))
		return
	}
	schedule, err := a.Store.StartSourceScheduleNow(r.Context(), source, a.Now(), a.bilibiliFloor())
	if errors.Is(err, sqlite.ErrWorkflowValidation) {
		httpx.WriteError(w, httpx.Invalid("the %s timer is off", source))
		return
	}
	if err != nil {
		workflowError(w, err, "not_found")
		return
	}
	if a.TickScheduler != nil {
		a.TickScheduler(r.Context())
	}
	entry, err := a.scheduleEntry(r.Context(), schedule)
	if err != nil {
		httpx.WriteCode(w, http.StatusServiceUnavailable, "database_unavailable")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, entry)
}

// runSource starts Run now: the source's fixed chain once, triggered by the
// operator and read with its schedule's sync setting. It does not move the
// schedule's next run.
func (a *api) runSource(w http.ResponseWriter, r *http.Request) {
	source := r.PathValue("source")
	if !slices.Contains(sqlite.ScheduleSources, source) {
		httpx.WriteCode(w, http.StatusNotFound, "not_found")
		return
	}
	if a.skipped(source) {
		httpx.WriteError(w, httpx.Invalid("the %s source is disabled for this process", source))
		return
	}
	result, err := a.Runs.Start(r.Context(), jobs.KindWorkflow, "admin", jobs.Options{Source: source})
	if err != nil {
		workflowError(w, err, "not_found")
		return
	}
	writeStart(w, result)
}
