package jsvalue

import (
	"strings"
	"unicode"
)

// IsSpace reports whether String.prototype.trim removes r.
func IsSpace(r rune) bool {
	switch r {
	case '\t', '\n', '\v', '\f', '\r', ' ', 0x00a0, 0x1680, 0x2028, 0x2029, 0x202f, 0x205f, 0x3000, 0xfeff:
		return true
	}
	return r >= 0x2000 && r <= 0x200a
}

// Trim mirrors String.prototype.trim, whose whitespace set differs from
// strings.TrimSpace (U+FEFF is trimmed, U+0085 is not).
func Trim(s string) string { return strings.TrimFunc(s, IsSpace) }

// Length mirrors String.prototype.length: UTF-16 code units.
func Length(s string) int {
	n := 0
	for _, r := range s {
		if r > 0xffff {
			n += 2
		} else {
			n++
		}
	}
	return n
}

// Slice mirrors value.slice(0, limit), which counts UTF-16 code units. A
// surrogate pair cut in half becomes U+FFFD, matching how the lone surrogate
// is encoded when a JavaScript string is hashed or stored as UTF-8.
func Slice(s string, limit int) string {
	if limit <= 0 {
		return ""
	}
	units := 0
	for i, r := range s {
		width := 1
		if r > 0xffff {
			width = 2
		}
		if units+width > limit {
			if units+1 == limit && width == 2 {
				return s[:i] + "�"
			}
			return s[:i]
		}
		units += width
	}
	return s
}

// SliceWhole is Slice that never splits a surrogate pair: a pair cut in half
// is dropped instead of becoming U+FFFD, so the result holds only whole
// characters. Use Slice where the stored value must match JavaScript.
func SliceWhole(s string, limit int) string {
	units := 0
	for i, r := range s {
		width := 1
		if r > 0xffff {
			width = 2
		}
		if units+width > limit {
			return s[:i]
		}
		units += width
	}
	return s
}

func isCased(r rune) bool {
	return unicode.IsUpper(r) || unicode.IsLower(r) || unicode.IsTitle(r)
}

func isCaseIgnorable(r rune) bool {
	switch r {
	case '\'', '.', ':', '^', '`', 0x00a8, 0x00ad, 0x00af, 0x00b4, 0x00b7, 0x00b8, 0x2018, 0x2019, 0x2024, 0x2027, 0xfe13, 0xfe52, 0xfe55, 0xff07, 0xff0e, 0xff1a:
		return true
	}
	return unicode.In(r, unicode.Mn, unicode.Me, unicode.Cf, unicode.Lm, unicode.Sk)
}

// ToLower mirrors String.prototype.toLowerCase, including the two
// SpecialCasing rules that simple per-rune mapping misses.
func ToLower(s string) string {
	special := false
	for _, r := range s {
		if r == 0x0130 || r == 0x03a3 {
			special = true
			break
		}
	}
	if !special {
		return strings.Map(unicode.ToLower, s)
	}
	runes := []rune(s)
	var b strings.Builder
	for i, r := range runes {
		switch r {
		case 0x0130:
			b.WriteString("i̇")
		case 0x03a3:
			before := false
			for j := i - 1; j >= 0; j-- {
				if isCaseIgnorable(runes[j]) {
					continue
				}
				before = isCased(runes[j])
				break
			}
			after := false
			for j := i + 1; j < len(runes); j++ {
				if isCaseIgnorable(runes[j]) {
					continue
				}
				after = isCased(runes[j])
				break
			}
			if before && !after {
				b.WriteRune(0x03c2)
			} else {
				b.WriteRune(0x03c3)
			}
		default:
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String()
}
