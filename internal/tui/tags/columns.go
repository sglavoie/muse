package tags

import "github.com/sglavoie/muse/internal/tui/text"

const (
	trackDurationFixed   = 10 // "mm:ss.xxx"
	trackSepCount        = 3  // spaces between four columns
	selectionMarkerFixed = 2  // '●' + space
)

// TrackColumns holds computed character widths for the tag track-level view
// columns: selection marker, title, artist, album, and duration.
type TrackColumns struct {
	SelectionMarker int
	Title           int
	Artist          int
	Album           int
	Duration        int
}

// ComputeTrack uses the shared identity-first allocation at supported widths,
// retaining the existing fallback for terminals smaller than 40 columns.
func ComputeTrack(totalWidth int) TrackColumns {
	if totalWidth >= 40 {
		c := text.TrackColumns(totalWidth, false, 0, 0)
		return TrackColumns{SelectionMarker: 2, Duration: c.Duration, Title: c.Title, Artist: c.Artist, Album: c.Album}
	}
	fixed := selectionMarkerFixed + trackDurationFixed + trackSepCount
	remaining := totalWidth - fixed
	cols := TrackColumns{
		SelectionMarker: selectionMarkerFixed,
		Title:           0,
		Artist:          0,
		Album:           0,
		Duration:        trackDurationFixed,
	}
	if remaining <= 0 {
		cols.Duration = max(totalWidth-selectionMarkerFixed-trackSepCount, 0)
		return cols
	}

	cols.Artist = remaining * 35 / 100
	cols.Album = remaining * 25 / 100
	cols.Title = remaining - cols.Artist - cols.Album
	return cols
}
