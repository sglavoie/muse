package library

import (
	"testing"
)

// TestComputeSumsToWidth verifies that column widths plus separators equal
// the requested terminal width for all reasonable inputs.
func TestComputeSumsToWidth(t *testing.T) {
	// minWidth = selectionMarkerFixed(2) + trackNumFixed(5) + durationFixed(10) + sepCount(4) = 21
	// so widths > 21 are non-degenerate.
	for _, w := range []int{26, 40, 80, 120, 160, 200, 300} {
		cols := Compute(w)
		got := cols.SelectionMarker + cols.TrackNum + cols.Artist + cols.Title + cols.Album + cols.Duration + 4
		if cols.Album == 0 && cols.Artist > 0 {
			got -= 1
		}
		if got != w {
			t.Errorf("Compute(%d): column widths + separators = %d, want %d", w, got, w)
		}
	}
}

// TestComputeDegenerateWidth verifies that very narrow terminals do not panic
// and return a TrackNum-only layout.
func TestComputeDegenerateWidth(t *testing.T) {
	// minWidth = 21, so widths <= 21 are degenerate.
	for _, w := range []int{0, 1, 19, 20, 21} {
		cols := Compute(w)
		if cols.Artist != 0 || cols.Title != 0 || cols.Album != 0 || cols.Duration != 0 {
			t.Errorf("Compute(%d): expected degenerate layout (all flex=0), got %+v", w, cols)
		}
	}
}
