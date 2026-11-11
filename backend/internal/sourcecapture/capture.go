// Package sourcecapture records bounded HTTP source responses for later
// inspection without changing the source clients' parsing behavior.
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
	"time"
)

const MaxBodyBytes int64 = 4 << 20

type Execution struct {
	RunID, WorkflowID, StepID string
	Attempt                   int
}

type Response struct {
	Source, Endpoint, Method, ContentType string
	StatusCode                            int
	CapturedAt                            time.Time
	Execution
	Body      []byte
	SHA256    string
	Truncated bool
}

type Recorder interface {
	RecordSourceResponse(context.Context, Response) error
}

type requestCapture struct {
	recorder  Recorder
	execution Execution
}

type contextKey struct{}

func WithExecution(ctx context.Context, recorder Recorder, execution Execution) context.Context {
	if recorder == nil {
		return ctx
	}
	return context.WithValue(ctx, contextKey{}, requestCapture{recorder: recorder, execution: execution})
}

type Transport struct {
	Source string
	Next   http.RoundTripper
}

type replayReadCloser struct {
	io.Reader
	io.Closer
}

func NewTransport(source string, next http.RoundTripper) *Transport {
	return &Transport{Source: strings.TrimSpace(source), Next: next}
}

func (t *Transport) RoundTrip(request *http.Request) (*http.Response, error) {
	next := t.Next
	if next == nil {
		next = http.DefaultTransport
	}
	response, err := next.RoundTrip(request)
	if err != nil || response == nil || response.Body == nil {
		return response, err
	}
	capture, ok := request.Context().Value(contextKey{}).(requestCapture)
	if !ok || capture.recorder == nil || t.Source == "" {
		return response, nil
	}

	originalBody := response.Body
	body, readErr := io.ReadAll(io.LimitReader(originalBody, MaxBodyBytes+1))
	response.Body = replayReadCloser{Reader: io.MultiReader(bytes.NewReader(body), originalBody), Closer: originalBody}
	contentType := response.Header.Get("Content-Type")
	if !textual(contentType, body) {
		return response, nil
	}
	truncated := readErr != nil || int64(len(body)) > MaxBodyBytes || response.ContentLength > MaxBodyBytes
	if int64(len(body)) > MaxBodyBytes {
		body = body[:MaxBodyBytes]
	}
	digest := sha256.Sum256(body)
	entry := Response{
		Source: t.Source, Endpoint: endpointKey(t.Source, request.URL), Method: request.Method,
		ContentType: contentType, StatusCode: response.StatusCode, CapturedAt: time.Now().UTC(),
		Execution: capture.execution, Body: body, SHA256: hex.EncodeToString(digest[:]), Truncated: truncated,
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(request.Context()), 2*time.Second)
	defer cancel()
	_ = capture.recorder.RecordSourceResponse(ctx, entry)
	return response, nil
}

func textual(contentType string, body []byte) bool {
	mediaType := strings.ToLower(strings.TrimSpace(strings.SplitN(contentType, ";", 2)[0]))
	switch {
	case strings.HasPrefix(mediaType, "text/"):
		return true
	case mediaType == "application/json", mediaType == "application/xml", mediaType == "application/atom+xml":
		return true
	case strings.HasSuffix(mediaType, "+json"), strings.HasSuffix(mediaType, "+xml"):
		return true
	case mediaType != "":
		return false
	default:
		sniffed := strings.ToLower(http.DetectContentType(body))
		return strings.HasPrefix(sniffed, "text/") || strings.Contains(sniffed, "json") || strings.Contains(sniffed, "xml")
	}
}

func endpointKey(source string, target *url.URL) string {
	if target == nil {
		return source
	}
	parts := strings.Split(target.EscapedPath(), "/")
	for i, part := range parts {
		if dynamicSegment(part) {
			parts[i] = ":id"
		}
	}
	return source + "://" + strings.ToLower(target.Hostname()) + strings.Join(parts, "/")
}

func dynamicSegment(part string) bool {
	if len(part) >= 6 && strings.Trim(part, "0123456789") == "" {
		return true
	}
	if len(part) >= 18 && strings.Trim(part, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-") == "" {
		return true
	}
	return len(part) > 4 && strings.HasPrefix(strings.ToLower(part), "bv")
}
