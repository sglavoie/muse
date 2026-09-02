package text

import (
	"testing"
)

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
		got := TrackDisplay(tt.artist, tt.title, tt.path)
		if got != tt.want {
			t.Errorf("trackDisplay(%q, %q, %q) = %q, want %q", tt.artist, tt.title, tt.path, got, tt.want)
		}
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
		got := FormatListeningTime(tt.secs)
		if got != tt.want {
			t.Errorf("formatListeningTime(%d) = %q, want %q", tt.secs, got, tt.want)
		}
	}
}
