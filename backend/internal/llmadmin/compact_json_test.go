package llmadmin

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
	"unicode/utf8"
)

// The llmadmin golden file in internal/jsvalue/testdata also pins the
// helpers that stayed here when the JavaScript-semantics copy moved to
// internal/jsvalue; this test checks those sections.
func TestRowHelpersPinned(t *testing.T) {
	var inputs struct {
		JSON []string `json:"json"`
	}
	var pinned map[string]any
	for path, out := range map[string]any{"../jsvalue/testdata/inputs.json": &inputs, "../jsvalue/testdata/llmadmin.golden.json": &pinned} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err = json.Unmarshal(raw, out); err != nil {
			t.Fatal(err)
		}
	}
	normalize := func(value any) any {
		raw, _ := json.Marshal(value)
		var out any
		_ = json.Unmarshal(raw, &out)
		return out
	}
	shown := func(s string) string {
		if !utf8.ValidString(s) {
			t.Fatalf("invalid UTF-8 from compactJSON: %q", s)
		}
		return s
	}
	compact := []any{}
	for _, text := range inputs.JSON {
		var value any
		if json.Unmarshal([]byte(text), &value) != nil {
			compact = append(compact, nil)
			continue
		}
		compact = append(compact, shown(compactJSON(value)))
	}
	text := "a <b>"
	goTyped := map[string]any{
		"int": 5, "int64": int64(-3), "raw": json.RawMessage(`{"x":[1, 2]}`),
		"nilStringPtr": (*string)(nil), "stringPtr": &text,
		"strings": []string{"a", " ", "<&>"}, "nilStrings": []string(nil), "emptyStrings": []string{},
		"map": map[string]any{"b": 1, "a": 2},
	}
	pinnedGoTyped, _ := pinned["goTyped"].(map[string]any)
	if len(pinnedGoTyped) != len(goTyped) {
		t.Fatalf("goTyped has %d rows, pinned %d", len(goTyped), len(pinnedGoTyped))
	}
	for name, value := range goTyped {
		if got, want := compactJSON(value), pinnedGoTyped[name].([]any)[2]; got != want {
			t.Errorf("compactJSON(%s) = %s, pinned %v", name, got, want)
		}
	}
	for name, got := range map[string]any{
		"compactJSON":    compact,
		"uniqueStrings":  uniqueStrings([]string{"b", "a", "b", "", ""}),
		"nullableString": []any{nullableString(""), *nullableString("x")},
	} {
		if !reflect.DeepEqual(normalize(got), pinned[name]) {
			t.Errorf("%s = %v, pinned %v", name, got, pinned[name])
		}
	}
}
