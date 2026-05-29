package strip

import (
	"strings"

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
	if !strings.ContainsAny(v, "\r\n\t") {
		return v
	}
	return strings.NewReplacer("\r\n", " ", "\r", " ", "\n", " ", "\t", " ").Replace(v)
}

// styledLine preserves caller-rendered ANSI while still removing raw control
// bytes that would break the strip's one-row layout contract.
func styledLine(v string) string {
	v = singleLine(v)
	if !strings.ContainsAny(v, "\x00\x01\x02\x03\x04\x05\x06\x07\b\x0b\f\x0e\x0f\x10\x11\x12\x13\x14\x15\x16\x17\x18\x19\x1a\x1c\x1d\x1e\x1f\x7f") {
		return v
	}
	return strings.Map(func(r rune) rune {
		if r == '\x1b' {
			return r
		}
		if r == '\x7f' || r < ' ' {
			return ' '
		}
		return r
	}, v)
}

// plainLine sanitizes untrusted, plain-text strip fields before they are
// styled. InputView and FilterChunk.Hint intentionally do not use this path
// because callers pass pre-rendered ANSI there for cursors and placeholders.
func plainLine(v string) string {
	v = ansi.Strip(singleLine(v))
	if !strings.ContainsAny(v, "\x00\x01\x02\x03\x04\x05\x06\x07\b\x0b\f\x0e\x0f\x10\x11\x12\x13\x14\x15\x16\x17\x18\x19\x1a\x1b\x1c\x1d\x1e\x1f\x7f") {
		return v
	}
	return strings.Map(func(r rune) rune {
		if r == '\x7f' || r < ' ' {
			return ' '
		}
		return r
	}, v)
}

func plainHintLine(v string) string {
	return strings.Join(strings.Fields(plainLine(v)), " ")
}
