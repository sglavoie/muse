package favorites

import "github.com/sglavoie/muse/internal/tui/text"

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
	Position  int // three to five cells as space allows
	Title     int // takes priority over secondary columns
	Artist    int // grows without shrinking on resize
	Album     int // hidden until a useful width fits
	Duration  int // clock plus any reserved secondary-column space
	PlayCount int // fixed: 7 chars
}

// ComputeTrack uses the shared identity-first allocation at supported widths,
// retaining the existing fallback for terminals smaller than 40 columns.
func ComputeTrack(totalWidth int) TrackColumns {
	if totalWidth >= 40 {
		c := text.TrackColumns(totalWidth, true, 0, playCountFixed)
		return TrackColumns{Marker: 2, Position: c.Position, Duration: c.Duration, Title: c.Title, Artist: c.Artist, Album: c.Album, PlayCount: c.Detail}
	}
	cols := TrackColumns{
		Marker:    trackMarkerFixed,
		Position:  trackPosFixed,
		Duration:  trackDurationFixed,
		PlayCount: playCountFixed,
	}
	available := totalWidth - trackMarkerFixed - trackSepCount
	if available <= 0 {
		cols.Position = 0
		cols.Duration = 0
		cols.PlayCount = 0
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
