package favorites

const (
	trackMarkerFixed   = 2  // '●' glyph + separator space
	trackPosFixed      = 5  // "9999 " — supports up to 4-digit track numbers
	trackDurationFixed = 10 // "mm:ss.xxx" — duration column
	playCountFixed     = 7  // "Plays" header + right-aligned values up to 99999
	trackSepCount      = 5  // spaces between the six track columns

	trackTitleRatio  = 40
	trackArtistRatio = 35
	trackAlbumRatio  = 25
	trackTotalRatio  = trackTitleRatio + trackArtistRatio + trackAlbumRatio
)

// TrackColumns holds computed character widths for the favorite track-level
// view columns.
type TrackColumns struct {
	Marker    int // fixed: trackMarkerFixed chars ('●'/' ' + space)
	Position  int // fixed: ~4 chars
	Title     int // proportional: 40% of flex
	Artist    int // proportional: 35% of flex
	Album     int // proportional: 25% of flex
	Duration  int // fixed: ~10 chars
	PlayCount int // fixed: 7 chars
}

// ComputeTrack calculates column widths for the track-level view given the
// total terminal width. Position and Duration receive fixed allocations; the
// remaining width is split among Title, Artist, and Album. Title absorbs any
// rounding remainder. On very narrow terminals, fixed columns are reduced
// (PlayCount, then Duration, then Position) to avoid over-allocating width.
func ComputeTrack(totalWidth int) TrackColumns {
	cols := TrackColumns{
		Marker:    trackMarkerFixed,
		Position:  trackPosFixed,
		Duration:  trackDurationFixed,
		PlayCount: playCountFixed,
	}
	available := totalWidth - trackMarkerFixed - trackSepCount
	if available <= 0 {
		return cols
	}

	// Keep a one-character title column when possible by shrinking less critical
	// fixed columns first.
	required := cols.Position + cols.Duration + cols.PlayCount + 1
	if available < required {
		deficit := required - available

		shrink := min(deficit, cols.PlayCount)
		cols.PlayCount -= shrink
		deficit -= shrink

		shrink = min(deficit, cols.Duration)
		cols.Duration -= shrink
		deficit -= shrink

		minPosition := 1
		if available <= 1 {
			minPosition = 0
		}
		maxPosShrink := max(cols.Position-minPosition, 0)
		shrink = min(deficit, maxPosShrink)
		cols.Position -= shrink
		deficit -= shrink

		cols.Title = 1
		if deficit > 0 {
			cols.Title = max(cols.Title-deficit, 0)
		}
		return cols
	}

	flex := available - cols.Position - cols.Duration - cols.PlayCount
	artist := flex * trackArtistRatio / trackTotalRatio
	album := flex * trackAlbumRatio / trackTotalRatio
	title := flex - artist - album // absorbs integer division remainder

	cols.Title = title
	cols.Artist = artist
	cols.Album = album
	return cols
}
