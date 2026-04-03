package library

import (
	"math"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// TestComputeSumsToWidth verifies that column widths plus separators equal
// the requested terminal width for all reasonable inputs.
func TestComputeSumsToWidth(t *testing.T) {
	// minWidth = selectionMarkerFixed(2) + trackNumFixed(5) + durationFixed(10) + sepCount(4) = 21
	// so widths > 21 are non-degenerate.
	for _, w := range []int{22, 40, 80, 120, 160, 200, 300} {
		cols := Compute(w)
		got := cols.SelectionMarker + cols.TrackNum + cols.Artist + cols.Title + cols.Album + cols.Duration + 4
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

// columnWidthsFromRender renders the library view at the given width, strips
// ANSI sequences from the first track row using charmbracelet/x/ansi.Strip,
// and returns the visible rune widths of the Artist, Title, and Album columns.
func columnWidthsFromRender(t *testing.T, width int) (artistW, titleW, albumW int) {
	t.Helper()
	idx := buildIndex(sampleTracks)
	v := View{Index: idx, Width: width, Height: 0}
	rendered := v.Render()

	lines := strings.Split(rendered, "\n")
	// lines[0]=header, lines[1]=first track row
	if len(lines) < 2 {
		t.Fatalf("columnWidthsFromRender: expected ≥2 lines, got %d", len(lines))
	}

	// Strip all ANSI escape sequences to obtain only visible characters.
	trackRow := ansi.Strip(lines[1])
	runes := []rune(trackRow)
	if len(runes) != width {
		t.Fatalf("columnWidthsFromRender: stripped row has %d runes, want %d", len(runes), width)
	}

	cols := Compute(width)
	// Row layout: selectionMarker(2) + trackNum(5) + sep(1) + Artist + sep(1) + Title + sep(1) + Album + sep(1) + Duration(10)
	artistStart := selectionMarkerFixed + trackNumFixed + 1
	artistEnd := artistStart + cols.Artist
	titleStart := artistEnd + 1
	titleEnd := titleStart + cols.Title
	albumStart := titleEnd + 1
	albumEnd := albumStart + cols.Album

	if albumEnd > len(runes) {
		t.Fatalf("columnWidthsFromRender: album column extends beyond row: albumEnd=%d len=%d", albumEnd, len(runes))
	}

	return len(runes[artistStart:artistEnd]),
		len(runes[titleStart:titleEnd]),
		len(runes[albumStart:albumEnd])
}

// assertRatioWithinTolerance verifies that colWidth/flexWidth is within ±2%
// of wantPct (expressed as a percentage, e.g. 33.33 for 33.33%).
func assertRatioWithinTolerance(t *testing.T, colName string, colWidth, flexWidth int, wantPct float64) {
	t.Helper()
	gotPct := float64(colWidth) / float64(flexWidth) * 100
	if math.Abs(gotPct-wantPct) > 2.0 {
		t.Errorf("column %s: ratio %.2f%% (width=%d/flex=%d) exceeds ±2%% tolerance from expected %.2f%%",
			colName, gotPct, colWidth, flexWidth, wantPct)
	}
}

// TestArtistColumnRatioAtWidth120 asserts that the Artist column visible width
// is within ±2% of the expected 30/90 ≈ 33.33% share of flex space at width=120.
func TestArtistColumnRatioAtWidth120(t *testing.T) {
	const width = 120
	flex := width - selectionMarkerFixed - trackNumFixed - durationFixed - sepCount
	artistW, _, _ := columnWidthsFromRender(t, width)
	wantPct := float64(artistRatio) / float64(totalRatio) * 100
	assertRatioWithinTolerance(t, "Artist", artistW, flex, wantPct)
}

// TestTitleColumnRatioAtWidth120 asserts that the Title column visible width
// is within ±2% of the expected 35/90 ≈ 38.89% share of flex space at width=120.
func TestTitleColumnRatioAtWidth120(t *testing.T) {
	const width = 120
	flex := width - selectionMarkerFixed - trackNumFixed - durationFixed - sepCount
	_, titleW, _ := columnWidthsFromRender(t, width)
	wantPct := float64(titleRatio) / float64(totalRatio) * 100
	assertRatioWithinTolerance(t, "Title", titleW, flex, wantPct)
}

// TestAlbumColumnRatioAtWidth120 asserts that the Album column visible width
// is within ±2% of the expected 25/90 ≈ 27.78% share of flex space at width=120.
func TestAlbumColumnRatioAtWidth120(t *testing.T) {
	const width = 120
	flex := width - selectionMarkerFixed - trackNumFixed - durationFixed - sepCount
	_, _, albumW := columnWidthsFromRender(t, width)
	wantPct := float64(albumRatio) / float64(totalRatio) * 100
	assertRatioWithinTolerance(t, "Album", albumW, flex, wantPct)
}
