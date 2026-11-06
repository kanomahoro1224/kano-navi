package workflow

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
	"github.com/yexca/kano-navi/backend/internal/testutil"
)

// flakyRenewStore fails the first renewal the way a busy database does and
// reports each later successful one.
type flakyRenewStore struct {
	*sqlite.Store
	calls   atomic.Int32
	renewed chan struct{}
}

func (s *flakyRenewStore) RenewWorkflowLease(ctx context.Context, l sqlite.WorkflowLease, now time.Time, ttl time.Duration) error {
	if s.calls.Add(1) == 1 {
		return sqlite.ErrWorkflowStorage
	}
	err := s.Store.RenewWorkflowLease(ctx, l, now, ttl)
	if err == nil {
		select {
		case s.renewed <- struct{}{}:
		default:
		}
	}
	return err
}

func TestATransientRenewalFailureKeepsTheRunGoing(t *testing.T) {
	s, err := sqlite.OpenWorkflowCore(testutil.NewSnapshot(t), true)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	store := &flakyRenewStore{Store: s, renewed: make(chan struct{}, 1)}
	step := &blockingStep{entered: make(chan StepInput, 1), release: make(chan struct{})}
	// The lease is claimed, renewed and fenced at one frozen instant, so a slow
	// runner that delays a heartbeat past the short TTL cannot expire it; only
	// the failed renewal can stop the run.
	frozen := time.Now()
	clock := func() time.Time { return frozen }
	q := &Queue{Store: s, Executor: &Executor{Store: store, Steps: step, LeaseTTL: 300 * time.Millisecond, Clock: clock, SyntheticSteps: map[string]bool{"x": true}}}
	defer q.Close()
	r, err := q.SubmitDefinition(context.Background(), xRecent, TriggerOperator, nil)
	if err != nil {
		t.Fatal(err)
	}
	<-step.entered
	select {
	case <-store.renewed:
	case <-time.After(5 * time.Second):
		t.Fatal("no renewal after the failed one")
	}
	close(step.release)
	if got := waitRun(t, s, r.ID, "succeeded", "failed", "cancelled"); got.Status != "succeeded" {
		t.Fatalf("one failed renewal stopped the run: %s", got.Status)
	}
}

func TestARunPastItsTimeLimitFailsAndCanBeRetried(t *testing.T) {
	s, err := sqlite.OpenWorkflowCore(testutil.NewSnapshot(t), true)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	step := &blockingStep{entered: make(chan StepInput, 1), release: make(chan struct{})}
	e := &Executor{Store: s, Steps: step, SyntheticSteps: map[string]bool{"x": true}}
	run, err := e.PrepareDefinition(context.Background(), xRecent, TriggerOperator, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, _ = e.ExecuteRun(ctx, run)
	got, err := s.GetWorkflowRun(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "failed" || got.ErrorCode == nil || *got.ErrorCode != "timeout" {
		t.Fatalf("status=%s code=%v", got.Status, got.ErrorCode)
	}
	if err = s.RetryWorkflowRun(context.Background(), run.ID, time.Now()); err != nil {
		t.Fatalf("a timed-out run cannot be retried: %v", err)
	}
}
