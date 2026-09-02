package tui

import (
	"math"
	"testing"
)

func TestFormatCount(t *testing.T) {
	tests := []struct {
		input int
		want  string
	}{
		{0, "0"},
		{1, "1"},
		{999, "999"},
		{1000, "1,000"},
		{1234, "1,234"},
		{12345, "12,345"},
		{123456, "123,456"},
		{1234567, "1,234,567"},
		{-1, "-1"},
		{-999, "-999"},
		{-1234, "-1,234"},
		{-1234567, "-1,234,567"},
	}
	for _, tt := range tests {
		got := formatCount(tt.input)
		if got != tt.want {
			t.Errorf("formatCount(%d) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// formatCount previously recursed via formatCount(-n), which for math.MinInt
// overflows back to math.MinInt and loops forever. Guard against regressions.
func TestFormatCount_MinInt(t *testing.T) {
	got := formatCount(math.MinInt)
	if got == "" || got[0] != '-' {
		t.Fatalf("formatCount(math.MinInt) = %q; expected non-empty negative", got)
	}
	// Sanity: at least it should contain commas (every 64-bit MinInt has > 3 digits).
	if len(got) < 5 {
		t.Errorf("formatCount(math.MinInt) = %q; expected long comma-formatted output", got)
	}
}
