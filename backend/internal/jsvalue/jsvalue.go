// Package jsvalue reproduces the JavaScript value semantics that the Go
// backend inherited from the original JavaScript server: JSON.parse and
// JSON.stringify, String(), Number(), truthiness, trim(), length, slice() and
// toLowerCase(). Stored fingerprints, stored rows and admin request coercion
// depend on these exact semantics, so every package uses this one copy
// instead of keeping its own.
//
// JSON values are modelled as nil (null), bool, float64, string, []any and
// *Object, which keeps insertion order. A missing property is reported
// through the second return value of Get, which stands in for JavaScript's
// undefined. Values decoded by encoding/json (map[string]any) are accepted
// wherever String, Truthy and Number take a value.
package jsvalue

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math"
	"sort"
	"strconv"
	"unicode/utf8"
)

type member struct {
	key   string
	value any
}

// Object is a JSON object that keeps JavaScript's property order.
type Object struct {
	members []member
	index   map[string]int
}

// NewObject returns an empty object.
func NewObject() *Object { return &Object{index: map[string]int{}} }

// Obj builds an ordered object from alternating key/value arguments.
func Obj(pairs ...any) *Object {
	o := NewObject()
	for i := 0; i+1 < len(pairs); i += 2 {
		o.Set(pairs[i].(string), pairs[i+1])
	}
	return o
}

// Set keeps the first position of a repeated key, as JavaScript does.
func (o *Object) Set(key string, value any) {
	if i, ok := o.index[key]; ok {
		o.members[i].value = value
		return
	}
	o.index[key] = len(o.members)
	o.members = append(o.members, member{key, value})
}

// Get returns a property; false stands for undefined.
func (o *Object) Get(key string) (any, bool) {
	if o == nil {
		return nil, false
	}
	i, ok := o.index[key]
	if !ok {
		return nil, false
	}
	return o.members[i].value, true
}

// arrayIndex reports whether a property name is a canonical array index.
// JavaScript enumerates those before the other keys, in numeric order.
func arrayIndex(key string) (uint64, bool) {
	if key == "" || len(key) > 10 || (len(key) > 1 && key[0] == '0') {
		return 0, false
	}
	for i := 0; i < len(key); i++ {
		if key[i] < '0' || key[i] > '9' {
			return 0, false
		}
	}
	n, err := strconv.ParseUint(key, 10, 64)
	if err != nil || n >= 4294967295 {
		return 0, false
	}
	return n, true
}

func (o *Object) ordered() []member {
	numeric := false
	for _, m := range o.members {
		if _, ok := arrayIndex(m.key); ok {
			numeric = true
			break
		}
	}
	if !numeric {
		return o.members
	}
	type indexed struct {
		n uint64
		m member
	}
	var first []indexed
	var rest []member
	for _, m := range o.members {
		if n, ok := arrayIndex(m.key); ok {
			first = append(first, indexed{n, m})
		} else {
			rest = append(rest, m)
		}
	}
	sort.SliceStable(first, func(i, j int) bool { return first[i].n < first[j].n })
	out := make([]member, 0, len(o.members))
	for _, item := range first {
		out = append(out, item.m)
	}
	return append(out, rest...)
}

// Raw is emitted verbatim by Stringify. It carries fixed schemas.
type Raw string

var errInvalidJSON = errors.New("invalid JSON")

// Parse mirrors JSON.parse for well-formed UTF-8 input. Objects become
// *Object and numbers float64; an out-of-range number becomes ±Infinity.
func Parse(data []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	value, err := decodeJSONValue(dec)
	if err != nil {
		return nil, errInvalidJSON
	}
	if _, err = dec.Token(); err != io.EOF {
		return nil, errInvalidJSON
	}
	return value, nil
}

func decodeJSONValue(dec *json.Decoder) (any, error) {
	token, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch value := token.(type) {
	case json.Delim:
		switch value {
		case '{':
			out := NewObject()
			for dec.More() {
				keyToken, err := dec.Token()
				if err != nil {
					return nil, err
				}
				key, ok := keyToken.(string)
				if !ok {
					return nil, errInvalidJSON
				}
				item, err := decodeJSONValue(dec)
				if err != nil {
					return nil, err
				}
				out.Set(key, item)
			}
			if _, err = dec.Token(); err != nil {
				return nil, err
			}
			return out, nil
		case '[':
			out := []any{}
			for dec.More() {
				item, err := decodeJSONValue(dec)
				if err != nil {
					return nil, err
				}
				out = append(out, item)
			}
			if _, err = dec.Token(); err != nil {
				return nil, err
			}
			return out, nil
		}
		return nil, errInvalidJSON
	case json.Number:
		number, err := strconv.ParseFloat(string(value), 64)
		if err != nil && !errors.Is(err, strconv.ErrRange) {
			return nil, errInvalidJSON
		}
		return number, nil
	case string, bool, nil:
		return value, nil
	}
	return nil, errInvalidJSON
}

// Stringify mirrors JSON.stringify without indentation. Besides the JSON
// values it writes int, int64, Raw, *string and []string; any other Go type
// becomes null.
func Stringify(value any) string {
	return string(appendJSON(nil, value))
}

func appendJSON(b []byte, value any) []byte {
	switch v := value.(type) {
	case nil:
		return append(b, "null"...)
	case bool:
		if v {
			return append(b, "true"...)
		}
		return append(b, "false"...)
	case string:
		return appendQuoted(b, v)
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return append(b, "null"...)
		}
		return append(b, FormatNumber(v)...)
	case int:
		return strconv.AppendInt(b, int64(v), 10)
	case int64:
		return strconv.AppendInt(b, v, 10)
	case Raw:
		return append(b, v...)
	case *string:
		if v == nil {
			return append(b, "null"...)
		}
		return appendQuoted(b, *v)
	case []string:
		b = append(b, '[')
		for i, item := range v {
			if i > 0 {
				b = append(b, ',')
			}
			b = appendQuoted(b, item)
		}
		return append(b, ']')
	case []any:
		b = append(b, '[')
		for i, item := range v {
			if i > 0 {
				b = append(b, ',')
			}
			b = appendJSON(b, item)
		}
		return append(b, ']')
	case *Object:
		if v == nil {
			return append(b, "null"...)
		}
		b = append(b, '{')
		for i, m := range v.ordered() {
			if i > 0 {
				b = append(b, ',')
			}
			b = appendQuoted(b, m.key)
			b = append(b, ':')
			b = appendJSON(b, m.value)
		}
		return append(b, '}')
	}
	return append(b, "null"...)
}

const lowerHex = "0123456789abcdef"

// appendQuoted follows ECMAScript QuoteJSONString: only quotes, backslashes
// and C0 controls are escaped. HTML characters and U+2028/9 stay literal.
func appendQuoted(b []byte, s string) []byte {
	b = append(b, '"')
	for _, r := range s {
		switch r {
		case '"':
			b = append(b, '\\', '"')
		case '\\':
			b = append(b, '\\', '\\')
		case '\b':
			b = append(b, '\\', 'b')
		case '\f':
			b = append(b, '\\', 'f')
		case '\n':
			b = append(b, '\\', 'n')
		case '\r':
			b = append(b, '\\', 'r')
		case '\t':
			b = append(b, '\\', 't')
		default:
			if r < 0x20 {
				b = append(b, '\\', 'u', '0', '0', lowerHex[r>>4], lowerHex[r&0xf])
			} else {
				b = utf8.AppendRune(b, r)
			}
		}
	}
	return append(b, '"')
}
