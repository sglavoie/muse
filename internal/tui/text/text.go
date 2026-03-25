// Package text provides display-width-aware string formatting utilities
// for terminal rendering. All width calculations use ansi.StringWidth
// to correctly handle wide characters (CJK, emoji) and ANSI escape codes.
package text

import (
	"fmt"
	"math"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// Truncate shortens s to at most maxLen display columns, appending "..." when
// truncated. Returns an empty string when maxLen is zero or negative.
func Truncate(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	if ansi.StringWidth(s) <= maxLen {
		return s
	}
	if maxLen <= 1 {
		return "…"
	}
	return ansi.Truncate(s, maxLen, "…")
}

// Pad truncates or right-pads s with spaces to exactly width display columns.
func Pad(s string, width int) string {
	if width <= 0 {
		return ""
	}
	s = Truncate(s, width)
	n := ansi.StringWidth(s)
	if n < width {
		return s + strings.Repeat(" ", width-n)
	}
	return s
}

// FormatDurationSecs converts seconds to M:SS or H:MM:SS format.
// Returns "--:--" when secs is zero, negative, NaN, or infinite.
func FormatDurationSecs(secs float64) string {
	if secs <= 0 || math.IsNaN(secs) || math.IsInf(secs, 0) {
		return "--:--"
	}
	total := int(secs)
	h := total / 3600
	m := (total % 3600) / 60
	s := total % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

// FormatDurationMs converts milliseconds to M:SS or H:MM:SS format.
// Returns "--:--" when ms is zero or negative.
func FormatDurationMs(ms int64) string {
	if ms <= 0 {
		return "--:--"
	}
	return FormatDurationSecs(float64(ms) / 1000)
}

// PadLeft truncates or left-pads s with spaces to exactly width display columns.
func PadLeft(s string, width int) string {
	if width <= 0 {
		return ""
	}
	s = Truncate(s, width)
	n := ansi.StringWidth(s)
	if n < width {
		return strings.Repeat(" ", width-n) + s
	}
	return s
}
