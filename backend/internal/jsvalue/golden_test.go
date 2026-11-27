package jsvalue

import (
	"cmp"
	"encoding/hex"
	"encoding/json"
	"math"
	"os"
	"reflect"
	"strconv"
	"testing"
	"unicode/utf8"

	"github.com/yexca/kano-navi/backend/internal/digest"
)

// testdata/*.golden.json pin what the three package-local copies that this
// package replaced (schedule, llmadmin, contentadmin) returned for
// testdata/inputs.json. Each copy's callers now use the function named in
// its check below; a row whose result changed on purpose is listed in
// changed with the reason, and must differ from the pinned value.

type goldenText struct {
	S   string `json:"s"`
	Hex string `json:"hex"`
	N   int    `json:"n"`
}

type goldenInputs struct {
	Numbers []string     `json:"numbers"`
	JSON    []string     `json:"json"`
	Trim    []goldenText `json:"trim"`
	Length  []goldenText `json:"length"`
	Slice   []goldenText `json:"slice"`
	Lower   []string     `json:"lower"`
}

func readJSON(t *testing.T, path string, out any) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, out); err != nil {
		t.Fatal(err)
	}
}

func (g goldenText) value(t *testing.T) string {
	if g.Hex == "" {
		return g.S
	}
	raw, err := hex.DecodeString(g.Hex)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// shown keeps invalid UTF-8 visible, as the golden files record it.
func shown(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	return "hex:" + hex.EncodeToString([]byte(s))
}

func numberText(f float64) string {
	if math.IsNaN(f) {
		return "NaN"
	}
	return strconv.FormatFloat(f, 'g', -1, 64)
}

// normalize gives a computed value the shape json.Unmarshal gives the
// golden file, so the two compare with reflect.DeepEqual.
func normalize(t *testing.T, value any) any {
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var out any
	if err = json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// changed maps "copy section" to input -> new result.
var changed = map[string]map[string]any{
	// contentadmin formatted numbers with FormatFloat('f'/'g') and passed
	// unparseable numbers through; String() gives these results.
	"contentadmin string": {
		"-0": "0", "[-0]": "0", "1e-7": "1e-7", "1.5e-7": "1.5e-7",
		"1e400": "Infinity", "-1e400": "-Infinity",
	},
	// contentadmin re-encoded the kept prefix, turning invalid UTF-8 into
	// U+FFFD; Slice keeps the input bytes, as schedule did.
	"contentadmin slice": {"hex:61ff62ff63/2": "hex:61ff"},
	// schedule answered "" for map[string]any, which its parsed values never
	// are; llmadmin reaches String with decoded objects, which are.
	"schedule goTyped": {"map": []any{"[object Object]", true, "null"}},
	// llmadmin answered "[object Object]" for every non-JSON Go type. Its
	// inputs are encoding/json values and Go strings, so only map[string]any
	// is reachable, and that still gives "[object Object]".
	"llmadmin goTyped": {
		"int": []any{"", true}, "int64": []any{"", true}, "raw": []any{"", true},
		"nilStringPtr": []any{"", true}, "stringPtr": []any{"", true},
		"strings": []any{"", true}, "nilStrings": []any{"", true}, "emptyStrings": []any{"", true},
	},
}

type goldenCheck struct {
	t      *testing.T
	copy   string
	pinned map[string]any
}

func (c goldenCheck) section(name string) any {
	value, ok := c.pinned[name]
	if !ok {
		c.t.Fatalf("%s golden has no %s section", c.copy, name)
	}
	return value
}

func (c goldenCheck) value(name string, got any) {
	c.t.Helper()
	if want := c.section(name); !reflect.DeepEqual(normalize(c.t, got), want) {
		c.t.Errorf("%s %s = %v, pinned %v", c.copy, name, got, want)
	}
}

// rows compares one result per input; keys name the inputs for changed.
func (c goldenCheck) rows(name string, keys []string, got []any) {
	c.t.Helper()
	pinned, _ := c.section(name).([]any)
	if len(pinned) != len(got) {
		c.t.Fatalf("%s %s has %d rows, pinned %d", c.copy, name, len(got), len(pinned))
	}
	overrides := changed[c.copy+" "+name]
	for i, row := range got {
		row = normalize(c.t, row)
		if want, ok := overrides[keys[i]]; ok {
			if reflect.DeepEqual(pinned[i], normalize(c.t, want)) {
				c.t.Errorf("%s %s[%s] is listed as changed but matches the pin", c.copy, name, keys[i])
			}
			if !reflect.DeepEqual(row, normalize(c.t, want)) {
				c.t.Errorf("%s %s(%s) = %v, want %v", c.copy, name, keys[i], row, want)
			}
			continue
		}
		if !reflect.DeepEqual(row, pinned[i]) {
			c.t.Errorf("%s %s(%s) = %v, pinned %v", c.copy, name, keys[i], row, pinned[i])
		}
	}
}

// goTyped compares the first columns of the pinned goTyped rows, one row
// per Go value that is not a JSON value.
func (c goldenCheck) goTyped(values map[string]any, columns int, fn func(any) []any) {
	c.t.Helper()
	pinnedRows, _ := c.section("goTyped").(map[string]any)
	if len(pinnedRows) != len(values) {
		c.t.Fatalf("%s goTyped has %d rows, pinned %d", c.copy, len(values), len(pinnedRows))
	}
	names, got, pinned := []string{}, []any{}, []any{}
	for name, value := range values {
		names = append(names, name)
		got = append(got, fn(value))
		pinned = append(pinned, pinnedRows[name].([]any)[:columns])
	}
	sub := goldenCheck{t: c.t, copy: c.copy, pinned: map[string]any{"goTyped": pinned}}
	sub.rows("goTyped", names, got)
}

func loadGolden(t *testing.T, copy string) (goldenInputs, goldenCheck) {
	var in goldenInputs
	readJSON(t, "testdata/inputs.json", &in)
	check := goldenCheck{t: t, copy: copy}
	readJSON(t, "testdata/"+copy+".golden.json", &check.pinned)
	return in, check
}

func textKeys(t *testing.T, items []goldenText) []string {
	keys := make([]string, len(items))
	for i, item := range items {
		keys[i] = shown(item.value(t)) + "/" + strconv.Itoa(item.N)
	}
	return keys
}

// texts runs fn over the text inputs of one section.
func texts(t *testing.T, items []goldenText, fn func(goldenText) any) []any {
	out := make([]any, len(items))
	for i, item := range items {
		out[i] = fn(item)
	}
	return out
}

func numbers(t *testing.T, in goldenInputs) []any {
	out := []any{}
	for _, text := range in.Numbers {
		f, err := strconv.ParseFloat(text, 64)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, FormatNumber(f))
	}
	return out
}

func goTypedValues(raw any) map[string]any {
	text := "a <b>"
	return map[string]any{
		"int": 5, "int64": int64(-3), "raw": raw,
		"nilStringPtr": (*string)(nil), "stringPtr": &text,
		"strings": []string{"a", " ", "<&>"}, "nilStrings": []string(nil), "emptyStrings": []string{},
		"map": map[string]any{"b": 1, "a": 2},
	}
}

func TestScheduleCopyPinned(t *testing.T) {
	in, check := loadGolden(t, "schedule")
	var str, or, truthy, number, stringify []any
	for _, text := range in.JSON {
		value, err := Parse([]byte(text))
		if err != nil {
			t.Fatalf("Parse(%s): %v", text, err)
		}
		str = append(str, shown(String(value)))
		or = append(or, shown(StringOr(value, "fb")))
		truthy = append(truthy, Truthy(value))
		number = append(number, numberText(Number(value)))
		stringify = append(stringify, shown(Stringify(value)))
	}
	check.rows("numberString", in.Numbers, numbers(t, in))
	check.rows("string", in.JSON, str)
	check.rows("stringOrFallback", in.JSON, or)
	check.rows("truthy", in.JSON, truthy)
	check.rows("number", in.JSON, number)
	check.rows("stringify", in.JSON, stringify)
	check.value("numberUndefined", numberText(NumberOf(nil, false)))
	check.rows("trim", textKeys(t, in.Trim), texts(t, in.Trim, func(g goldenText) any { return shown(Trim(g.value(t))) }))
	check.rows("length", textKeys(t, in.Length), texts(t, in.Length, func(g goldenText) any { return Length(g.value(t)) }))
	check.rows("slice", textKeys(t, in.Slice), texts(t, in.Slice, func(g goldenText) any { return shown(Slice(g.value(t), g.N)) }))
	lower := []any{}
	for _, text := range in.Lower {
		lower = append(lower, ToLower(text))
	}
	check.rows("lower", in.Lower, lower)
	check.goTyped(goTypedValues(Raw(`{"x":[1, 2]}`)), 3, func(value any) []any {
		return []any{shown(String(value)), Truthy(value), shown(Stringify(value))}
	})
	check.value("finite", []any{Finite(0), Finite(math.NaN()), Finite(math.Inf(1)), Finite(-1e308)})
	check.value("sha256Hex", digest.SHA256Hex("kano "))
}

func TestLLMAdminCopyPinned(t *testing.T) {
	in, check := loadGolden(t, "llmadmin")
	var str, orEmpty, truthyString, truthy, number []any
	for _, text := range in.JSON {
		// llmadmin reads request members with encoding/json, which refuses
		// out-of-range numbers; those rows are pinned as null.
		var value any
		if json.Unmarshal([]byte(text), &value) != nil {
			str, orEmpty, truthyString, truthy, number = append(str, nil), append(orEmpty, nil), append(truthyString, nil), append(truthy, nil), append(number, nil)
			continue
		}
		str = append(str, shown(String(value)))
		orEmpty = append(orEmpty, shown(StringOrEmpty(value)))
		truthyString = append(truthyString, shown(StringOr(value, "")))
		truthy = append(truthy, Truthy(value))
		number = append(number, numberText(Number(value)))
	}
	check.rows("numberString", in.Numbers, numbers(t, in))
	check.rows("string", in.JSON, str)
	check.rows("stringOrEmpty", in.JSON, orEmpty)
	check.rows("truthyString", in.JSON, truthyString)
	check.rows("truthy", in.JSON, truthy)
	check.rows("number", in.JSON, number)
	check.rows("trim", textKeys(t, in.Trim), texts(t, in.Trim, func(g goldenText) any { return shown(Trim(g.value(t))) }))
	check.rows("length", textKeys(t, in.Length), texts(t, in.Length, func(g goldenText) any { return Length(g.value(t)) }))
	check.rows("slice", textKeys(t, in.Slice), texts(t, in.Slice, func(g goldenText) any { return shown(SliceWhole(g.value(t), g.N)) }))
	// The third column of goTyped and the compactJSON, uniqueStrings and
	// nullableString sections stay in llmadmin and are checked there.
	check.goTyped(goTypedValues(json.RawMessage(`{"x":[1, 2]}`)), 2, func(value any) []any {
		return []any{shown(String(value)), Truthy(value)}
	})
}

func TestContentAdminCopyPinned(t *testing.T) {
	in, check := loadGolden(t, "contentadmin")
	str := []any{}
	for _, text := range in.JSON {
		str = append(str, shown(MemberString(json.RawMessage(text))))
	}
	check.rows("string", in.JSON, str)
	check.value("stringMissing", MemberString(nil))
	check.rows("trim", textKeys(t, in.Trim), texts(t, in.Trim, func(g goldenText) any { return shown(Trim(g.value(t))) }))
	check.rows("length", textKeys(t, in.Length), texts(t, in.Length, func(g goldenText) any { return Length(g.value(t)) }))
	check.rows("slice", textKeys(t, in.Slice), texts(t, in.Slice, func(g goldenText) any { return shown(Slice(g.value(t), g.N)) }))
	check.value("sha256Hex", digest.SHA256Hex([]byte("kano ")))
	check.value("firstNonEmpty", []any{cmp.Or[string](), cmp.Or("", ""), cmp.Or("", "b", "c")})
}

func TestNumberParsesJavaScriptLiterals(t *testing.T) {
	for input, want := range map[string]float64{"": 0, " 42 ": 42, "0x10": 16, "1e3": 1000, ".5": 0.5, "1e-400": 0} {
		if got := Number(input); got != want {
			t.Errorf("Number(%q) = %v, want %v", input, got, want)
		}
	}
	for _, input := range []string{"abc", "1_000", "0x", "1 2", "inf", "0x1p3"} {
		if got := Number(input); !math.IsNaN(got) {
			t.Errorf("Number(%q) = %v, want NaN", input, got)
		}
	}
}
