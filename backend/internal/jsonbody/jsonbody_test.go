package jsonbody

import (
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
)

func read(body string) (map[string]string, error) {
	r := httptest.NewRequest("POST", "/", strings.NewReader(body))
	object, err := Object(httptest.NewRecorder(), r)
	out := map[string]string{}
	for key, value := range object {
		out[key] = string(value)
	}
	return out, err
}

func TestObjectAcceptsOneObject(t *testing.T) {
	got, err := read(` {"a":1,"b":{"c":[1,{"d":2}]}} ` + "\n")
	if err != nil || got["a"] != "1" || got["b"] != `{"c":[1,{"d":2}]}` {
		t.Fatalf("got=%v err=%v", got, err)
	}
}

func TestObjectRejectsAmbiguousOrNonObjectBodies(t *testing.T) {
	for _, body := range []string{
		``, `   `, `null`, `[]`, `"text"`, `1`,
		`{"a":1,"a":2}`,
		`{"a":{"b":1,"b":2}}`,
		`{"a":[{"b":1,"b":2}]}`,
		`{"a":1} {"b":2}`,
		`{"a":1} x`,
		`{"a":1`,
	} {
		if _, err := read(body); !errors.Is(err, ErrInvalid) {
			t.Errorf("%q: err=%v, want ErrInvalid", body, err)
		}
	}
}

func TestObjectBoundsTheWholeBody(t *testing.T) {
	// Trailing whitespace counts toward the limit.
	padded := `{"a":1}` + strings.Repeat(" ", Limit)
	if _, err := read(padded); !errors.Is(err, ErrLarge) {
		t.Fatalf("err=%v, want ErrLarge", err)
	}
	fits := `{"a":"` + strings.Repeat("x", Limit-len(`{"a":""}`)) + `"}`
	if _, err := read(fits); err != nil {
		t.Fatalf("a body at the limit: %v", err)
	}
}
