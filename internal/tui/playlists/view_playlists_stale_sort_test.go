package playlists

import (
	"strings"
	"testing"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// TestPlaylistsViewRenderStaleSortOrderLengthFallback verifies that
// PlaylistsView.render does not panic and does not render blank rows when
// SortedTrackOrder is shorter than tracks. It must fall back to natural
// order so the user sees real titles.
func TestPlaylistsViewRenderStaleSortOrderLengthFallback(t *testing.T) {
	v := PlaylistsView{
		Level: TrackLevel,
		Tracks: []db.PlaylistTrackRow{
			{ID: 1, Title: "Alpha", Hash: "h1"},
			{ID: 2, Title: "Beta", Hash: "h2"},
			{ID: 3, Title: "Gamma", Hash: "h3"},
		},
		// Stale: shorter than tracks.
		SortedTrackOrder: []int{2, 0},
		Width:            120,
		Height:           20,
	}
	s := styles.NewStyles(true)
	// Must not panic.
	out := v.Render(s, nil, nil, nil, "", "")
	// All three titles must appear (natural order fallback) — no blank rows.
	for _, want := range []string{"Alpha", "Beta", "Gamma"} {
		if !strings.Contains(out, want) {
			t.Errorf("render output missing title %q (stale fallback should show all rows)", want)
		}
	}
}

// TestPlaylistsViewRenderStaleSortOrderIndicesFallback verifies render
// falls back to natural order when SortedTrackOrder has out-of-range
// indices, rather than panicking.
func TestPlaylistsViewRenderStaleSortOrderIndicesFallback(t *testing.T) {
	v := PlaylistsView{
		Level: TrackLevel,
		Tracks: []db.PlaylistTrackRow{
			{ID: 1, Title: "Alpha", Hash: "h1"},
			{ID: 2, Title: "Beta", Hash: "h2"},
		},
		// Length matches but index 5 is out of range for 2 tracks.
		SortedTrackOrder: []int{1, 5},
		Width:            120,
		Height:           20,
	}
	s := styles.NewStyles(true)
	out := v.Render(s, nil, nil, nil, "", "") // must not panic
	if !strings.Contains(out, "Alpha") || !strings.Contains(out, "Beta") {
		t.Errorf("expected both titles in fallback render, got: %s", out)
	}
}
