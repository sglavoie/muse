package metadata

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/textinput"
	"github.com/charmbracelet/x/ansi"
)

func TestInputOverflowShowsHiddenSidesWithoutChangingValueOrCursor(t *testing.T) {
	input := textinput.New()
	input.Prompt = ""
	input.Focus()
	value := "Beginning of a sufficiently long metadata value ending here"
	input.SetValue(value)
	for _, tc := range []struct {
		cursor      int
		left, right bool
	}{{0, false, true}, {30, true, true}, {len(value), true, false}} {
		input.SetCursor(tc.cursor)
		for _, width := range []int{16, 24, 16} {
			out := ansi.Strip(inputWithOverflow(input, width))
			if strings.HasPrefix(out, "‹") != tc.left || strings.HasSuffix(out, "›") != tc.right {
				t.Fatalf("cursor %d, width %d: %q", tc.cursor, width, out)
			}
			if ansi.StringWidth(out) != width || input.Value() != value || input.Position() != tc.cursor {
				t.Fatalf("render changed field or exceeded width: %q", out)
			}
		}
	}
}

func TestInputOverflowKeepsUnicodeGraphemesAndShortValues(t *testing.T) {
	for _, value := range []string{"Short title", strings.Repeat("全角é🎵", 20)} {
		input := textinput.New()
		input.Prompt = ""
		input.Focus()
		input.SetValue(value)
		out := ansi.Strip(inputWithOverflow(input, 24))
		if ansi.StringWidth(out) != 24 || strings.Contains(out, "�") {
			t.Fatalf("invalid viewport: %q", out)
		}
		if value == "Short title" && (strings.ContainsAny(out, "‹›") || !strings.Contains(out, value)) {
			t.Fatalf("short value truncated: %q", out)
		}
	}
}
