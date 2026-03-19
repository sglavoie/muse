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
