package workflow

import (
	"context"
	xsource "github.com/yexca/kano-navi/backend/internal/source/x"
	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
	"github.com/yexca/kano-navi/backend/internal/testutil"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

type xTestRT func(*http.Request) (*http.Response, error)

func (f xTestRT) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestXRecentExecutorPersistsWorkflowAttempt(t *testing.T) {
	p := testutil.NewSnapshot(t)
	s, err := sqlite.OpenWorkflowCore(p, true)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	rt := xTestRT(func(r *http.Request) (*http.Response, error) {
		body := `/kano/status/111111111111`
		if r.URL.Host == "api.vxtwitter.com" {
			body = `{"tweetID":"111111111111","text":"hello","date":"2026-10-11T00:00:00Z"}`
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})
	a := xsource.NewAdapter(xsource.NewClient(&http.Client{Transport: rt}), xsource.Options{Handles: []string{"kano"}, DiscoveryLimit: 1, MaxDetailRequests: 1, BootstrapDays: 7})
	ex := &XRecentExecutor{Store: s, Adapter: a, Options: a.Options}
	run, err := (&Executor{Store: s, Steps: ex, SyntheticSteps: map[string]bool{"x": false}}).Run(context.Background(), sqlite.WorkflowDefinition{ID: sqlite.AdhocFetchWorkflow, Steps: []string{"x"}}, TriggerOperator, "test-x")
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != "succeeded" {
		t.Fatalf("status=%s", run.Status)
	}
	got, err := s.GetWorkflowRun(context.Background(), run.ID)
	if err != nil || len(got.Steps) != 1 || len(got.Steps[0].Attempts) != 1 {
		t.Fatalf("run=%#v err=%v", got, err)
	}
}

func TestXSingleRequestBudgetRotatesAccountsAcrossRestart(t *testing.T) {
	p := testutil.NewSnapshot(t)
	s, err := sqlite.OpenWorkflowCore(p, true)
	if err != nil {
		t.Fatal(err)
	}
	requested := []string{}
	rt := xTestRT(func(r *http.Request) (*http.Response, error) {
		id, account := "111111111111", "fixture_a"
		if strings.Contains(r.URL.Path, "fixture_b") {
			id, account = "222222222222", "fixture_b"
		}
		body := "/" + account + "/status/" + id
		if r.URL.Host == "api.vxtwitter.com" {
			requested = append(requested, account)
			body = `{"tweetID":"` + id + `","text":"fixture","date":"2026-01-02T00:00:00Z"}`
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	opts := xsource.Options{Handles: []string{"fixture_a", "fixture_b"}, DiscoveryLimit: 1, MaxDetailRequests: 1, BootstrapDays: 7}
	a := xsource.NewAdapter(xsource.NewClient(&http.Client{Transport: rt}), opts)
	a.Now = func() time.Time { return time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC) }
	ex := &XRecentExecutor{Store: s, Adapter: a, Options: opts}
	if _, err = ex.Execute(context.Background(), StepInput{StepID: "x"}); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = sqlite.OpenWorkflowCore(p, true)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ex.Store = s
	if _, err = ex.Execute(context.Background(), StepInput{StepID: "x"}); err != nil {
		t.Fatal(err)
	}
	if len(requested) != 2 || requested[0] != "fixture_a" || requested[1] != "fixture_b" {
		t.Fatalf("account budget starved a source: %v", requested)
	}
}

func TestXRecentStoresTimelinePostsAndDropsAGoneStatus(t *testing.T) {
	p := testutil.NewSnapshot(t)
	s, err := sqlite.OpenWorkflowCore(p, true)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	// An earlier run left a status that no longer exists in the queue.
	gone := "1900000000000000001"
	testutil.Exec(t, p, `INSERT INTO sync_state(source,account_id,metadata_json) VALUES('x','fixture',?)`, `{"pendingStatusIds":["`+gone+`"]}`)
	page := ""
	details := []string{}
	rt := xTestRT(func(r *http.Request) (*http.Response, error) {
		status, body := 200, page
		switch r.URL.Host {
		case "api.fxtwitter.com":
		case "api.vxtwitter.com":
			details = append(details, r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:])
			status, body = 404, `{"error":"not found"}`
		default:
			t.Fatalf("unexpected destination %s", r.URL.Host)
		}
		return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	listed := func(id, hour string) string {
		return `{"code":200,"results":[{"id":"` + id + `","text":"fixture","created_timestamp":` + hour + `,"author":{"screen_name":"fixture"}}],"cursor":{"bottom":""}}`
	}
	now := time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)
	opts := xsource.Options{Handles: []string{"fixture"}, DiscoveryLimit: 50, MaxDetailRequests: 5, BootstrapDays: 7}
	a := xsource.NewAdapter(xsource.NewClient(&http.Client{Transport: rt}), opts)
	a.Now = func() time.Time { return now }
	ex := &XRecentExecutor{Store: s, Adapter: a, Options: opts}
	run := func(at time.Duration, id string) map[string]any {
		t.Helper()
		a.Now = func() time.Time { return now.Add(at) }
		page, details = listed(id, strconv.FormatInt(now.Add(at-time.Hour).Unix(), 10)), nil
		got, runErr := ex.Execute(context.Background(), StepInput{StepID: "x"})
		if runErr != nil {
			t.Fatal(runErr)
		}
		return got.Result
	}
	// The timeline's status is stored without a detail request; only the
	// pending one is requested, and its first 404 keeps it for an hour.
	first := run(0, "200000000000")
	if first["timelinePosts"] != 1 || first["posts"] != 1 || first["requested"] != 1 || strings.Join(details, ",") != gone || first["pendingDropped"] != nil {
		t.Fatalf("result=%#v details=%v", first, details)
	}
	held := run(30*time.Minute, "300000000000")
	if held["pendingHeld"] != 1 || held["requested"] != 0 || len(details) != 0 {
		t.Fatalf("result=%#v details=%v", held, details)
	}
	second := run(time.Hour, "400000000000")
	dropped, _ := second["pendingDropped"].([]string)
	if strings.Join(dropped, ",") != "fixture: "+gone+": "+xsource.DroppedGone {
		t.Fatalf("result=%#v", second)
	}
	state, err := s.XRecentState(context.Background(), "fixture")
	if err != nil || len(state.PendingIDs) != 0 || len(state.PendingRetries) != 0 || state.CursorID != "400000000000" {
		t.Fatalf("state=%#v err=%v", state, err)
	}
}

func TestXRecentDropsAnotherAccountsStatusWithoutFailing(t *testing.T) {
	p := testutil.NewSnapshot(t)
	s, err := sqlite.OpenWorkflowCore(p, true)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	// An earlier profile fallback queued a status the account quoted.
	foreign := "1900000000000000002"
	testutil.Exec(t, p, `INSERT INTO sync_state(source,account_id,metadata_json) VALUES('x','fixture',?)`, `{"pendingStatusIds":["`+foreign+`"]}`)
	now := time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)
	details := []string{}
	rt := xTestRT(func(r *http.Request) (*http.Response, error) {
		var body string
		switch r.URL.Host {
		case "api.fxtwitter.com":
			body = `{"code":200,"results":[{"id":"200000000000","text":"fixture","created_timestamp":` + strconv.FormatInt(now.Add(-time.Hour).Unix(), 10) + `,"author":{"screen_name":"fixture"}}],"cursor":{"bottom":""}}`
		case "api.vxtwitter.com":
			details = append(details, r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:])
			body = `{"tweetID":"` + foreign + `","text":"quoted","date":"2026-01-02T00:00:00Z","author":{"screenName":"other"}}`
		default:
			t.Fatalf("unexpected destination %s", r.URL.Host)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	opts := xsource.Options{Handles: []string{"fixture"}, DiscoveryLimit: 50, MaxDetailRequests: 5, BootstrapDays: 7}
	a := xsource.NewAdapter(xsource.NewClient(&http.Client{Transport: rt}), opts)
	a.Now = func() time.Time { return now }
	got, err := (&XRecentExecutor{Store: s, Adapter: a, Options: opts}).Execute(context.Background(), StepInput{StepID: "x"})
	if err != nil {
		t.Fatal(err)
	}
	dropped, _ := got.Result["pendingDropped"].([]string)
	errs, _ := got.Result["errors"].([]string)
	if got.Status != "succeeded" || got.Result["requested"] != 1 || strings.Join(details, ",") != foreign || len(errs) != 0 || strings.Join(dropped, ",") != "fixture: "+foreign+": "+xsource.DroppedForeign {
		t.Fatalf("status=%s result=%#v details=%v", got.Status, got.Result, details)
	}
	state, err := s.XRecentState(context.Background(), "fixture")
	if err != nil || len(state.PendingIDs) != 0 || len(state.PendingRetries) != 0 {
		t.Fatalf("state=%#v err=%v", state, err)
	}
}

func TestXRecentPagesTimelineBackToStoredCursor(t *testing.T) {
	p := testutil.NewSnapshot(t)
	s, err := sqlite.OpenWorkflowCore(p, true)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	// Synthetic IDs below Snowflake length stay inside the bootstrap window.
	page := map[string]string{"": `{"code":200,"results":[{"id":"200000000000","author":{"screen_name":"fixture"}}],"cursor":{"bottom":""}}`}
	cursors := []string{}
	rt := xTestRT(func(r *http.Request) (*http.Response, error) {
		body := ""
		switch r.URL.Host {
		case "api.fxtwitter.com":
			cursor := r.URL.Query().Get("cursor")
			cursors = append(cursors, cursor)
			body = page[cursor]
		case "api.vxtwitter.com":
			id := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
			// Later IDs are later posts, so the cursor advances.
			body = `{"tweetID":"` + id + `","text":"fixture","date":"2026-01-02T0` + id[:1] + `:00:00Z"}`
		default:
			t.Fatalf("unexpected destination %s", r.URL.Host)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	opts := xsource.Options{Handles: []string{"fixture"}, DiscoveryLimit: 50, MaxDetailRequests: 5, BootstrapDays: 7}
	a := xsource.NewAdapter(xsource.NewClient(&http.Client{Transport: rt}), opts)
	a.Now = func() time.Time { return time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC) }
	ex := &XRecentExecutor{Store: s, Adapter: a, Options: opts}
	if _, err = ex.Execute(context.Background(), StepInput{StepID: "x"}); err != nil {
		t.Fatal(err)
	}
	// The second page's last post was never stored but predates the cursor, so
	// the gap is closed without a third page.
	page[""] = `{"code":200,"results":[{"id":"400000000000","author":{"screen_name":"fixture"}}],"cursor":{"bottom":"page-2"}}`
	page["page-2"] = `{"code":200,"results":[{"id":"300000000000","author":{"screen_name":"fixture"}},{"id":"150000000000","author":{"screen_name":"fixture"}}],"cursor":{"bottom":"page-3"}}`
	cursors = cursors[:0]
	got, err := ex.Execute(context.Background(), StepInput{StepID: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(cursors, ",") != ",page-2" || got.Result["posts"] != 3 || got.Result["profileFallbacks"] != 0 {
		t.Fatalf("cursors=%q result=%#v", cursors, got.Result)
	}
	state, err := s.XRecentState(context.Background(), "fixture")
	if err != nil || state.CursorID != "400000000000" || !state.KnownIDs["300000000000"] {
		t.Fatalf("state=%#v err=%v", state, err)
	}
}

// A quiet run whose listing served is a success: the failed optional refresh
// of a stored post is a note that leaves the step partial, and the account's
// last success moves.
func TestXRecentQuietRunWithAFailedRefreshSucceeds(t *testing.T) {
	p := testutil.NewSnapshot(t)
	s, err := sqlite.OpenWorkflowCore(p, true)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	now := time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)
	stored := "200000000000"
	testutil.Exec(t, p, `INSERT INTO posts(id,source,account_handle,text,published_at,url) VALUES(?,'x','fixture','stored',?,?)`, stored, now.Add(-time.Hour).Format(time.RFC3339Nano), "https://x.com/fixture/status/"+stored)
	testutil.Exec(t, p, `INSERT INTO sync_state(source,account_id,cursor_id,cursor_time,metadata_json) VALUES('x','fixture',?,?,'{}')`, stored, now.Add(-time.Hour).Format(time.RFC3339Nano))
	details := []string{}
	rt := xTestRT(func(r *http.Request) (*http.Response, error) {
		status, body := 200, ""
		switch r.URL.Host {
		case "api.fxtwitter.com":
			body = `{"code":200,"results":[{"id":"` + stored + `","text":"stored","created_timestamp":` + strconv.FormatInt(now.Add(-time.Hour).Unix(), 10) + `,"author":{"screen_name":"fixture"}}],"cursor":{"bottom":""}}`
		case "api.vxtwitter.com":
			details = append(details, r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:])
			status, body = 503, "down"
		default:
			t.Fatalf("unexpected destination %s", r.URL.Host)
		}
		return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	opts := xsource.Options{Handles: []string{"fixture"}, DiscoveryLimit: 50, MaxDetailRequests: 5, RefreshKnown: 1, BootstrapDays: 7}
	a := xsource.NewAdapter(xsource.NewClient(&http.Client{Transport: rt}), opts)
	a.Now = func() time.Time { return now }
	got, err := (&XRecentExecutor{Store: s, Adapter: a, Options: opts}).Execute(context.Background(), StepInput{StepID: "x"})
	if err != nil {
		t.Fatalf("quiet run failed: %v", err)
	}
	errs, _ := got.Result["errors"].([]string)
	if got.Status != "partial" || got.Result["posts"] != 0 || strings.Join(details, ",") != stored || strings.Join(errs, ",") != "fixture: "+stored+": "+string(xsource.ErrServer) {
		t.Fatalf("status=%s result=%#v details=%v", got.Status, got.Result, details)
	}
	var success string
	if err = s.DB().QueryRow(`SELECT COALESCE(last_success_at,'') FROM sync_state WHERE source='x' AND account_id='fixture'`).Scan(&success); err != nil || success == "" {
		t.Fatalf("last_success_at=%q err=%v", success, err)
	}
	state, err := s.XRecentState(context.Background(), "fixture")
	if err != nil || len(state.PendingIDs) != 0 || state.CursorID != stored {
		t.Fatalf("state=%#v err=%v", state, err)
	}
}

// A status stored under another account's handle never fails the account
// that reads it: the post is skipped with a note, the rest of the run is
// written and the ID leaves the queue.
func TestXRecentSkipsAStatusAnotherAccountOwns(t *testing.T) {
	p := testutil.NewSnapshot(t)
	s, err := sqlite.OpenWorkflowCore(p, true)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	now := time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)
	owned := "1900000000000000003"
	testutil.Exec(t, p, `INSERT INTO posts(id,source,account_handle,text,published_at,url) VALUES(?,'x','old_fixture','stored wording','2026-01-02T00:00:00Z',?)`, owned, "https://x.com/old_fixture/status/"+owned)
	testutil.Exec(t, p, `INSERT INTO sync_state(source,account_id,metadata_json) VALUES('x','fixture',?)`, `{"pendingStatusIds":["`+owned+`"]}`)
	rt := xTestRT(func(r *http.Request) (*http.Response, error) {
		var body string
		switch r.URL.Host {
		case "api.fxtwitter.com":
			body = `{"code":200,"results":[{"id":"200000000000","text":"fresh","created_timestamp":` + strconv.FormatInt(now.Add(-time.Hour).Unix(), 10) + `,"author":{"screen_name":"fixture"}}],"cursor":{"bottom":""}}`
		case "api.vxtwitter.com":
			body = `{"tweetID":"` + owned + `","text":"renamed wording","date":"2026-01-02T00:00:00Z","author":{"screenName":"fixture"}}`
		default:
			t.Fatalf("unexpected destination %s", r.URL.Host)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	opts := xsource.Options{Handles: []string{"fixture"}, DiscoveryLimit: 50, MaxDetailRequests: 5, BootstrapDays: 7}
	a := xsource.NewAdapter(xsource.NewClient(&http.Client{Transport: rt}), opts)
	a.Now = func() time.Time { return now }
	got, err := (&XRecentExecutor{Store: s, Adapter: a, Options: opts}).Execute(context.Background(), StepInput{StepID: "x"})
	if err != nil {
		t.Fatalf("the conflict failed the step: %v", err)
	}
	errs, _ := got.Result["errors"].([]string)
	if got.Status != "partial" || strings.Join(errs, ",") != "fixture: "+owned+": "+sqlite.XPostOwnerConflict {
		t.Fatalf("status=%s result=%#v", got.Status, got.Result)
	}
	var account, text string
	if err = s.DB().QueryRow(`SELECT account_handle,text FROM posts WHERE id=?`, owned).Scan(&account, &text); err != nil || account != "old_fixture" || text != "stored wording" {
		t.Fatalf("owned row=%s %q err=%v", account, text, err)
	}
	state, err := s.XRecentState(context.Background(), "fixture")
	if err != nil || len(state.PendingIDs) != 0 || !state.KnownIDs["200000000000"] {
		t.Fatalf("state=%#v err=%v", state, err)
	}
}
