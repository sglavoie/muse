package genres

const (
	selectionMarkerFixed = 2  // '●' glyph + separator space
	trackPosFixed        = 5  // "9999 " — supports up to 4-digit track numbers
	trackDurationFixed   = 10 // "mm:ss.xxx"
	trackSepCount        = 2  // spaces between the three columns
)

// TrackColumns holds computed character widths for the genre track-level view
// columns: position, title, and duration.
type TrackColumns struct {
	SelectionMarker int
	Position        int
	Title           int
	Duration        int
}

// ComputeTrack calculates column widths for the track-level view given the
// total terminal width. Position and Duration receive fixed allocations; Title
// takes the remainder. When the terminal is too narrow, Title is clamped to 1.
func ComputeTrack(totalWidth int) TrackColumns {
	minWidth := selectionMarkerFixed + trackPosFixed + trackDurationFixed + trackSepCount + 1
	if totalWidth <= minWidth {
		return TrackColumns{
			SelectionMarker: selectionMarkerFixed,
			Position:        trackPosFixed,
			Title:           1,
			Duration:        trackDurationFixed,
		}
	}
	title := totalWidth - selectionMarkerFixed - trackPosFixed - trackDurationFixed - trackSepCount
	return TrackColumns{
		SelectionMarker: selectionMarkerFixed,
		Position:        trackPosFixed,
		Title:           title,
		Duration:        trackDurationFixed,
	}
}
