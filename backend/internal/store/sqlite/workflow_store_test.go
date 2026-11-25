package sqlite

import (
	"context"
	"errors"
	"github.com/yexca/kano-navi/backend/internal/testutil"
	"testing"
	"time"
)

func fixture(t *testing.T) (*Store, string) {
	p := testutil.NewSnapshot(t)
	s, e := OpenWorkflowCore(p, true)
	if e != nil {
		t.Fatal(e)
	}
	return s, p
}

// fixtureDefinition is an ad hoc step run; no saved definition is read.
var fixtureDefinition = &WorkflowDefinition{ID: AdhocFetchWorkflow, Steps: []string{"x", "schedule"}}

func TestWorkflowMigrationAndLeaseSingleFlight(t *testing.T) {
	s, _ := fixture(t)
	defer s.Close()
	ctx := context.Background()
	def := fixtureDefinition
	r, e := s.CreateWorkflowRun(ctx, *def, "operator", "key", map[string]bool{"x": true, "schedule": true}, time.Now())
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.CreateWorkflowRun(ctx, *def, "scheduler", "other", nil, time.Now())
	if !errors.Is(e, ErrWorkflowBusy) {
		t.Fatalf("busy=%v", e)
	}
	l, e := s.ClaimWorkflowRun(ctx, r.ID, "a", time.Now(), time.Minute)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.ClaimWorkflowRun(ctx, r.ID, "b", time.Now(), time.Minute); !errors.Is(e, ErrWorkflowBusy) {
		t.Fatalf("lease=%v", e)
	}
	n, e := s.StartWorkflowStep(ctx, l, r.Steps[0].ID, time.Now())
	if e != nil || n != 1 {
		t.Fatalf("step %v %v", n, e)
	}
	if e = s.FinishWorkflowStep(ctx, l, r.Steps[0].ID, n, WorkflowOutcome{Status: "succeeded"}, false, time.Now()); e != nil {
		t.Fatal(e)
	}
	if e = s.FinishWorkflowRun(ctx, l, "succeeded", "", time.Now()); e != nil {
		t.Fatal(e)
	}
	got, e := s.GetWorkflowRun(ctx, r.ID)
	if e != nil || got.Status != "succeeded" || len(got.Steps) != 2 {
		t.Fatalf("got=%#v e=%v", got, e)
	}
}
func TestWorkflowRecoveryMarksExpiredRunningState(t *testing.T) {
	s, p := fixture(t)
	ctx := context.Background()
	def := fixtureDefinition
	r, e := s.CreateWorkflowRun(ctx, *def, "operator", "", nil, time.Now())
	if e != nil {
		t.Fatal(e)
	}
	s.Close()
	testutil.Exec(t, p, `UPDATE workflow_runs SET status='running' WHERE id=?`, r.ID)
	testutil.Exec(t, p, `INSERT INTO workflow_leases(scope,run_id,owner_id,acquired_at,expires_at,generation) VALUES('global',?,?,?,?,1)`, r.ID, "dead", "2026-10-12T00:00:00.000Z", "2026-10-12T00:01:00.000Z")
	s, e = OpenWorkflowCore(p, true)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	n, e := s.RecoverWorkflowRuns(ctx, time.Date(2026, 10, 12, 0, 2, 0, 0, time.UTC))
	if e != nil || n != 1 {
		t.Fatalf("recovery=%d %v", n, e)
	}
	got, e := s.GetWorkflowRun(ctx, r.ID)
	if e != nil || got.Status != "queued" || got.RecoveryCount != 1 {
		t.Fatalf("got=%#v e=%v", got, e)
	}
}

// settleSteps finishes a claimed run's steps with the given outcomes, in order.
func settleSteps(t *testing.T, s *Store, l WorkflowLease, r *WorkflowRun, outcomes []WorkflowOutcome) {
	t.Helper()
	ctx := context.Background()
	for i, out := range outcomes {
		n, e := s.StartWorkflowStep(ctx, l, r.Steps[i].ID, time.Now())
		if e != nil {
			t.Fatal(e)
		}
		if e = s.FinishWorkflowStep(ctx, l, r.Steps[i].ID, n, out, false, time.Now()); e != nil {
			t.Fatal(e)
		}
	}
}

