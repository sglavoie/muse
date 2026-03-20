package favorites

const (
	trackMarkerFixed   = 2  // '●' glyph + separator space
	trackPosFixed      = 5  // "9999 " — supports up to 4-digit track numbers
	trackDurationFixed = 10 // "mm:ss.xxx" — duration column
	trackSepCount      = 4  // spaces between the five track columns

	trackTitleRatio  = 40
	trackArtistRatio = 35
	trackAlbumRatio  = 25
	trackTotalRatio  = trackTitleRatio + trackArtistRatio + trackAlbumRatio
)

// TrackColumns holds computed character widths for the favorite track-level
// view columns.
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
// remaining width is split among Title, Artist, and Album. Title absorbs any
// rounding remainder.
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
