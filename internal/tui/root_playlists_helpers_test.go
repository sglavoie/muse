package tui

import (
	"github.com/sglavoie/muse/internal/db"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
)

// drillIntoPlaylistTrackLevel puts m into Playlists view at TrackLevel with
// the provided tracks seeded directly, returning a ready-to-use model.
func drillIntoPlaylistTrackLevel(m Model, tracks []db.PlaylistTrackRow) Model {
	m.activeView = viewPlaylists
	m.playlists.Level = pltui.TrackLevel
	m.playlists.Tracks = tracks
	m.playlists.Cursor = 0
	return m
}
