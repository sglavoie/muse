package library

const (
	selectionMarkerFixed = 2  // '●' glyph + separator space
	trackNumFixed        = 5
	durationFixed        = 10
	sepCount             = 4 // spaces separating the five content columns

	artistRatio = 30
	titleRatio  = 35
	albumRatio  = 25
	totalRatio  = artistRatio + titleRatio + albumRatio
)

// Columns holds the computed character widths for each track list column.
// All widths are in Unicode code points (runes), not bytes.
type Columns struct {
	SelectionMarker int // fixed: selectionMarkerFixed chars ('●'/' ' + space)
	TrackNum        int // fixed: trackNumFixed chars
	Artist          int // proportional: ~30% of flex
	Title           int // proportional: ~35% of flex; absorbs rounding remainder
	Album           int // proportional: ~25% of flex
	Duration        int // fixed: durationFixed chars
}

// Compute calculates column widths for the given total terminal width.
// TrackNum and Duration receive fixed character allocations. The remaining
// width (after subtracting fixed columns and inter-column separators) is
// distributed proportionally in a 30:35:25 ratio among Artist, Title, and
// Album. Title absorbs any rounding remainder so that the sum of all column
// widths plus separators equals totalWidth exactly.
//
// When totalWidth is too small for the full layout, only TrackNum is set and
// the remaining fields are zero, signalling a degenerate (no-column) render.
func Compute(totalWidth int) Columns {
	minWidth := selectionMarkerFixed + trackNumFixed + durationFixed + sepCount
	if totalWidth <= minWidth {
		return Columns{SelectionMarker: selectionMarkerFixed, TrackNum: min(totalWidth, trackNumFixed)}
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
