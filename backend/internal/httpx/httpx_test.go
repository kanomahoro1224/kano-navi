package httpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/yexca/kano-navi/backend/internal/jsonbody"
)

func decode(t *testing.T, w *httptest.ResponseRecorder) map[string]string {
	t.Helper()
	if got := w.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("content type %q", got)
	}
	out := map[string]string{}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("body %q: %v", w.Body, err)
	}
	return out
}

func TestAdminMuxAnswersUnknownPathsAndMethodsInJSON(t *testing.T) {
	mux := NewAdminMux([]Route{{Method: http.MethodGet, Path: "/items/{id}", Handler: func(w http.ResponseWriter, r *http.Request) {
		WriteJSON(w, http.StatusOK, map[string]string{"id": r.PathValue("id")})
	}}})
	serve := func(method, path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(method, path, nil))
		return w
	}
	if w := serve("GET", AdminPrefix+"/items/a1"); w.Code != 200 || decode(t, w)["id"] != "a1" {
		t.Fatalf("route=%d %s", w.Code, w.Body)
	}
	if w := serve("GET", AdminPrefix+"/unknown"); w.Code != 404 || decode(t, w)["error"] != "not_found" {
		t.Fatalf("unknown=%d %s", w.Code, w.Body)
	}
	w := serve("DELETE", AdminPrefix+"/items/a1")
	if w.Code != 405 || decode(t, w)["error"] != "method_not_allowed" || !strings.Contains(w.Header().Get("Allow"), "GET") {
		t.Fatalf("method=%d allow=%q %s", w.Code, w.Header().Get("Allow"), w.Body)
	}
}

func TestWriteErrorKeepsOnlyValidationMessages(t *testing.T) {
	for _, tc := range []struct {
		err          error
		status       int
		code, detail string
	}{
		{Invalid("name is %s", "required"), 400, "invalid_input", "name is required"},
		{fmt.Errorf("wrapped: %w", Invalid("bad")), 400, "invalid_input", "bad"},
		{jsonbody.ErrLarge, 413, "request_too_large", ""},
		{jsonbody.ErrInvalid, 400, "invalid_json", ""},
		{errors.New("synthetic SQL failure at /srv/path"), 500, "internal_error", ""},
	} {
		w := httptest.NewRecorder()
		WriteError(w, tc.err)
		body := decode(t, w)
		if w.Code != tc.status || body["error"] != tc.code || body["message"] != tc.detail {
			t.Errorf("%v: %d %v", tc.err, w.Code, body)
		}
		if strings.Contains(w.Body.String(), "/srv/path") {
			t.Fatal("an internal error leaked its text")
		}
	}
}

func TestBodyTreatsAnAbsentBodyAsAnEmptyObject(t *testing.T) {
	r := httptest.NewRequest("POST", "/", nil)
	got, err := Body(httptest.NewRecorder(), r)
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("got=%v err=%v", got, err)
	}
	r = httptest.NewRequest("POST", "/", strings.NewReader(`{"a":1,"a":2}`))
	if _, err = Body(httptest.NewRecorder(), r); !errors.Is(err, jsonbody.ErrInvalid) {
		t.Fatalf("a present body is still checked: %v", err)
	}
}
