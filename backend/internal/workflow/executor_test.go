package workflow

import (
	"context"
	"testing"
	"time"

	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
	"github.com/yexca/kano-navi/backend/internal/testutil"
)

type retryOnce struct {
	calls int
	at    []time.Time
}

func (x *retryOnce) Execute(_ context.Context, in StepInput) (AttemptResult, error) {
	x.calls++
	x.at = append(x.at, time.Now())
	if x.calls == 1 {
		return AttemptResult{}, NewStepError("temporary_dependency", ErrDependency)
	}
	return AttemptResult{Status: "succeeded", Result: map[string]any{"step": in.StepID}}, nil
}
func TestExecutorPersistsOrderedSyntheticRunAndRetry(t *testing.T) {
	path := testutil.NewSnapshot(t)
	store, err := sqlite.OpenWorkflowCore(path, true)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	x := &retryOnce{}
	ex := &Executor{Store: store, Steps: x, OwnerID: "test", MaxAttempts: 2, RetryDelay: 200 * time.Millisecond}
	run, err := ex.Run(context.Background(), sqlite.WorkflowDefinition{ID: sqlite.AdhocFetchWorkflow, Steps: []string{"x", "schedule"}}, TriggerOperator, "idem")
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != "succeeded" || x.calls != 3 {
		t.Fatalf("run=%#v calls=%d", run, x.calls)
	}
	// The retry of a retryable failure waits rather than meeting the same
	// limit at once.
	if waited := x.at[1].Sub(x.at[0]); waited < 200*time.Millisecond {
		t.Fatalf("retried after %s", waited)
	}
	got, err := store.GetWorkflowRun(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Steps) != 2 || len(got.Steps[0].Attempts) != 2 || got.Steps[1].StepID != "schedule" {
		t.Fatalf("steps=%#v", got.Steps)
	}
}
