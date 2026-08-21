package metaexport

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// ErrBadEscape reports a backslash followed by anything other than one of the
// four legal escapes. Repairing it silently would corrupt the value.
var ErrBadEscape = errors.New("invalid escape sequence")

// trimCutset holds the whitespace and invisible characters chat clients
// sprinkle around pasted values: NBSP, zero-width space/joiners, word joiner
// and BOM alongside ordinary spaces.
const trimCutset = " \t\n\r\v\f\u00a0\u200b\u200c\u200d\u2060\ufeff"

// escapeField encodes the only four sequences the format defines, so that a
// value can never contain a raw TAB, LF or CR and one record stays one line.
func escapeField(s string) string {
	if !strings.ContainsAny(s, "\\\t\n\r") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 8)
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case '\\':
			b.WriteString(`\\`)
		case '\t':
			b.WriteString(`\t`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// unescapeField reverses escapeField. Any other escape is an error rather than
// a best-effort repair.
func unescapeField(s string) (string, error) {
	if !strings.ContainsRune(s, '\\') {
		return s, nil
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c != '\\' {
			b.WriteByte(c)
			continue
		}
		i++
		if i >= len(s) {
			return "", fmt.Errorf("%w: trailing backslash", ErrBadEscape)
		}
		switch s[i] {
		case '\\':
			b.WriteByte('\\')
		case 't':
			b.WriteByte('\t')
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		default:
			return "", fmt.Errorf("%w: %q", ErrBadEscape, s[i-1:i+1])
		}
	}
	return b.String(), nil
}

// quotePairs are the symmetric wrappers LLMs add around values.
var quotePairs = [][2]rune{
	{'"', '"'},
	{'\'', '\''},
	{'“', '”'},
	{'‘', '’'},
}

// Cell applies the lenient cleanup an unescaped field gets on import: trim
// surrounding whitespace and invisibles, then unwrap one symmetric quote pair.
// A value that decoded to something containing a TAB keeps its quotes, since
// there the quoting is more likely to be data than decoration.
func Cell(s string) string {
	s = strings.Trim(s, trimCutset)
	if inner, ok := unwrapQuotes(s); ok {
		s = strings.Trim(inner, trimCutset)
	}
	return s
}

func unwrapQuotes(s string) (string, bool) {
	first, fsize := utf8.DecodeRuneInString(s)
	if fsize == 0 || len(s) <= fsize {
		return s, false
	}
	last, lsize := utf8.DecodeLastRuneInString(s)
	for _, p := range quotePairs {
		if first != p[0] || last != p[1] {
			continue
		}
		inner := s[fsize : len(s)-lsize]
		if strings.ContainsRune(inner, '\t') {
			return s, false
		}
		return inner, true
	}
	return s, false
}

// ParsePosition parses a track or disc number. Empty and dash mean unknown and
// yield 0; "3/12" yields 3, mirroring internal/library's exiftool parsing.
// Anything else non-numeric reports false so the caller can skip the row:
// coercing it to 0 would silently erase a real position.
func ParsePosition(s string) (int, bool) {
	s = strings.Trim(s, trimCutset)
	switch s {
	case "", "-", "–", "—":
		return 0, true
	}
	if i := strings.IndexByte(s, '/'); i >= 0 {
		s = strings.Trim(s[:i], trimCutset)
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}
