package tui

import "testing"

// TestParseDurationFromEditor verifies correct parsing of duration strings.
func TestParseDurationFromEditor(t *testing.T) {
	tests := []struct {
		input   string
		want    float64
		wantErr bool
	}{
		{"", 0, false},
		{"  ", 0, false},
		{"0:00", 0, false},
		{"3:45", 225, false},
		{"1:02:30", 3750, false},
		{"10:00", 600, false},
		{"1:00:00", 3600, false},
		{"abc", 0, true},
		{"1:60", 0, true},
		{"-1:00", 0, true},
		{"1:2:3:4", 0, true},
	}
	for _, tt := range tests {
		got, err := parseDurationFromEditor(tt.input)
		if tt.wantErr {
			if err == nil {
				t.Errorf("parseDurationFromEditor(%q): expected error, got nil (value=%v)", tt.input, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseDurationFromEditor(%q): unexpected error: %v", tt.input, err)
		}
		if got != tt.want {
			t.Errorf("parseDurationFromEditor(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

// TestFormatDurationForEditor verifies correct formatting of duration values.
func TestFormatDurationForEditor(t *testing.T) {
	tests := []struct {
		secs float64
		want string
	}{
		{0, ""},
		{-1, ""},
		{225, "3:45"},
		{3750, "1:02:30"},
		{60, "1:00"},
		{3661, "1:01:01"},
	}
	for _, tt := range tests {
		got := formatDurationForEditor(tt.secs)
		if got != tt.want {
			t.Errorf("formatDurationForEditor(%v) = %q, want %q", tt.secs, got, tt.want)
		}
	}
}

// TestDurationFieldPopulated verifies that NewMetadataEditor pre-populates
// the Duration field when a non-zero duration is provided.
func TestDurationFieldPopulated(t *testing.T) {
	ed := NewMetadataEditor("h1", "T", "A", "B", "G", 225)
	if ed.Duration() != "3:45" {
		t.Errorf("expected Duration()=%q, got %q", "3:45", ed.Duration())
	}
}

// TestDurationFieldEmptyWhenZero verifies that NewMetadataEditor leaves the
// Duration field empty when duration is 0.
func TestDurationFieldEmptyWhenZero(t *testing.T) {
	ed := NewMetadataEditor("h1", "T", "A", "B", "G", 0)
	if ed.Duration() != "" {
		t.Errorf("expected empty Duration(), got %q", ed.Duration())
	}
}
