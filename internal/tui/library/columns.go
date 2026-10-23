package library

import "github.com/sglavoie/muse/internal/tui/text"

const (
	selectionMarkerFixed = 2 // '●' glyph + separator space
	trackNumFixed        = 5
	durationFixed        = 10
	dateAddedFixed       = 10
	playedAtFixed        = 16
	sepCount             = 4 // 1-char gaps × 4 separators between five content columns
	sepCountWithDate     = 5 // 1-char gaps × 5 separators between six content columns

	artistRatio = 35
	titleRatio  = 40
	albumRatio  = 25
	totalRatio  = artistRatio + titleRatio + albumRatio
)

// Columns holds the computed character widths for each track list column.
// All widths are terminal display columns, not bytes.
type Columns struct {
	SelectionMarker int // fixed: selectionMarkerFixed chars ('●'/' ' + space)
	TrackNum        int // three to five cells as space allows
	Artist          int // grows without shrinking on resize
	Title           int // takes priority over secondary columns
	Album           int // hidden until a useful width fits
	Duration        int // clock plus any reserved secondary-column space
	DateAdded       int // fixed: dateAddedFixed chars; zero when column hidden
}

// IsTrackNumOnly reports whether the layout is in true degenerate mode where
// only the marker and track-number columns are available.
func (c Columns) IsTrackNumOnly() bool {
	return c.Artist == 0 && c.Title == 0 && c.Album == 0 && c.Duration == 0 && c.DateAdded == 0
}

// Compute uses the shared identity-first allocation at supported widths.
// Very small widths retain the track-number-only fallback.
func Compute(totalWidth int) Columns {
	if totalWidth >= 40 {
		c := text.TrackColumns(totalWidth, true, 0, 0)
		return Columns{SelectionMarker: 2, TrackNum: c.Position, Duration: c.Duration, Title: c.Title, Artist: c.Artist, Album: c.Album}
	}
	minWidth := selectionMarkerFixed + trackNumFixed + durationFixed + sepCount
	if totalWidth <= minWidth {
		return computeDegenerate(totalWidth)
	}

	flex := totalWidth - selectionMarkerFixed - trackNumFixed - durationFixed - sepCount
	artist := flex * artistRatio / totalRatio
	album := flex * albumRatio / totalRatio
	title := flex - artist - album // absorbs integer division remainder

	return Columns{
		SelectionMarker: selectionMarkerFixed,
		TrackNum:        trackNumFixed,
		Artist:          artist,
		Title:           title,
		Album:           album,
		Duration:        durationFixed,
	}
}

// ComputeWithDateAdded calculates widths for the library-style track table
// when a fixed Date Added column is displayed.
func ComputeWithDateAdded(totalWidth int) Columns {
	return computeWithDateCol(totalWidth, dateAddedFixed)
}

// ComputeWithPlayedAt calculates widths for the history-style track table
// where the date column is wider to include HH:MM.
func ComputeWithPlayedAt(totalWidth int) Columns {
	return computeWithDateCol(totalWidth, playedAtFixed)
}

func computeWithDateCol(totalWidth, dateColWidth int) Columns {
	if totalWidth >= 40 {
		c := text.TrackColumns(totalWidth, true, 10, dateColWidth)
		return Columns{SelectionMarker: 2, TrackNum: c.Position, Duration: c.Duration, Title: c.Title, Artist: c.Artist, Album: c.Album, DateAdded: c.Detail}
	}
	minWidth := selectionMarkerFixed + trackNumFixed + durationFixed + dateColWidth + sepCountWithDate
	if totalWidth <= minWidth {
		return computeDegenerate(totalWidth)
	}

	flex := totalWidth - selectionMarkerFixed - trackNumFixed - durationFixed - dateColWidth - sepCountWithDate
	artist := flex * artistRatio / totalRatio
	album := flex * albumRatio / totalRatio
	title := flex - artist - album

	return Columns{
		SelectionMarker: selectionMarkerFixed,
		TrackNum:        trackNumFixed,
		Artist:          artist,
		Title:           title,
		Album:           album,
		Duration:        durationFixed,
		DateAdded:       dateColWidth,
	}
}

func computeDegenerate(totalWidth int) Columns {
	selection := min(max(totalWidth, 0), selectionMarkerFixed)
	return Columns{
		SelectionMarker: selection,
		TrackNum:        max(0, totalWidth-selection),
	}
}

func (c Columns) separator() string { return " " }
