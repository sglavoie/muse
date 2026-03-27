package tui

import (
	"testing"
)

func TestSinceUnixForPeriod_AllTime(t *testing.T) {
	got := sinceUnixForPeriod(0) // "All Time" → sinceDays = 0
	if got != 0 {
		t.Errorf("all-time period: got %d, want 0", got)
	}
}

func TestSinceUnixForPeriod_ValidPeriods(t *testing.T) {
	// Ensure each valid period index does not panic and returns a non-negative value.
	for i := range len(statsPeriods) {
		got := sinceUnixForPeriod(i)
		if got < 0 {
			t.Errorf("period %d (%s): got negative unix timestamp %d", i, statsPeriods[i].label, got)
		}
	}
}

func TestSinceUnixForPeriod_OutOfBounds(t *testing.T) {
	// Out-of-bounds indices must not panic and should return 0 (all-time).
	if got := sinceUnixForPeriod(-1); got != 0 {
		t.Errorf("negative index: got %d, want 0", got)
	}
	if got := sinceUnixForPeriod(len(statsPeriods)); got != 0 {
		t.Errorf("past-end index: got %d, want 0", got)
	}
	if got := sinceUnixForPeriod(999); got != 0 {
		t.Errorf("large index: got %d, want 0", got)
	}
}

func TestFormatListeningTime(t *testing.T) {
	tests := []struct {
		secs int
		want string
	}{
		{0, "0s"},
		{-5, "0s"},
		{30, "30s"},
		{60, "1m"},
		{90, "1m"},
		{3600, "1h"},
		{3660, "1h 1m"},
		{7200, "2h"},
		{7260, "2h 1m"},
	}
	for _, tt := range tests {
		got := formatListeningTime(tt.secs)
		if got != tt.want {
			t.Errorf("formatListeningTime(%d) = %q, want %q", tt.secs, got, tt.want)
		}
	}
}
