package workflow

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	bilisource "github.com/yexca/kano-navi/backend/internal/source/bilibili"
	"github.com/yexca/kano-navi/backend/internal/source/history"
	xsource "github.com/yexca/kano-navi/backend/internal/source/x"
	ytsource "github.com/yexca/kano-navi/backend/internal/source/youtube"
	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
)

// HistorySource runs the operator's manual fetches, each through the one
// listing method it names. A method that fails fails the step; a manual fetch
// never falls back to another method. Credentials come from the environment
// and never enter a result.
type HistorySource struct {
	Fetcher *history.Fetcher
	Store   *sqlite.Store
	// XToken is the X API credential. YoutubeKey only reports whether the
	// YouTube API method can run; the uploads adapter holds its own copy.
	XToken, YoutubeKey string
	Handles            []string
	ChannelID          string
	// RequestLimit is the X page budget of one step, shared by all accounts.
	RequestLimit int
	// Bilibili is the adapter the Bilibili fetches read through; the source
	// steps run them, so it only reports whether they can run.
	Bilibili *bilisource.Adapter
}

// MethodAvailable reports whether a source's method can run now: the API
// methods need their credential, and every method its adapter.
func (h *HistorySource) MethodAvailable(source, method string) bool {
	if h == nil || h.Fetcher == nil || h.Store == nil {
		return false
	}
	switch {
	case source == history.SourceX && method == history.MethodTimeline,
		// The x_related step reads through the X adapter that serves the
		// timeline here.
		source == history.SourceXRelated && method == history.MethodTimeline:
		return h.Fetcher.Timeline != nil
	case source == history.SourceX && method == history.MethodAPI:
		return strings.TrimSpace(h.XToken) != ""
	case source == history.SourceYoutube && method == history.MethodAPI:
		return h.Fetcher.Uploads != nil && strings.TrimSpace(h.YoutubeKey) != ""
	case source == history.SourceYoutube && method == history.MethodRSS:
		return h.Fetcher.Uploads != nil
	case source == history.SourceYoutube && method == history.MethodYTDLP:
		reader, ok := h.Fetcher.Uploads.(history.YTDLPUploads)
		if !ok {
			return false
		}
		if checker, ok := reader.(interface{ YTDLPAvailable() bool }); ok {
			return checker.YTDLPAvailable()
		}
		return true
	case source == history.SourceShop && method == history.MethodCatalog:
		// The shop step reads the catalog itself, with the shop client.
		return true
	case source == history.SourceBilibili && method == history.MethodSpace:
		return h.Bilibili != nil
	case source == history.SourceBilibiliDynamics && method == history.MethodSpace:
		// The dynamics listing answers only a login cookie.
		return h.Bilibili != nil && h.Bilibili.Options.SignedIn()
	}
	return false
}

// Methods describes a source's listing methods and which can run now.
func (h *HistorySource) Methods(source string) []history.MethodStatus {
	out := []history.MethodStatus{}
	for _, method := range history.Methods(source) {
		out = append(out, history.MethodStatus{Method: method, Available: h.MethodAvailable(source, method.ID)})
	}
	return out
}

// historyMessage keeps controlled codes and operator-facing validation text;
// anything else is reduced to a fixed code.
func historyMessage(err error) string {
	var controlled *history.Error
	if errors.As(err, &controlled) {
		return controlled.Code
	}
	if history.IsValidation(err) {
		return err.Error()
	}
	return "source_write_failed"
}

func failedResult(message string) AttemptResult {
	return AttemptResult{Status: "failed", Result: map[string]any{"error": message}}
}

func stepStatus(messages []string) string {
	if len(messages) > 0 {
		return "partial"
	}
	return "succeeded"
}

// selection validates a window frozen on a run. A selection no method serves,
// such as one frozen before fetches named their method, fails cleanly.
func (h *HistorySource) selection(source string, window history.Window) (history.Window, string) {
	normalized, err := window.Normalize(source)
	if err != nil {
		return history.Window{}, historyMessage(err)
	}
	if !h.MethodAvailable(source, normalized.Method) {
		return history.Window{}, history.UnavailableMessage(source, normalized.Method)
	}
	return normalized, ""
}

