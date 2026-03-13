package library

import (
	"strings"
	"testing"

	musiclib "github.com/sglavoie/muse/internal/library"
)

func buildIndex(tracks []musiclib.Track) musiclib.LibraryIndex {
	var idx musiclib.LibraryIndex
	for _, t := range tracks {
		idx.Insert(t)
	}
	return idx
}

var sampleTracks = []musiclib.Track{
	{Artist: "Alice", Title: "Song One", Album: "Debut", Duration: 185},
	{Artist: "Bob", Title: "Track Two", Album: "Second", Duration: 242},
	{Artist: "Unknown Artist", Title: "Mystery", Album: "", Duration: 0},
}

// TestComputeSumsToWidth verifies that column widths plus separators equal
// the requested terminal width for all reasonable inputs.
func TestComputeSumsToWidth(t *testing.T) {
	for _, w := range []int{20, 40, 80, 120, 160, 200, 300} {
		cols := Compute(w)
		got := cols.TrackNum + cols.Artist + cols.Title + cols.Album + cols.Duration + 4
		if got != w {
			t.Errorf("Compute(%d): column widths + separators = %d, want %d", w, got, w)
		}
	}
}

// TestComputeDegenerateWidth verifies that very narrow terminals do not panic
// and return a TrackNum-only layout.
func TestComputeDegenerateWidth(t *testing.T) {
	for _, w := range []int{0, 1, 19} {
		cols := Compute(w)
		if cols.Artist != 0 || cols.Title != 0 || cols.Album != 0 || cols.Duration != 0 {
			t.Errorf("Compute(%d): expected degenerate layout (all flex=0), got %+v", w, cols)
		}
	}
}

// TestRenderColumnsDoNotOverflow verifies that each row in the rendered output
// has exactly the expected byte length at a given terminal width.
func TestRenderColumnsDoNotOverflow(t *testing.T) {
	idx := buildIndex(sampleTracks)

	for _, width := range []int{40, 80, 120, 160} {
		v := View{Index: idx, Width: width, Height: 0}
		rendered := v.Render()
		lines := strings.Split(rendered, "\n")

		// Skip the heading and blank line; the header row is lines[2],
		// track rows follow.
		if len(lines) < 3 {
			t.Fatalf("width=%d: expected at least 3 lines, got %d", width, len(lines))
		}

		for _, line := range lines[2:] {
			got := len([]rune(line))
			if got != width {
				t.Errorf("width=%d: row has %d runes, want %d: %q", width, got, width, line)
			}
		}
	}
}

// TestRenderEmptyIndex returns the empty placeholder.
func TestRenderEmptyIndex(t *testing.T) {
	v := View{Width: 80, Height: 24}
	got := v.Render()
	if got != "Library\n\n(no tracks)" {
		t.Errorf("unexpected empty render: %q", got)
	}
}

// TestRenderZeroWidthFallback verifies that a zero Width renders a simple
// "artist – title" format without panic.
func TestRenderZeroWidthFallback(t *testing.T) {
	idx := buildIndex(sampleTracks)
	v := View{Index: idx, Width: 0, Height: 0}
	got := v.Render()
	if !strings.Contains(got, "Library") {
		t.Errorf("expected heading in zero-width render, got: %q", got)
	}
	// In fallback mode each row is "artist – title", not a padded columnar row.
	// The column header row starts with the literal "#" track-number header.
	for line := range strings.SplitSeq(got, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			t.Errorf("zero-width render should not contain a column header row, got: %q", line)
		}
	}
}

// TestRenderSortOrderUnknownArtistLast verifies that "Unknown Artist" entries
// sort below named artists in the rendered output.
func TestRenderSortOrderUnknownArtistLast(t *testing.T) {
	idx := buildIndex(sampleTracks)
	v := View{Index: idx, Width: 80, Height: 0}
	got := v.Render()
	alicePos := strings.Index(got, "Alice")
	unknownPos := strings.Index(got, "Unknown Artist")
	if alicePos == -1 || unknownPos == -1 {
		t.Fatalf("could not find expected artist names in render:\n%s", got)
	}
	if alicePos > unknownPos {
		t.Errorf("expected named artists before Unknown Artist; alice at %d, unknown at %d", alicePos, unknownPos)
	}
}

// TestPadTruncation verifies that pad correctly truncates long strings and
// appends an ellipsis.
func TestPadTruncation(t *testing.T) {
	got := pad("Hello World", 7)
	if len([]rune(got)) != 7 {
		t.Errorf("pad: expected 7 runes, got %d: %q", len([]rune(got)), got)
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("pad: expected ellipsis suffix, got %q", got)
	}
}

// TestPadPadding verifies that pad right-pads short strings with spaces.
func TestPadPadding(t *testing.T) {
	got := pad("Hi", 5)
	if got != "Hi   " {
		t.Errorf("pad: expected %q, got %q", "Hi   ", got)
	}
}
