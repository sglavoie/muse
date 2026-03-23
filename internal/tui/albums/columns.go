package albums

const (
	selectionMarkerFixed = 2  // '●' glyph + separator space
	trackPosFixed        = 5  // "9999 " — supports up to 4-digit track numbers
	trackDurationFixed   = 10 // "mm:ss.xxx" — duration column
	trackSepCount        = 3  // spaces between the four columns

	trackTitleRatio  = 55
	trackArtistRatio = 45
	trackTotalRatio  = trackTitleRatio + trackArtistRatio
)

// TrackColumns holds computed character widths for the album track-level view
// columns: position, title, artist, and duration.
type TrackColumns struct {
	SelectionMarker int
	Position        int
	Title           int
	Artist          int
	Duration        int
}

// ComputeTrack calculates column widths for the track-level view given the
// total terminal width. Position and Duration receive fixed allocations; the
// remaining width is split between Title and Artist. Title absorbs any
// rounding remainder.
func ComputeTrack(totalWidth int) TrackColumns {
	minWidth := selectionMarkerFixed + trackPosFixed + trackDurationFixed + trackSepCount + 1
	if totalWidth <= minWidth {
		return TrackColumns{
			SelectionMarker: selectionMarkerFixed,
			Position:        trackPosFixed,
			Title:           1,
			Artist:          0,
			Duration:        trackDurationFixed,
		}
	}
	flex := totalWidth - selectionMarkerFixed - trackPosFixed - trackDurationFixed - trackSepCount
	artist := flex * trackArtistRatio / trackTotalRatio
	title := flex - artist // absorbs integer division remainder
	return TrackColumns{
		SelectionMarker: selectionMarkerFixed,
		Position:        trackPosFixed,
		Title:           title,
		Artist:          artist,
		Duration:        trackDurationFixed,
	}
}
