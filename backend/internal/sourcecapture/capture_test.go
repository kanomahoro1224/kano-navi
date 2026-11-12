package sourcecapture

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

type recordingCapture struct{ got Response }

func (r *recordingCapture) RecordSourceResponse(_ context.Context, response Response) error {
	r.got = response
	return nil
}

type closeTrackingBody struct {
	*bytes.Reader
	closed bool
}

func (b *closeTrackingBody) Close() error {
	b.closed = true
	return nil
}

func TestTransportCapturesMetadataAndLeavesResponseReadable(t *testing.T) {
	want := []byte("{\"error\":\"slow down\"}")
	recorder := &recordingCapture{}
	originalBody := &closeTrackingBody{Reader: bytes.NewReader(want)}
	transport := NewTransport("x", roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusTooManyRequests, Header: http.Header{"Content-Type": {"application/json"}}, Body: originalBody, ContentLength: int64(len(want)), Request: request}, nil
	}))
	target, _ := url.Parse("https://api.x.com/2/users/123456789/tweets?token=synthetic-secret")
	request := httptestRequest(target)
	request = request.WithContext(WithExecution(request.Context(), recorder, Execution{RunID: "run-1", WorkflowID: "@source-x", StepID: "x", Attempt: 2}))
	response, err := transport.RoundTrip(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := response.Body.Close(); err != nil {
			t.Errorf("close response body: %v", err)
		}
		if !originalBody.closed {
			t.Error("original response body was not closed")
		}
	}()
	got, err := io.ReadAll(response.Body)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("response=%q err=%v", got, err)
	}
	if recorder.got.StatusCode != http.StatusTooManyRequests || !bytes.Equal(recorder.got.Body, want) || recorder.got.RunID != "run-1" || recorder.got.StepID != "x" || recorder.got.Attempt != 2 {
		t.Fatalf("capture=%+v", recorder.got)
	}
	if strings.Contains(recorder.got.Endpoint, "synthetic-secret") || strings.Contains(recorder.got.Endpoint, "123456789") {
		t.Fatalf("endpoint retained sensitive request detail: %q", recorder.got.Endpoint)
	}
	digest := sha256.Sum256(want)
	if recorder.got.SHA256 != hex.EncodeToString(digest[:]) {
		t.Fatalf("hash=%s", recorder.got.SHA256)
	}
}

func TestTransportBoundsStoredBodyWithoutTruncatingSourceResponse(t *testing.T) {
	want := bytes.Repeat([]byte("a"), int(MaxBodyBytes)+31)
	recorder := &recordingCapture{}
	transport := NewTransport("shop", roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/plain"}}, Body: io.NopCloser(bytes.NewReader(want)), ContentLength: int64(len(want)), Request: request}, nil
	}))
	request := httptestRequest(&url.URL{Scheme: "https", Host: "shop.example.com", Path: "/products.json"})
	request = request.WithContext(WithExecution(request.Context(), recorder, Execution{}))
	response, err := transport.RoundTrip(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	got, err := io.ReadAll(response.Body)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("response bytes=%d err=%v", len(got), err)
	}
	if !recorder.got.Truncated || int64(len(recorder.got.Body)) != MaxBodyBytes {
		t.Fatalf("capture bytes=%d truncated=%v", len(recorder.got.Body), recorder.got.Truncated)
	}
}

func httptestRequest(target *url.URL) *http.Request {
	request, _ := http.NewRequest(http.MethodGet, target.String(), nil)
	return request
}
