package playlists

import (
	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/db"
	"strings"
	"testing"
)

func TestWideningPreservesVisibleSongIdentity(t *testing.T) {
	v := View{Level: TrackLevel, Tracks: []db.PlaylistTrackRow{{Title: "A Song With a Preposterously Long Title", Artist: "The Exceptionally Verbose Orchestra", Album: "An album", DurationMs: 183000}}}
	previous := ComputeTrack(60)
	for width := 60; width <= 160; width++ {
		v.Width = width
		cols := ComputeTrack(width)
		if cols.Title < previous.Title || cols.Artist < previous.Artist {
			t.Fatalf("identity shrank at %d: %+v -> %+v", width, previous, cols)
		}
		for _, line := range strings.Split(ansi.Strip(v.Render()), "\n") {
			if ansi.StringWidth(line) > width {
				t.Fatalf("overflow at %d: %s", width, line)
			}
		}
		if width >= 79 && !strings.Contains(ansi.Strip(v.Render()), v.Tracks[0].Title) {
			t.Fatalf("title lost at %d: %s", width, v.Render())
		}
		previous = cols
	}
}
