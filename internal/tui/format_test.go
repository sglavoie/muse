package tui

import "testing"

func TestTrackDisplay(t *testing.T) {
	tests := []struct {
		artist, title, path string
		want                string
	}{
		{"Artist", "Title", "/a.flac", "Artist – Title"},
		{"", "Title", "/a.flac", "Title"},
		{"Artist", "", "/a.flac", "Artist"},
		{"", "", "/a.flac", "/a.flac"},
	}
	for _, tt := range tests {
		got := trackDisplay(tt.artist, tt.title, tt.path)
		if got != tt.want {
			t.Errorf("trackDisplay(%q, %q, %q) = %q, want %q", tt.artist, tt.title, tt.path, got, tt.want)
		}
	}
}

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
