package playlists

import (
	"testing"

	"github.com/sglavoie/muse/internal/tui/text"
)

func TestFormatDurationHours(t *testing.T) {
	got := text.FormatDurationMs(3661000)
	if got != "1:01:01" {
		t.Errorf("FormatDurationMs hours: expected 1:01:01, got %s", got)
	}
}

func TestFormatDurationZero(t *testing.T) {
	got := text.FormatDurationMs(0)
	if got != "--:--" {
		t.Errorf("FormatDurationMs 0: expected --:--, got %s", got)
	}
}

func TestFormatDurationNegative(t *testing.T) {
	got := text.FormatDurationMs(-1000)
	if got != "--:--" {
		t.Errorf("FormatDurationMs negative: expected --:--, got %s", got)
	}
}

func TestTruncateZeroMaxLen(t *testing.T) {
	got := text.Truncate("hello", 0)
	if got != "" {
		t.Errorf("Truncate maxLen=0: expected empty, got %q", got)
	}
}

func TestTruncateNegativeMaxLen(t *testing.T) {
	got := text.Truncate("hello", -1)
	if got != "" {
		t.Errorf("Truncate maxLen=-1: expected empty, got %q", got)
	}
}

func TestTruncateMaxLenOne(t *testing.T) {
	got := text.Truncate("hello", 1)
	if got != "…" {
		t.Errorf("Truncate maxLen=1: expected '…', got %q", got)
	}
}

func TestTruncateNoTruncationNeeded(t *testing.T) {
	got := text.Truncate("hi", 5)
	if got != "hi" {
		t.Errorf("Truncate no-op: expected 'hi', got %q", got)
	}
}
