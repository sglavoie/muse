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
// total terminal width. Position and Duration receive fixed allocations when
// space allows; Title takes the remainder. On narrow terminals, fixed columns
// shrink before the title disappears so the allocation does not exceed
// totalWidth.
func ComputeTrack(totalWidth int) TrackColumns {
	cols := TrackColumns{
		SelectionMarker: selectionMarkerFixed,
		Position:        trackPosFixed,
		Duration:        trackDurationFixed,
	}
	available := totalWidth - selectionMarkerFixed - trackSepCount
	if available <= 0 {
		cols.SelectionMarker = max(totalWidth-trackSepCount, 0)
		cols.Position = 0
		cols.Duration = 0
		return cols
	}

	required := cols.Position + cols.Duration + 1
	if available < required {
		deficit := required - available

		shrink := min(deficit, cols.Duration)
		cols.Duration -= shrink
		deficit -= shrink

		shrink = min(deficit, cols.Position)
		cols.Position -= shrink
		deficit -= shrink

		cols.Title = 1
		if deficit > 0 {
			cols.Title = max(cols.Title-deficit, 0)
		}
		return cols
	}

	cols.Title = available - cols.Position - cols.Duration
	return cols
}
