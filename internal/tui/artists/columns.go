package artists

const (
	trackPosFixed      = 4  // "999 "
	trackDurationFixed = 10 // "mm:ss.xxx"
	trackSepCount      = 2  // spaces between the three columns
)

// TrackColumns holds computed character widths for the artist track-level view
// columns: position, title, and duration.
type TrackColumns struct {
	Position int
	Title    int
	Duration int
}

// ComputeTrack calculates column widths for the track-level view given the
// total terminal width. Position and Duration receive fixed allocations; Title
// takes the remainder. When the terminal is too narrow, Title is clamped to 1.
func ComputeTrack(totalWidth int) TrackColumns {
	minWidth := trackPosFixed + trackDurationFixed + trackSepCount + 1
	if totalWidth <= minWidth {
		return TrackColumns{
			Position: trackPosFixed,
			Title:    1,
			Duration: trackDurationFixed,
		}
	}
	title := totalWidth - trackPosFixed - trackDurationFixed - trackSepCount
	return TrackColumns{
		Position: trackPosFixed,
		Title:    title,
		Duration: trackDurationFixed,
	}
}
