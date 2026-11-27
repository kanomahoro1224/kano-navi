package jsvalue

import (
	"encoding/json"
	"errors"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// FormatNumber mirrors Number.prototype.toString.
func FormatNumber(f float64) string {
	switch {
	case math.IsNaN(f):
		return "NaN"
	case math.IsInf(f, 1):
		return "Infinity"
	case math.IsInf(f, -1):
		return "-Infinity"
	case f == 0:
		return "0"
	}
	abs := math.Abs(f)
	if abs >= 1e-6 && abs < 1e21 {
		return strconv.FormatFloat(f, 'f', -1, 64)
	}
	text := strconv.FormatFloat(f, 'e', -1, 64)
	mantissa, exponent, _ := strings.Cut(text, "e")
	sign := exponent[:1]
	digits := strings.TrimLeft(exponent[1:], "0")
	return mantissa + "e" + sign + digits
}

// String mirrors String(value) for JSON values. Go types that are not JSON
// values, such as int or []string, give "".
func String(value any) string {
	switch v := value.(type) {
	case nil:
		return "null"
	case bool:
		if v {
			return "true"
		}
		return "false"
	case float64:
		return FormatNumber(v)
	case string:
		return v
	case []any:
		parts := make([]string, len(v))
		for i, item := range v {
			if item != nil {
				parts[i] = String(item)
			}
		}
		return strings.Join(parts, ",")
	case *Object, map[string]any:
		return "[object Object]"
	}
	return ""
}

// StringOrEmpty mirrors String(value ?? "").
func StringOrEmpty(value any) string {
	if value == nil {
		return ""
	}
	return String(value)
}

// StringOr mirrors String(value || fallback).
func StringOr(value any, fallback string) string {
	if Truthy(value) {
		return String(value)
	}
	return fallback
}

// MemberString is String(body[key] ?? "") for one member of a request body
// decoded into json.RawMessage values. A missing or malformed member is "".
func MemberString(raw json.RawMessage) string {
	value, err := Parse(raw)
	if err != nil {
		return ""
	}
	return StringOrEmpty(value)
}

// Truthy mirrors JavaScript truthiness.
func Truthy(value any) bool {
	switch v := value.(type) {
	case nil:
		return false
	case bool:
		return v
	case float64:
		return v != 0 && !math.IsNaN(v)
	case string:
		return v != ""
	}
	return true
}

var decimalLiteral = regexp.MustCompile(`^[+-]?(?:\d+\.?\d*(?:[eE][+-]?\d+)?|\.\d+(?:[eE][+-]?\d+)?)$`)

// Number mirrors Number(value) for a present value; NaN reports failure.
func Number(value any) float64 { return NumberOf(value, true) }

// NumberOf mirrors Number(value) for a property read with Object.Get:
// defined=false stands for undefined, which is NaN.
func NumberOf(value any, defined bool) float64 {
	if !defined {
		return math.NaN()
	}
	switch v := value.(type) {
	case nil:
		return 0
	case bool:
		if v {
			return 1
		}
		return 0
	case float64:
		return v
	case string:
		text := Trim(v)
		switch text {
		case "":
			return 0
		case "Infinity", "+Infinity":
			return math.Inf(1)
		case "-Infinity":
			return math.Inf(-1)
		}
		if len(text) > 2 && text[0] == '0' {
			base := 0
			switch text[1] {
			case 'x', 'X':
				base = 16
			case 'o', 'O':
				base = 8
			case 'b', 'B':
				base = 2
			}
			if base != 0 {
				n, err := strconv.ParseUint(text[2:], base, 64)
				if err != nil {
					return math.NaN()
				}
				return float64(n)
			}
		}
		if !decimalLiteral.MatchString(text) {
			return math.NaN()
		}
		n, err := strconv.ParseFloat(text, 64)
		if err != nil && !errors.Is(err, strconv.ErrRange) {
			return math.NaN()
		}
		return n
	case []any:
		return NumberOf(String(v), true)
	}
	return math.NaN()
}

// Finite mirrors Number.isFinite for a number.
func Finite(f float64) bool { return !math.IsNaN(f) && !math.IsInf(f, 0) }
