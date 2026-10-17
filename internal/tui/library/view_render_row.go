package library

import (
	"github.com/sglavoie/muse/internal/tui/text"
	"strings"
)

// renderRow formats a single row with each field padded or truncated to its
// column width. When columns are in track-number-only mode, the layout
// degenerates to just the
// track number field.
func renderRow(num, artist, title, album, duration, dateAdded string, cols Columns) string {
	if cols.IsTrackNumOnly() {
		return text.Pad(num, cols.TrackNum)
	}
	row := joinTrackCells(cols, cols.separator(), text.Pad(num, cols.TrackNum),
		text.Pad(artist, cols.Artist), text.Pad(title, cols.Title),
		text.Pad(album, cols.Album), text.PadLeft(duration, cols.Duration), text.Pad(dateAdded, cols.DateAdded))
	return row
}

func joinTrackCells(cols Columns, sep, num, artist, title, album, duration, date string) string {
	cells := []string{num, title, artist}
	if cols.Album > 0 || cols.TrackNum != 3 {
		cells = append(cells, album)
	}
	cells = append(cells, duration)
	if cols.DateAdded > 0 {
		cells = append(cells, date)
	}
	return strings.Join(cells, sep)
}
