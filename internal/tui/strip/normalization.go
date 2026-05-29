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

// styledLine preserves caller-rendered SGR styling while still removing raw
// control bytes that would break the strip's one-row layout contract.
func styledLine(v string) string {
	v = singleLine(v)
	if !strings.ContainsAny(v, "\x00\x01\x02\x03\x04\x05\x06\x07\b\x0b\f\x0e\x0f\x10\x11\x12\x13\x14\x15\x16\x17\x18\x19\x1a\x1b\x1c\x1d\x1e\x1f\x7f") {
		return v
	}

	var b strings.Builder
	b.Grow(len(v))
	for i := 0; i < len(v); {
		c := v[i]
		if c == '\x1b' {
			if end, ok := sgrSequenceEnd(v, i); ok {
				b.WriteString(v[i:end])
				i = end
				continue
			}
			b.WriteByte(' ')
			i++
			continue
		}
		if c == '\x7f' || c < ' ' {
			b.WriteByte(' ')
			i++
			continue
		}
		r, size := utf8.DecodeRuneInString(v[i:])
		if unicode.IsControl(r) || (r == utf8.RuneError && size == 1 && c >= 0x80 && c <= 0x9f) {
			b.WriteByte(' ')
			i += size
			continue
		}
		b.WriteString(v[i : i+size])
		i += size
	}
	return b.String()
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
	if !strings.ContainsAny(v, "\x00\x01\x02\x03\x04\x05\x06\x07\b\x0b\f\x0e\x0f\x10\x11\x12\x13\x14\x15\x16\x17\x18\x19\x1a\x1b\x1c\x1d\x1e\x1f\x7f\u0080\u0081\u0082\u0083\u0084\u0085\u0086\u0087\u0088\u0089\u008a\u008b\u008c\u008d\u008e\u008f\u0090\u0091\u0092\u0093\u0094\u0095\u0096\u0097\u0098\u0099\u009a\u009b\u009c\u009d\u009e\u009f") {
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
