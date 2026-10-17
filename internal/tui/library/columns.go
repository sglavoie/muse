package library

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
	TrackNum        int // fixed: trackNumFixed chars
	Artist          int // proportional: ~35% of flex
	Title           int // proportional: ~40% of flex; absorbs rounding remainder
	Album           int // proportional: ~25% of flex
	Duration        int // fixed: durationFixed chars
	DateAdded       int // fixed: dateAddedFixed chars; zero when column hidden
}

// IsTrackNumOnly reports whether the layout is in true degenerate mode where
// only the marker and track-number columns are available.
func (c Columns) IsTrackNumOnly() bool {
	return c.Artist == 0 && c.Title == 0 && c.Album == 0 && c.Duration == 0 && c.DateAdded == 0
}

// Compute calculates column widths for the given total terminal width.
// TrackNum and Duration receive fixed character allocations. The remaining
// width (after subtracting fixed columns and inter-column separators) is
// distributed proportionally in a 35:40:25 ratio among Artist, Title, and
// Album. Title absorbs any rounding remainder so that the sum of all column
// widths plus separators equals totalWidth exactly.
//
// When totalWidth is too small for the full layout, only TrackNum is set and
// the remaining fields are zero, signalling a degenerate (no-column) render.
func Compute(totalWidth int) Columns {
	if totalWidth >= 40 && totalWidth < 80 {
		return compactColumns(totalWidth, 0)
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
	if totalWidth >= 40 && totalWidth < 80+dateColWidth {
		return compactColumns(totalWidth, 10)
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

// Compact tables trade album and timestamp detail for readable song identities.
func compactColumns(width, dateWidth int) Columns {
	separators := 3
	if dateWidth > 0 {
		separators++
	}
	flex := max(width-2-3-7-dateWidth-separators, 0)
	artist := flex * 2 / 5
	return Columns{SelectionMarker: 2, TrackNum: 3, Duration: 7,
		Artist: artist, Title: flex - artist, DateAdded: dateWidth}
}

func (c Columns) separator() string { return " " }
