package playlists

import "github.com/sglavoie/muse/internal/tui/text"

const (
	countFixed = 14 // labeled track count, including comma separators
	nameSep    = 2  // two spaces between name and count columns

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
	Count int // fixed: countFixed chars for labeled track counts
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
	Position int // three to five cells as space allows
	Title    int // takes priority over secondary columns
	Artist   int // grows without shrinking on resize
	Album    int // hidden until a useful width fits
	Duration int // clock plus any reserved secondary-column space
}

// ComputeTrack uses the shared identity-first allocation at supported widths,
// retaining the existing fallback for terminals smaller than 40 columns.
func ComputeTrack(totalWidth int) TrackColumns {
	if totalWidth >= 40 {
		c := text.TrackColumns(totalWidth, true, 0, 0)
		return TrackColumns{Marker: 2, Position: c.Position, Duration: c.Duration, Title: c.Title, Artist: c.Artist, Album: c.Album}
	}
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
