package shop

import "strings"

// Shopify text sometimes carries a kana and its voiced or semi-voiced sound
// mark as two code points (ホ + U+3099 for ボ), as text written on macOS
// often does, while other titles of the same shop use the precomposed kana.
// composedKana is the canonical composition of every kana with U+3099 or
// U+309A, so composeKana gives what Unicode NFC gives for these marks. The
// table is complete: no other character composes with them, and Unicode
// never changes a canonical composition. Other decomposed text, such as
// accented Latin letters, is left as it is.
var composedKana = map[[2]rune]rune{
	// The voiced sound mark U+3099.
	{'う', '\u3099'}: 'ゔ', {'か', '\u3099'}: 'が', {'き', '\u3099'}: 'ぎ', {'く', '\u3099'}: 'ぐ',
	{'け', '\u3099'}: 'げ', {'こ', '\u3099'}: 'ご', {'さ', '\u3099'}: 'ざ', {'し', '\u3099'}: 'じ',
	{'す', '\u3099'}: 'ず', {'せ', '\u3099'}: 'ぜ', {'そ', '\u3099'}: 'ぞ', {'た', '\u3099'}: 'だ',
	{'ち', '\u3099'}: 'ぢ', {'つ', '\u3099'}: 'づ', {'て', '\u3099'}: 'で', {'と', '\u3099'}: 'ど',
	{'は', '\u3099'}: 'ば', {'ひ', '\u3099'}: 'び', {'ふ', '\u3099'}: 'ぶ', {'へ', '\u3099'}: 'べ',
	{'ほ', '\u3099'}: 'ぼ', {'ゝ', '\u3099'}: 'ゞ', {'ウ', '\u3099'}: 'ヴ', {'カ', '\u3099'}: 'ガ',
	{'キ', '\u3099'}: 'ギ', {'ク', '\u3099'}: 'グ', {'ケ', '\u3099'}: 'ゲ', {'コ', '\u3099'}: 'ゴ',
	{'サ', '\u3099'}: 'ザ', {'シ', '\u3099'}: 'ジ', {'ス', '\u3099'}: 'ズ', {'セ', '\u3099'}: 'ゼ',
	{'ソ', '\u3099'}: 'ゾ', {'タ', '\u3099'}: 'ダ', {'チ', '\u3099'}: 'ヂ', {'ツ', '\u3099'}: 'ヅ',
	{'テ', '\u3099'}: 'デ', {'ト', '\u3099'}: 'ド', {'ハ', '\u3099'}: 'バ', {'ヒ', '\u3099'}: 'ビ',
	{'フ', '\u3099'}: 'ブ', {'ヘ', '\u3099'}: 'ベ', {'ホ', '\u3099'}: 'ボ', {'ワ', '\u3099'}: 'ヷ',
	{'ヰ', '\u3099'}: 'ヸ', {'ヱ', '\u3099'}: 'ヹ', {'ヲ', '\u3099'}: 'ヺ', {'ヽ', '\u3099'}: 'ヾ',
	// The semi-voiced sound mark U+309A.
	{'は', '\u309a'}: 'ぱ', {'ひ', '\u309a'}: 'ぴ', {'ふ', '\u309a'}: 'ぷ', {'へ', '\u309a'}: 'ぺ',
	{'ほ', '\u309a'}: 'ぽ', {'ハ', '\u309a'}: 'パ', {'ヒ', '\u309a'}: 'ピ', {'フ', '\u309a'}: 'プ',
	{'ヘ', '\u309a'}: 'ペ', {'ホ', '\u309a'}: 'ポ',
}

// composeKana joins each kana and a following combining voiced or
// semi-voiced sound mark into the precomposed kana.
func composeKana(value string) string {
	if !strings.ContainsAny(value, "\u3099\u309a") {
		return value
	}
	runes := []rune(value)
	out := make([]rune, 0, len(runes))
	for _, r := range runes {
		if n := len(out); n > 0 && (r == '\u3099' || r == '\u309a') {
			if composed, ok := composedKana[[2]rune{out[n-1], r}]; ok {
				out[n-1] = composed
				continue
			}
		}
		out = append(out, r)
	}
	return string(out)
}
