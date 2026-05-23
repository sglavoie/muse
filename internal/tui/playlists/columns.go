package playlists

const (
	countFixed = 7 // "(smart)" — enough for the widest built-in count label
	nameSep    = 2 // two spaces between name and count columns

	trackMarkerFixed   = 2  // '●' glyph + separator space
	trackPosFixed      = 5  // "9999 " — supports up to 4-digit track numbers
	trackDurationFixed = 10 // "mm:ss.xxx" — duration column
	trackSepCount      = 4  // spaces between the five track columns

	trackTitleRatio  = 40
	trackArtistRatio = 35
	trackAlbumRatio  = 25
	trackTotalRatio  = trackTitleRatio + trackArtistRatio + trackAlbumRatio
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
	Marker   int // fixed: trackMarkerFixed chars ('●'/' ' + space)
	Position int // fixed: ~4 chars
	Title    int // proportional: 40% of flex
	Artist   int // proportional: 35% of flex
	Album    int // proportional: 25% of flex
	Duration int // fixed: ~10 chars
}

// ComputeTrack calculates column widths for the track-level view given the
// total terminal width. Position and Duration receive fixed allocations; the
// remaining width is split 56:44 between Title and Artist so the overall
// proportions land near 45% / 35% of the total. Title absorbs any rounding
// remainder. When the terminal is too narrow, Title is clamped to 1 and
// Artist to 0.
func ComputeTrack(totalWidth int) TrackColumns {
	minWidth := trackMarkerFixed + trackPosFixed + trackDurationFixed + trackSepCount + 1
	if totalWidth <= minWidth {
		return TrackColumns{
			Marker:   trackMarkerFixed,
			Position: trackPosFixed,
			Title:    1,
			Artist:   0,
			Album:    0,
			Duration: trackDurationFixed,
		}
	}

	flex := totalWidth - trackMarkerFixed - trackPosFixed - trackDurationFixed - trackSepCount
	artist := flex * trackArtistRatio / trackTotalRatio
	album := flex * trackAlbumRatio / trackTotalRatio
	title := flex - artist - album // absorbs integer division remainder

	return TrackColumns{
		Marker:   trackMarkerFixed,
		Position: trackPosFixed,
		Title:    title,
		Artist:   artist,
		Album:    album,
		Duration: trackDurationFixed,
	}
}
