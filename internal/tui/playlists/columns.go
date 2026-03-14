package playlists

const (
	countFixed = 6 // "(9999)" — enough for typical playlist track counts
	nameSep    = 2 // two spaces between name and count columns
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
