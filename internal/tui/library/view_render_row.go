package library

import "github.com/sglavoie/muse/internal/tui/text"

// renderRow formats a single row with each field padded or truncated to its
// column width. When cols.Artist is zero the layout degenerates to just the
// track number field.
func renderRow(num, artist, title, album, duration, dateAdded string, cols Columns) string {
	if cols.Artist == 0 {
		return text.Pad(num, cols.TrackNum)
	}
	row := text.Pad(num, cols.TrackNum) + "  " +
		text.Pad(artist, cols.Artist) + "  " +
		text.Pad(title, cols.Title) + "  " +
		text.Pad(album, cols.Album) + "  " +
		text.PadLeft(duration, cols.Duration)
	if cols.DateAdded > 0 {
		row += "  " + text.Pad(dateAdded, cols.DateAdded)
	}
	return row
}
