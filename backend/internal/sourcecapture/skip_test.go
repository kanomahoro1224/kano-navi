package sourcecapture

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestARequestMarkedWithoutCaptureIsNeverRecorded(t *testing.T) {
	recorder := &recordingCapture{}
	transport := NewTransport("bilibili", roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"code":0,"data":{"refresh_token":"synthetic"}}`)), Request: request}, nil
	}))
	target, _ := url.Parse("https://passport.bilibili.com/x/passport-login/web/cookie/refresh")
	request := httptestRequest(target)
	request = request.WithContext(WithoutCapture(WithExecution(request.Context(), recorder, Execution{RunID: "run-1", StepID: "bilibili"})))
	response, err := transport.RoundTrip(request)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if !strings.Contains(string(body), "refresh_token") || recorder.got.Endpoint != "" {
		t.Fatalf("body=%q capture=%+v", body, recorder.got)
	}
}
