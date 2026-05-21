package tui

import (
	"strings"
	"testing"

	"github.com/sglavoie/muse/internal/db"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// TestPlaylistsViewRenderStaleSortOrderLengthFallback verifies that
// PlaylistsView.render does not panic and does not render blank rows when
// sortedTrackOrder is shorter than tracks. It must fall back to natural
// order so the user sees real titles.
func TestPlaylistsViewRenderStaleSortOrderLengthFallback(t *testing.T) {
	v := PlaylistsView{
		level: pltui.TrackLevel,
		tracks: []db.PlaylistTrackRow{
			{ID: 1, Title: "Alpha", Hash: "h1"},
			{ID: 2, Title: "Beta", Hash: "h2"},
			{ID: 3, Title: "Gamma", Hash: "h3"},
		},
		// Stale: shorter than tracks.
		sortedTrackOrder: []int{2, 0},
		width:            120,
		height:           20,
	}
	s := styles.NewStyles(true)
	// Must not panic.
	out := v.render(s, nil, nil, nil, "", "")
	// All three titles must appear (natural order fallback) — no blank rows.
	for _, want := range []string{"Alpha", "Beta", "Gamma"} {
		if !strings.Contains(out, want) {
			t.Errorf("render output missing title %q (stale fallback should show all rows)", want)
		}
	}
}

// TestPlaylistsViewRenderStaleSortOrderIndicesFallback verifies render
// falls back to natural order when sortedTrackOrder has out-of-range
// indices, rather than panicking.
func TestPlaylistsViewRenderStaleSortOrderIndicesFallback(t *testing.T) {
	v := PlaylistsView{
		level: pltui.TrackLevel,
		tracks: []db.PlaylistTrackRow{
			{ID: 1, Title: "Alpha", Hash: "h1"},
			{ID: 2, Title: "Beta", Hash: "h2"},
		},
		// Length matches but index 5 is out of range for 2 tracks.
		sortedTrackOrder: []int{1, 5},
		width:            120,
		height:           20,
	}
	s := styles.NewStyles(true)
	out := v.render(s, nil, nil, nil, "", "") // must not panic
	if !strings.Contains(out, "Alpha") || !strings.Contains(out, "Beta") {
		t.Errorf("expected both titles in fallback render, got: %s", out)
	}
}