// Retrying a source step re-runs the post-processing after it, since the
// retried read can store new posts with media, while a later source that
// already read and a failure before every source keep their results.
func TestWorkflowRetryRerunsProcessingAfterARetriedSource(t *testing.T) {
	s, _ := fixture(t)
	defer s.Close()
	ctx := context.Background()
	steps := []string{"media", "x", "youtube", "schedule", "translate", "reconcile"}
	synthetic := map[string]bool{}
	for _, step := range steps {
		synthetic[step] = true
	}
	r, e := s.CreateWorkflowRun(ctx, WorkflowDefinition{ID: AdhocFetchWorkflow, Steps: steps}, "scheduler", "", synthetic, time.Now())
	if e != nil {
		t.Fatal(e)
	}
	l, e := s.ClaimWorkflowRun(ctx, r.ID, "a", time.Now(), time.Minute)
	if e != nil {
		t.Fatal(e)
	}
	ok, part := WorkflowOutcome{Status: "succeeded"}, WorkflowOutcome{Status: "partial"}
	settleSteps(t, s, l, r, []WorkflowOutcome{ok, {Status: "failed", Code: "x_source_failed", Retryable: true}, ok, part, ok, ok})
	if e = s.FinishWorkflowRun(ctx, l, "partial", "", time.Now()); e != nil {
		t.Fatal(e)
	}
	if e = s.RetryWorkflowRun(ctx, r.ID, time.Now()); e != nil {
		t.Fatal(e)
	}
	got, e := s.GetWorkflowRun(ctx, r.ID)
	if e != nil {
		t.Fatal(e)
	}
	want := map[string]string{"media": "succeeded", "x": "pending", "youtube": "succeeded", "schedule": "pending", "translate": "pending", "reconcile": "pending"}
	for _, step := range got.Steps {
		if step.Status != want[step.StepID] {
			t.Fatalf("%s=%s want %s: %#v", step.StepID, step.Status, want[step.StepID], got.Steps)
		}
	}
}

// A retried processing step re-runs alone: it stores no new records.
func TestWorkflowRetryOfAProcessingStepKeepsTheOthers(t *testing.T) {
	s, _ := fixture(t)
	defer s.Close()
	ctx := context.Background()
	r, e := s.CreateWorkflowRun(ctx, WorkflowDefinition{ID: AdhocFetchWorkflow, Steps: []string{"x", "media", "schedule"}}, "scheduler", "", map[string]bool{"x": true, "media": true, "schedule": true}, time.Now())
	if e != nil {
		t.Fatal(e)
	}
	l, e := s.ClaimWorkflowRun(ctx, r.ID, "a", time.Now(), time.Minute)
	if e != nil {
		t.Fatal(e)
	}
	ok := WorkflowOutcome{Status: "succeeded"}
	settleSteps(t, s, l, r, []WorkflowOutcome{ok, {Status: "failed", Code: "timeout", Retryable: true}})
	if e = s.FinishWorkflowRun(ctx, l, "failed", "timeout", time.Now()); e != nil {
		t.Fatal(e)
	}
	if e = s.RetryWorkflowRun(ctx, r.ID, time.Now()); e != nil {
		t.Fatal(e)
	}
	got, e := s.GetWorkflowRun(ctx, r.ID)
	if e != nil || got.Steps[0].Status != "succeeded" || got.Steps[1].Status != "pending" || got.Steps[2].Status != "pending" {
		t.Fatalf("got=%#v e=%v", got, e)
	}
}

// A run's outcome is operator diagnosis: finishing or cancelling a run that
// wrote nothing leaves the dashboard revision, so open boards stay put.
func TestWorkflowOutcomeLeavesTheDashboardRevision(t *testing.T) {
	s, _ := fixture(t)
	defer s.Close()
	ctx := context.Background()
	before, e := s.GetDashboardRevision(ctx)
	if e != nil {
		t.Fatal(e)
	}
	for _, status := range []string{"succeeded", "failed"} {
		r, e := s.CreateWorkflowRun(ctx, WorkflowDefinition{ID: AdhocLiveWorkflow, Steps: []string{"live"}}, "scheduler", "", map[string]bool{"live": true}, time.Now())
		if e != nil {
			t.Fatal(e)
		}
		l, e := s.ClaimWorkflowRun(ctx, r.ID, "a", time.Now(), time.Minute)
		if e != nil {
			t.Fatal(e)
		}
		settleSteps(t, s, l, r, []WorkflowOutcome{{Status: status, Code: map[string]string{"failed": "timeout"}[status], Retryable: status == "failed"}})
		if e = s.FinishWorkflowRun(ctx, l, status, "", time.Now()); e != nil {
			t.Fatal(e)
		}
	}
	r, e := s.CreateWorkflowRun(ctx, WorkflowDefinition{ID: AdhocLiveWorkflow, Steps: []string{"live"}}, "scheduler", "", map[string]bool{"live": true}, time.Now())
	if e != nil {
		t.Fatal(e)
	}
	if e = s.RequestWorkflowCancellation(ctx, r.ID, time.Now()); e != nil {
		t.Fatal(e)
	}
	if after, e := s.GetDashboardRevision(ctx); e != nil || after != before {
		t.Fatalf("revision %d -> %d err=%v", before, after, e)
	}
}