// counts is the console's summary of a fetch: what the selection listed,
// split into new, already stored and failed records, the pages it read,
// whether it reached the end of what it asked for, and the first new items.
func counts(result history.Result) map[string]any {
	return map[string]any{
		"listed": result.Listed, "new": result.New, "stored": result.Stored, "failed": result.Failed,
		"pages": result.Requested, "reachedEnd": !result.HasMore,
	}
}

// FetchX fetches every configured account, or the one the window names,
// through the window's method. One account's failure keeps the others; the
// rotation spreads a short request budget across runs, and a one-account
// fetch takes the whole budget and leaves the rotation as it is.
func (h *HistorySource) FetchX(ctx context.Context, window history.Window) (AttemptResult, error) {
	window, message := h.selection(history.SourceX, window)
	if message != "" {
		return failedResult(message), nil
	}
	fetch := h.Fetcher.FetchTimeline
	if window.Method == history.MethodAPI {
		fetch = h.Fetcher.FetchSearch
	}
	if len(h.Handles) == 0 {
		return AttemptResult{}, NewStepError("x_no_accounts", ErrValidation)
	}
	ordered, err := h.fetchOrder(ctx, window)
	if err != nil {
		return AttemptResult{}, err
	}
	if len(ordered) == 0 {
		return failedResult("x_account_unknown"), nil
	}
	remaining := h.RequestLimit
	if remaining < 1 {
		remaining = 12
	}
	accounts := []map[string]any{}
	messages := []string{}
	total := history.Result{Items: []history.Item{}}
	// read counts the accounts whose fetch answered; an account skipped for
	// having no stored posts read nothing and cannot carry the run.
	read := 0
	for index, handle := range ordered {
		if window.Mode == history.ModeBefore {
			oldest, lookupErr := h.Store.OldestSourceTimestamp(ctx, history.SourceX, handle)
			if lookupErr != nil {
				return AttemptResult{}, NewStepError("schema_failure", ErrSchema)
			}
			if oldest == nil {
				message := "no stored records for this account; select a date range first"
				accounts = append(accounts, map[string]any{"handle": handle, "listed": 0, "new": 0, "stored": 0, "failed": 0, "pages": 0, "reachedEnd": true})
				messages = append(messages, handle+": "+message)
				continue
			}
		}
		if remaining <= 0 {
			total.HasMore = true
			messages = append(messages, handle+": X request budget exhausted")
			continue
		}
		left := len(ordered) - index
		limit := (remaining + left - 1) / left
		result, fetchErr := fetch(ctx, history.Request{
			Source: history.SourceX, Account: handle, Credential: h.XToken, Window: window,
			PageLimit: min(limit, h.Fetcher.Options.MaxPages),
		})
		if ctx.Err() != nil {
			return AttemptResult{}, NewStepError("cancelled", ErrCancelled)
		}
		total.Requested += result.Requested
		remaining -= result.Requested
		// A manual fetch ignores the timed chain's gate and cap, but the posts
		// it reads count toward the day's X API usage (ADR-0004).
		total.PostsRead += result.PostsRead
		if h.Store.AddXAPIUsage(ctx, sqlite.JapanDay(time.Now()), result.PostsRead) != nil {
			return AttemptResult{}, NewStepError("schema_failure", ErrSchema)
		}
		if fetchErr != nil {
			total.HasMore = true
			messages = append(messages, handle+": "+historyMessage(fetchErr))
			continue
		}
		read++
		account := counts(result)
		account["handle"] = handle
		accounts = append(accounts, account)
		total.Listed += result.Listed
		total.New += result.New
		total.Stored += result.Stored
		total.Failed += result.Failed
		total.ScheduleAssets += result.ScheduleAssets
		total.HasMore = total.HasMore || result.HasMore
		for _, item := range result.Items {
			if len(total.Items) < history.MaxItems {
				total.Items = append(total.Items, item)
			}
		}
		for _, message := range result.Errors {
			messages = append(messages, handle+": "+message)
		}
	}
	if read == 0 {
		failure := "X fetch failed for every account"
		if len(messages) > 0 {
			failure += " (" + messages[0] + ")"
		}
		return failedResult(failure), nil
	}
	out := counts(total)
	out["source"], out["scheduleAssets"], out["items"], out["accounts"], out["errors"] = "x", total.ScheduleAssets, total.Items, accounts, messages
	out["discovery"], out["apiRead"] = "api", total.PostsRead
	if window.Method == history.MethodTimeline {
		out["discovery"] = xsource.DiscoveryTimeline
		delete(out, "apiRead")
	}
	return AttemptResult{Status: stepStatus(messages), Result: out}, nil
}

