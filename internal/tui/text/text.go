// Package text provides display-width-aware string formatting utilities
// for terminal rendering. All width calculations use ansi.StringWidth
// to correctly handle wide characters (CJK, emoji) and ANSI escape codes.
package text

import (
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
	return ansi.Truncate(s, maxLen-1, "…")
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
