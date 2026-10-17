package tags

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

// ComputeTrack follows the shared Title/Artist/Album proportions (40/35/25).
// Compact tables omit Album and give Title 60% of the remaining space.
func ComputeTrack(totalWidth int) TrackColumns {
	if totalWidth >= 40 && totalWidth < 80 {
		flex := totalWidth - 2 - 7 - 2
		artist := flex * 2 / 5
		return TrackColumns{SelectionMarker: 2, Duration: 7, Title: flex - artist, Artist: artist}
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
