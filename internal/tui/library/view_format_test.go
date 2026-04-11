package library

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/tui/text"
)

// TestPadTruncation verifies that pad correctly truncates long strings and
// includes an ellipsis. The result is right-padded to the requested width,
// so the ellipsis may not be the final character.
func TestPadTruncation(t *testing.T) {
	got := text.Pad("Hello World", 7)
	if ansi.StringWidth(got) != 7 {
		t.Errorf("pad: expected 7 display columns, got %d: %q", ansi.StringWidth(got), got)
	}
	if !strings.Contains(got, "…") {
		t.Errorf("pad: expected ellipsis in output, got %q", got)
	}
}

// TestPadPadding verifies that pad right-pads short strings with spaces.
func TestPadPadding(t *testing.T) {
	got := text.Pad("Hi", 5)
	if got != "Hi   " {
		t.Errorf("pad: expected %q, got %q", "Hi   ", got)
	}
}

func TestFormatDurationMinutesSeconds(t *testing.T) {
	got := text.FormatDurationSecs(185)
	if got != "3:05" {
		t.Errorf("FormatDurationSecs(185): expected 3:05, got %s", got)
	}
}

func TestFormatDurationHours(t *testing.T) {
	got := text.FormatDurationSecs(3661)
	if got != "1:01:01" {
		t.Errorf("FormatDurationSecs(3661): expected 1:01:01, got %s", got)
	}
}

func TestFormatDurationZero(t *testing.T) {
	got := text.FormatDurationSecs(0)
	if got != "--:--" {
		t.Errorf("FormatDurationSecs(0): expected --:--, got %s", got)
	}
}

func TestFormatDurationNegative(t *testing.T) {
	got := text.FormatDurationSecs(-5)
	if got != "--:--" {
		t.Errorf("FormatDurationSecs(-5): expected --:--, got %s", got)
	}
}

func TestPadZeroWidth(t *testing.T) {
	got := text.Pad("hello", 0)
	if got != "" {
		t.Errorf("pad width=0: expected empty, got %q", got)
	}
}

func TestPadWidthOne(t *testing.T) {
	got := text.Pad("hello", 1)
	if got != "…" {
		t.Errorf("pad width=1: expected '…', got %q", got)
	}
}

func TestPadExactFit(t *testing.T) {
	got := text.Pad("hi", 2)
	if got != "hi" {
		t.Errorf("pad exact fit: expected 'hi', got %q", got)
	}
}
