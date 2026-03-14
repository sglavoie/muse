package playlists

const (
	countFixed = 6 // "(9999)" — enough for typical playlist track counts
	nameSep    = 2 // two spaces between name and count columns

	trackPosFixed      = 4  // "999 " — position column
	trackDurationFixed = 10 // "mm:ss.xxx" — duration column
	trackSepCount      = 3  // spaces between the four track columns

	trackTitleRatio  = 56
	trackArtistRatio = 44
	trackTotalRatio  = trackTitleRatio + trackArtistRatio
)

// Columns holds computed character widths for the playlist list columns.
type Columns struct {
	Name  int // flexible: fills remaining width after fixed columns
	Count int // fixed: countFixed chars for "(n)" display
}

// Compute calculates column widths for the given total terminal width.
// Count receives a fixed allocation; Name takes the remainder.
// When the terminal is too narrow for the full layout, Name is clamped to 1.
func Compute(totalWidth int) Columns {
	if totalWidth <= countFixed+nameSep+1 {
		return Columns{Name: 1, Count: countFixed}
	}
	return Columns{
		Name:  totalWidth - countFixed - nameSep,
		Count: countFixed,
	}
}

// TrackColumns holds computed character widths for the track-level view columns.
type TrackColumns struct {
	Position int // fixed: ~4 chars
	Title    int // proportional: ~45% of flex (56% of flex after fixed cols)
	Artist   int // proportional: ~35% of flex (44% of flex after fixed cols)
	Duration int // fixed: ~10 chars
}

// ComputeTrack calculates column widths for the track-level view given the
// total terminal width. Position and Duration receive fixed allocations; the
// remaining width is split 56:44 between Title and Artist so the overall
// proportions land near 45% / 35% of the total. Title absorbs any rounding
// remainder. When the terminal is too narrow, Title is clamped to 1 and
// Artist to 0.
func ComputeTrack(totalWidth int) TrackColumns {
	minWidth := trackPosFixed + trackDurationFixed + trackSepCount + 1
	if totalWidth <= minWidth {
		return TrackColumns{
			Position: trackPosFixed,
			Title:    1,
			Artist:   0,
			Duration: trackDurationFixed,
		}
	}

	flex := totalWidth - trackPosFixed - trackDurationFixed - trackSepCount
	artist := flex * trackArtistRatio / trackTotalRatio
	title := flex - artist // absorbs integer division remainder

	return TrackColumns{
		Position: trackPosFixed,
		Title:    title,
		Artist:   artist,
		Duration: trackDurationFixed,
	}
}
