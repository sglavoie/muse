package strip

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

func normalizeBreadcrumb(breadcrumb []string) []string {
	cleaned := breadcrumb[:0:0]
	for _, c := range breadcrumb {
		c = plainLine(c)
		if strings.TrimSpace(ansi.Strip(c)) != "" {
			cleaned = append(cleaned, c)
		}
	}
	return cleaned
}

// singleLine keeps metadata-derived labels and pasted filter text from
// breaking the strip's one-row layout contract.
func singleLine(v string) string {
	if !strings.ContainsAny(v, "\r\n\t\u2028\u2029") {
		return v
	}
	return strings.NewReplacer(
		"\r\n", " ",
		"\r", " ",
		"\n", " ",
		"\t", " ",
		"\u2028", " ",
		"\u2029", " ",
	).Replace(v)
}

// asciiControlChars are the C0 controls plus DEL, minus the line breaks and
// tabs singleLine has already folded to spaces by the time either sanitizer
// scans for them.
const asciiControlChars = "\x00\x01\x02\x03\x04\x05\x06\x07\b\x0b\f\x0e\x0f\x10\x11\x12\x13\x14\x15\x16\x17\x18\x19\x1a\x1b\x1c\x1d\x1e\x1f\x7f"

// c1ControlChars are the C1 controls. Only plainLine's guard screens for them;
// styledLine's covers the ASCII controls alone and blanks any C1 byte it meets
// on the slow path that guard opens.
const c1ControlChars = "\u0080\u0081\u0082\u0083\u0084\u0085\u0086\u0087\u0088\u0089\u008a\u008b\u008c\u008d\u008e\u008f\u0090\u0091\u0092\u0093\u0094\u0095\u0096\u0097\u0098\u0099\u009a\u009b\u009c\u009d\u009e\u009f"

// styledLine preserves caller-rendered SGR styling while still removing raw
// control bytes that would break the strip's one-row layout contract.
func styledLine(v string) string {
	v = singleLine(v)
	if !strings.ContainsAny(v, asciiControlChars) {
		return v
	}
	return blankControlsKeepingSGR(v)
}

// blankControlsKeepingSGR replaces every control byte with a space, except the
// bytes of a complete SGR escape sequence, which are copied through verbatim.
func blankControlsKeepingSGR(v string) string {
	var b strings.Builder
	b.Grow(len(v))
	for i := 0; i < len(v); {
		c := v[i]
		if c == '\x1b' {
			i += writeEscape(&b, v, i)
			continue
		}
		if c == '\x7f' || c < ' ' {
			b.WriteByte(' ')
			i++
			continue
		}
		r, size := utf8.DecodeRuneInString(v[i:])
		if isControlRune(r, size, c) {
			b.WriteByte(' ')
			i += size
			continue
		}
		b.WriteString(v[i : i+size])
		i += size
	}
	return b.String()
}

// writeEscape copies the SGR sequence starting at v[i] through unchanged and
// reports how many bytes it consumed. Any other escape is a single byte
// replaced by a space, so a truncated or non-SGR sequence cannot survive.
func writeEscape(b *strings.Builder, v string, i int) int {
	if end, ok := sgrSequenceEnd(v, i); ok {
		b.WriteString(v[i:end])
		return end - i
	}
	b.WriteByte(' ')
	return 1
}

// isControlRune reports whether the rune decoded at a byte offset must be
// blanked: any Unicode control, plus a lone C1 byte that failed to decode as
// UTF-8 and would otherwise pass through as a raw control byte.
func isControlRune(r rune, size int, first byte) bool {
	if unicode.IsControl(r) {
		return true
	}
	return r == utf8.RuneError && size == 1 && first >= 0x80 && first <= 0x9f
}

func sgrSequenceEnd(v string, start int) (int, bool) {
	if start+2 >= len(v) || v[start+1] != '[' {
		return 0, false
	}
	for i := start + 2; i < len(v); i++ {
		c := v[i]
		if c == '\x7f' || c < ' ' {
			return 0, false
		}
		if c >= 0x40 && c <= 0x7e {
			return i + 1, c == 'm'
		}
	}
	return 0, false
}

// plainLine sanitizes untrusted, plain-text strip fields before they are
// styled. InputView and FilterChunk.Hint intentionally do not use this path
// because callers pass pre-rendered ANSI there for cursors and placeholders.
func plainLine(v string) string {
	v = ansi.Strip(singleLine(v))
	if !strings.ContainsAny(v, asciiControlChars+c1ControlChars) {
		return v
	}
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, v)
}

func plainHintLine(v string) string {
	return strings.Join(strings.Fields(plainLine(v)), " ")
}
