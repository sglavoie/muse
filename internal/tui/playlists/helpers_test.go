package playlists

import (
	"charm.land/lipgloss/v2"
	"github.com/sglavoie/muse/internal/db"
)

// styledText returns text wrapped with the given lipgloss style, producing
// the same string that production code generates via Style.Render.
func styledText(style lipgloss.Style, text string) string {
	return style.Render(text)
}

// fakePlaylistRows returns a slice of PlaylistRow values as if returned by
// db.ReadPlaylists, pre-sorted alphabetically.
func fakePlaylistRows(names ...string) []db.PlaylistRow {
	rows := make([]db.PlaylistRow, len(names))
	for i, name := range names {
		rows[i] = db.PlaylistRow{ID: int64(i + 1), Name: name}
	}
	return rows
}
