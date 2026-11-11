package workflow

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/yexca/kano-navi/backend/internal/sourcecapture"
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

type responseCaptureStep struct{ body []byte }

func (s responseCaptureStep) Execute(ctx context.Context, _ StepInput) (AttemptResult, error) {
	client := &http.Client{Transport: sourcecapture.NewTransport("x", captureRoundTrip(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusServiceUnavailable,
			Header:     http.Header{"Content-Type": {"application/json"}},
			Body:       io.NopCloser(bytes.NewReader(s.body)),
			Request:    request,
		}, nil
	}))}
	target, _ := url.Parse("https://api.example.test/2/tweets")
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	response, err := client.Do(request)
	if err != nil {
		return AttemptResult{}, err
	}
	defer response.Body.Close()
	if _, err = io.Copy(io.Discard, response.Body); err != nil {
		return AttemptResult{}, err
	}
	return AttemptResult{Status: "succeeded"}, nil
}

type captureRoundTrip func(*http.Request) (*http.Response, error)

func (fn captureRoundTrip) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func TestExecutorPersistsSourceResponseWithRunStepAttempt(t *testing.T) {
	path := testutil.NewSnapshot(t)
	store, err := sqlite.OpenWorkflowCore(path, true)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	want := []byte(`{}`)
	ex := &Executor{Store: store, Steps: responseCaptureStep{body: want}, OwnerID: "test", MaxAttempts: 1}
	run, err := ex.Run(context.Background(), sqlite.WorkflowDefinition{ID: sqlite.AdhocFetchWorkflow, Steps: []string{"x"}}, TriggerOperator, "capture-test")
	if err != nil {
		t.Fatal(err)
	}
	captures, err := store.ListSourceResponseCaptures(context.Background(), "x", 10)
	if err != nil || len(captures) != 1 {
		t.Fatalf("captures=%d err=%v", len(captures), err)
	}
	capture, err := store.ReadSourceResponseCapture(context.Background(), captures[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(capture.Body, want) || capture.StatusCode != http.StatusServiceUnavailable || capture.RunID != run.ID || capture.WorkflowID != run.WorkflowID || capture.StepID != "x" || capture.Attempt != 1 {
		t.Fatalf("capture=%+v body=%q", capture, capture.Body)
	}
}
