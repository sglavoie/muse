package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/db"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
)

// TestSelectionFooterHintsPlaylistsAndTags guards that a bulk selection in
// Playlists and Tags switches the footer to its selection-mode hint set, the
// same way every other view with bulk selection does.
func TestSelectionFooterHintsPlaylistsAndTags(t *testing.T) {
	rows := []db.PlaylistTrackRow{
		{ID: 1, Hash: "h1", Title: "Track A1", Artist: "Alice"},
		{ID: 2, Hash: "h2", Title: "Track A2", Artist: "Alice"},
	}
	cases := []struct {
		name  string
		setup func(m Model) Model
	}{
		{
			name: "playlists",
			setup: func(m Model) Model {
				m.ActiveView = viewPlaylists
				m.playlists.Level = pltui.TrackLevel
				m.playlists.Tracks = rows
				m.playlists.SelectedTrackIDs = map[int64]struct{}{1: {}}
				return m
			},
		},
		{
			name: "tags",
			setup: func(m Model) Model {
				m.ActiveView = viewTags
				m.tagsView.Level = tagtui.TrackLevel
				m.tagsView.Tracks = rows
				m.tagsView.SelectedHashes = map[string]struct{}{"h1": {}}
				return m
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := tc.setup(New(context.Background()))
			m.showFooterHints = true
			m.termWidth = 200
			if got := m.activeViewSelectionCount(); got != 1 {
				t.Errorf("activeViewSelectionCount = %d, want 1", got)
			}
			hint := ansi.Strip(m.renderFooterHint())
			if !strings.Contains(hint, "X clear") {
				t.Errorf("selection footer missing %q: %q", "X clear", hint)
			}
		})
	}
}
