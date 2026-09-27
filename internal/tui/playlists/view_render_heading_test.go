package playlists

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// TestTrackLevelHeadingNamesOpenPlaylist guards that an open playlist's
// heading names it, "Playlists — <name>", the way Tags heads a tag's tracks,
// rather than a bare "Tracks" — including when the list was sorted before the
// playlist was opened.
func TestTrackLevelHeadingNamesOpenPlaylist(t *testing.T) {
	v := PlaylistsView{
		Playlists: []db.PlaylistRow{{ID: 1, Name: "Workout"}, {ID: 2, Name: "Chill"}},
		Width:     80,
		Height:    20,
	}
	v.SortedPlaylists = []db.PlaylistRow{v.Playlists[1], v.Playlists[0]}
	if got, ok := v.FocusedPlaylist(); !ok || got.Name != "Chill" {
		t.Fatalf("precondition: sorted list should focus Chill first, got %+v", got)
	}
	v = v.OpenTracks([]db.PlaylistTrackRow{{ID: 10, Title: "Song A", Artist: "X", DurationMs: 60000}})

	out := ansi.Strip(v.Render(styles.NewStyles(true), nil, nil, nil, "", ""))
	if !strings.Contains(out, "Playlists — Chill") {
		t.Errorf("expected heading %q, got:\n%s", "Playlists — Chill", out)
	}
}

// TestTrackLevelHeadingFitsWidth guards that a long playlist name cannot push
// the heading past the view width.
func TestTrackLevelHeadingFitsWidth(t *testing.T) {
	v := View{
		Level:        TrackLevel,
		Width:        30,
		playlistName: "A Playlist Name That Is Far Too Long To Fit",
		Tracks:       []db.PlaylistTrackRow{{ID: 1, Title: "Song A", Artist: "X", DurationMs: 60000}},
	}
	first, _, _ := strings.Cut(ansi.Strip(v.Render()), "\n")
	if w := ansi.StringWidth(first); w > 30 {
		t.Errorf("heading is %d columns wide, want <= 30: %q", w, first)
	}
}