// fetchOrder is the accounts an X fetch reads, in order: the one the window
// names, none when it is not configured, or every account starting after the
// one the previous fetch of this mode started with.
func (h *HistorySource) fetchOrder(ctx context.Context, window history.Window) ([]string, error) {
	if window.Account != "" {
		for _, handle := range h.Handles {
			if strings.EqualFold(handle, window.Account) {
				return []string{handle}, nil
			}
		}
		return nil, nil
	}
	rotationID := "window-" + window.Mode
	rotation, err := h.Store.SyncState(ctx, "x-budget", rotationID)
	if err != nil {
		return nil, NewStepError("schema_failure", ErrSchema)
	}
	next := 0
	last, _ := rotation.Metadata["lastAccount"].(string)
	for i, handle := range h.Handles {
		if handle == last {
			next = (i + 1) % len(h.Handles)
		}
	}
	ordered := append(append([]string{}, h.Handles[next:]...), h.Handles[:next]...)
	if h.Store.WriteSyncMetadata(ctx, "x-budget", rotationID, map[string]any{"lastAccount": ordered[0]}) != nil {
		return nil, NewStepError("schema_failure", ErrSchema)
	}
	return ordered, nil
}

// FetchYoutube fetches the configured channel through the window's method.
func (h *HistorySource) FetchYoutube(ctx context.Context, window history.Window) (AttemptResult, error) {
	window, message := h.selection(history.SourceYoutube, window)
	if message != "" {
		return failedResult(message), nil
	}
	result, err := h.Fetcher.FetchUploads(ctx, history.Request{Source: history.SourceYoutube, Account: h.ChannelID, Window: window})
	if ctx.Err() != nil {
		return AttemptResult{}, NewStepError("cancelled", ErrCancelled)
	}
	if err != nil {
		return failedResult(historyMessage(err)), nil
	}
	out := counts(result)
	errs := result.Errors
	// The fetch also reads the members-only playlist, as a recent run does:
	// through the Data API with the API method unless a Data API call of the
	// fetch failed, else its public feed.
	if members, ok := h.Fetcher.Uploads.(membersReader); ok {
		apiFailed := slices.ContainsFunc(result.Errors, func(code string) bool { return strings.HasPrefix(code, "youtube_api_") })
		read := members.MembersPlaylist(ctx, window.Method == history.MethodAPI && !apiFailed, false)
		if ctx.Err() != nil {
			return AttemptResult{}, NewStepError("cancelled", ErrCancelled)
		}
		result.APIRequests += read.APIRequests
		errs = append(errs, read.Errors...)
		if _, err = h.Store.WriteYoutubeMembers(ctx, read.IDs); err != nil {
			return AttemptResult{}, NewStepError("schema_failure", ErrSchema)
		}
		out["members"], out["membersFrom"] = len(read.IDs), read.From
	}
	out["items"], out["errors"] = result.Items, tallyCodes(errs)
	// The API method spends quota on its listing, its detail checks and the
	// members-only playlist.
	if window.Method == history.MethodAPI {
		out["apiRequests"], out["inspected"] = result.APIRequests, result.Inspected
	}
	return AttemptResult{Status: stepStatus(errs), Result: out}, nil
}

// membersReader is the YouTube adapter's members-only playlist read.
type membersReader interface {
	MembersPlaylist(ctx context.Context, useAPI, skipFeed bool) ytsource.MembersRead
}
