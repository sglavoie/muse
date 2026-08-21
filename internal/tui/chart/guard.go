// Package chart renders small, width-aware text charts for the TUI. Every
// primitive is a pure function split into a plain-text core and a styled
// wrapper. The wrapper only attaches colors, so a styled line always has the
// same display width as its plain counterpart and callers can keep doing their
// width arithmetic on the unstyled string.
package chart

import (
	"math"
	"strings"
	"unicode"
)

// SafeFloat replaces NaN and ±Inf with 0. Every caller-supplied value must
// pass through here before it reaches width arithmetic: a NaN converted to a
// repeat count panics inside strings.Repeat.
//
// A non-finite value means "no usable number" throughout this package and
// always renders as zero, never as a clamped extreme — the same rule
// internal/radio applies. Clamping is reserved for finite values that merely
// fall outside their range.
func SafeFloat(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return v
}

// SafeMagnitude sanitizes v and clamps negatives to 0, for primitives that
// draw a length and have no way to represent a negative one.
func SafeMagnitude(v float64) float64 {
	v = SafeFloat(v)
	if v < 0 {
		return 0
	}
	return v
}

// SafeFraction sanitizes v and clamps the result to [0, 1]. Note the order:
// +Inf becomes 0, not 1, because it is missing data rather than a full bar.
func SafeFraction(v float64) float64 {
	v = SafeFloat(v)
	switch {
	case v < 0:
		return 0
	case v > 1:
		return 1
	}
	return v
}

// SafeWidth clamps a caller-supplied width to a non-negative cell count.
func SafeWidth(w int) int {
	if w < 0 {
		return 0
	}
	return w
}

// SafeRepeat is strings.Repeat with a non-positive count returning "" instead
// of panicking.
func SafeRepeat(s string, n int) string {
	if n <= 0 || s == "" {
		return ""
	}
	return strings.Repeat(s, n)
}

// SafeText makes s safe to embed in a single chart line: a tab becomes one
// space and every other control character is dropped, so no rendered output
// can carry a tab or split one line into two.
func SafeText(s string) string {
	if !strings.ContainsFunc(s, unsafeRune) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r == '\t':
			b.WriteRune(' ')
		case unsafeRune(r):
			// Dropped: newlines, carriage returns, and escape sequences.
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func unsafeRune(r rune) bool {
	return r == '\t' || unicode.IsControl(r)
}
