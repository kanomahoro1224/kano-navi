package workflow

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/yexca/kano-navi/backend/internal/source/history"
	xsource "github.com/yexca/kano-navi/backend/internal/source/x"
	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
	"github.com/yexca/kano-navi/backend/internal/testutil"
)

// relatedFixture is a store with the own account fixture, the related
// accounts given as the setting's JSON, and an FxTwitter transport that
// fails the handles in broken and records every path it serves.
type relatedFixture struct {
	store *sqlite.Store
	ex    *XRecentExecutor
	paths []string
}

func newRelatedStepFixture(t *testing.T, accounts string, broken ...string) *relatedFixture {
	t.Helper()
	p := testutil.NewSnapshot(t)
	s, err := sqlite.OpenWorkflowCore(p, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if accounts != "" {
		testutil.Exec(t, p, `INSERT INTO app_settings(key,value,updated_at) VALUES(?,?,'2026-01-01T00:00:00.000Z')`, sqlite.XRelatedSetting, accounts)
	}
	now := time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)
	hour := strconv.FormatInt(now.Add(-time.Hour).Unix(), 10)
	f := &relatedFixture{store: s}
	rt := xTestRT(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "api.fxtwitter.com" {
			t.Fatalf("unexpected destination %s", r.URL)
		}
		f.paths = append(f.paths, r.URL.Path)
		for _, handle := range broken {
			if strings.Contains(r.URL.Path, "/"+handle+"/") {
				return &http.Response{StatusCode: 503, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"code":503}`))}, nil
			}
		}
		body := `{"code":200,"results":[{"id":"200000000000","text":"fixture","created_timestamp":` + hour + `,"author":{"screen_name":"fixture"}}],"cursor":{"bottom":""}}`
		if strings.Contains(r.URL.Path, "/agency/") {
			body = `{"code":200,"results":[` +
				`{"id":"300000000002","text":"鹿乃まほろ出演決定","created_timestamp":` + hour + `,"author":{"screen_name":"agency","name":"Agency"}},` +
				`{"id":"300000000001","text":"other talent","created_timestamp":` + hour + `,"author":{"screen_name":"agency","name":"Agency"}},` +
				`{"id":"300000000000","text":"a repost","created_timestamp":` + hour + `,"author":{"screen_name":"someone"},"reposted_by":{"screen_name":"agency"}}` +
				`],"cursor":{"bottom":""}}`
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	opts := xsource.Options{Handles: []string{"fixture"}, DiscoveryLimit: 50, MaxDetailRequests: 5, BootstrapDays: 7}
	a := xsource.NewAdapter(xsource.NewClient(&http.Client{Transport: rt}), opts)
	a.Now = func() time.Time { return now }
	f.ex = &XRecentExecutor{Store: s, Adapter: a, Options: opts}
	return f
}

func (f *relatedFixture) query(t *testing.T, q string) string {
	t.Helper()
	var out string
	if err := f.store.DB().QueryRow(q).Scan(&out); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
	return out
}

func TestXStepReadsOnlyTheOwnAccounts(t *testing.T) {
	f := newRelatedStepFixture(t, `[{"handle":"agency","keywords":["鹿乃"]}]`)
	got, err := f.ex.Execute(context.Background(), StepInput{StepID: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if _, read := got.Result["related"]; read || got.Status != "succeeded" || len(f.paths) != 1 || strings.Contains(f.paths[0], "/agency/") {
		t.Fatalf("result=%#v requests=%v", got.Result, f.paths)
	}
	if pooled := f.query(t, `SELECT COUNT(*) FROM x_statuses`); pooled != "0" {
		t.Fatalf("pool=%s", pooled)
	}
}

func TestXStepFailureLeavesTheRelatedStepReading(t *testing.T) {
	f := newRelatedStepFixture(t, `[{"handle":"agency","keywords":["鹿乃"]}]`, "fixture")
	f.ex.Options.Chain = nil
	in := StepInput{StepID: "x", Chain: &sqlite.SourceChain{Methods: []sqlite.ChainMethod{{ID: "timeline", Enabled: true}, {ID: "profile", Enabled: false}, {ID: "api", Enabled: false}}}}
	var stepErr *StepError
	if _, err := f.ex.Execute(context.Background(), in); !errors.As(err, &stepErr) || stepErr.Code != "x_source_failed" {
		t.Fatalf("x err=%v", err)
	}
	got, err := f.ex.ExecuteRelated(context.Background(), StepInput{StepID: sqlite.XRelatedStep})
	if err != nil || got.Status != "succeeded" || got.Result["promoted"] != 1 {
		t.Fatalf("related=%#v err=%v", got, err)
	}
}

func TestXRelatedStepReadsTimelinesIntoThePool(t *testing.T) {
	f := newRelatedStepFixture(t, `[{"handle":"agency","keywords":["鹿乃"]}]`)
	got, err := f.ex.ExecuteRelated(context.Background(), StepInput{StepID: sqlite.XRelatedStep, Sync: sqlite.SourceSync{Mode: sqlite.SyncIncremental, Max: 50}})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"source": sqlite.XRelatedStep, "accounts": 1, "listed": 2, "promoted": 1, "demoted": 0}
	if got.Status != "succeeded" || len(got.Result) != len(want) {
		t.Fatalf("result=%#v", got)
	}
	for key, value := range want {
		if got.Result[key] != value {
			t.Fatalf("%s: result=%#v", key, got.Result)
		}
	}
	// The related account is read through its timeline only, and the own
	// accounts are not read.
	if len(f.paths) != 1 || !strings.Contains(f.paths[0], "/agency/") {
		t.Fatalf("requests=%v", f.paths)
	}
	if pool := f.query(t, `SELECT group_concat(id,',') FROM (SELECT id FROM x_statuses ORDER BY id)`); pool != "300000000001,300000000002" {
		t.Fatalf("pool=%s", pool)
	}
	if promoted := f.query(t, `SELECT id||'|'||account_handle FROM posts WHERE promoted_at IS NOT NULL`); promoted != "300000000002|agency" {
		t.Fatalf("promoted=%s", promoted)
	}
	state, err := f.store.XRelatedState(context.Background(), "agency")
	if err != nil || state.CursorID != "300000000002" || len(state.KnownIDs) != 2 {
		t.Fatalf("related state=%#v err=%v", state, err)
	}
	if own, _ := f.store.XRecentState(context.Background(), "fixture"); own.CursorID != "" {
		t.Fatalf("own state=%#v", own)
	}
}

func TestXRelatedStepWithoutAccountsSkipsWithoutARequest(t *testing.T) {
	f := newRelatedStepFixture(t, "")
	got, err := f.ex.ExecuteRelated(context.Background(), StepInput{StepID: sqlite.XRelatedStep})
	if err != nil || got.Status != "succeeded" || got.Result["skipped"] != true || got.Result["reason"] != "no_accounts" || len(f.paths) != 0 {
		t.Fatalf("result=%#v err=%v requests=%v", got, err, f.paths)
	}
}

func TestXRelatedStepFailuresArePartialThenFailed(t *testing.T) {
	ctx := context.Background()
	f := newRelatedStepFixture(t, `[{"handle":"agency","keywords":["鹿乃"]},{"handle":"broken","keywords":["鹿乃"]}]`, "broken")
	got, err := f.ex.ExecuteRelated(ctx, StepInput{StepID: sqlite.XRelatedStep})
	errs, _ := got.Result["errors"].([]string)
	if err != nil || got.Status != "partial" || got.Result["accounts"] != 2 || got.Result["promoted"] != 1 || len(errs) != 1 || !strings.HasPrefix(errs[0], "@broken: ") {
		t.Fatalf("partial=%#v err=%v", got, err)
	}
	f = newRelatedStepFixture(t, `[{"handle":"agency","keywords":["鹿乃"]},{"handle":"broken","keywords":["鹿乃"]}]`, "agency", "broken")
	var stepErr *StepError
	if _, err = f.ex.ExecuteRelated(ctx, StepInput{StepID: sqlite.XRelatedStep}); !errors.As(err, &stepErr) || stepErr.Code != "x_related_failed" || !stepErr.Retryable {
		t.Fatalf("failed err=%v", err)
	}
	// Each account records its failure as its last error.
	accounts, err := f.store.XRelatedAccounts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	statuses, err := f.store.XRelatedStatuses(ctx, accounts)
	if err != nil || len(statuses) != 2 || statuses[0].LastError == nil || statuses[1].LastError == nil {
		t.Fatalf("statuses=%+v err=%v", statuses, err)
	}
}

func TestXRelatedStepFollowsTheSchedulesCapAndSkipX(t *testing.T) {
	ex := &XRecentExecutor{Options: xsource.Options{Handles: []string{"fixture"}, DiscoveryLimit: 20, BootstrapDays: 7}}
	if opts := ex.RelatedOptions(StepInput{Sync: sqlite.SourceSync{Mode: sqlite.SyncIncremental, Max: 120}, LookbackDays: 3}); opts.DiscoveryLimit != 120 || opts.BootstrapDays != 3 {
		t.Fatalf("scheduled=%+v", opts)
	}
	if opts := ex.RelatedOptions(StepInput{}); opts.DiscoveryLimit != 20 || opts.BootstrapDays != 7 {
		t.Fatalf("full sync=%+v", opts)
	}
	steps := &SourceSteps{X: ex, Skip: map[string]bool{sqlite.XRelatedStep: true}}
	got, err := steps.Execute(context.Background(), StepInput{StepID: sqlite.XRelatedStep})
	if err != nil || got.Result["skipped"] != true || got.Result["reason"] != "environment" {
		t.Fatalf("skipped=%#v err=%v", got, err)
	}
}

func TestXRelatedFetchReadsOnlyTheNamedAccount(t *testing.T) {
	ctx := context.Background()
	f := newRelatedStepFixture(t, `[{"handle":"agency","keywords":["鹿乃"]},{"handle":"other","keywords":["鹿乃"]}]`)
	window := history.Window{Method: history.MethodTimeline, Mode: history.ModeIncremental, Account: "Agency"}
	got, err := f.ex.ExecuteRelated(ctx, StepInput{StepID: sqlite.XRelatedStep, Window: &window})
	if err != nil || got.Status != "succeeded" || got.Result["accounts"] != 1 || got.Result["promoted"] != 1 {
		t.Fatalf("result=%#v err=%v", got, err)
	}
	if len(f.paths) != 1 || !strings.Contains(f.paths[0], "/agency/") {
		t.Fatalf("requests=%v", f.paths)
	}
	// An account removed since the fetch was queued fails without a request.
	window.Account = "removed"
	got, err = f.ex.ExecuteRelated(ctx, StepInput{StepID: sqlite.XRelatedStep, Window: &window})
	if err != nil || got.Status != "failed" || got.Result["error"] != "x_related_account_unknown" || len(f.paths) != 1 {
		t.Fatalf("removed=%#v err=%v requests=%v", got, err, f.paths)
	}
	// A failed read fails the fetch once with the account's code.
	f = newRelatedStepFixture(t, `[{"handle":"broken","keywords":["鹿乃"]}]`, "broken")
	window.Account = "broken"
	got, err = f.ex.ExecuteRelated(ctx, StepInput{StepID: sqlite.XRelatedStep, Window: &window})
	if err != nil || got.Status != "failed" || !strings.HasPrefix(got.Result["error"].(string), "@broken: ") {
		t.Fatalf("broken=%#v err=%v", got, err)
	}
}
