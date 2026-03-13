package library

import (
	"regexp"
	"strings"
	"testing"

	musiclib "github.com/sglavoie/muse/internal/library"
)

// ansiEscape strips ANSI escape sequences from s so that display-width checks
// are not confused by non-printing highlight codes.
var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(s string) string {
	return ansiEscape.ReplaceAllString(s, "")
}

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
			got := len([]rune(stripANSI(line)))
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

// TestCursorDownAdvances verifies that CursorDown (j / arrow-down) increments
// the cursor index by one.
func TestCursorDownAdvances(t *testing.T) {
	idx := buildIndex(sampleTracks)
	v := View{Index: idx, Width: 80, Height: 0, Cursor: 0}
	v = v.CursorDown()
	if v.Cursor != 1 {
		t.Errorf("CursorDown: expected cursor 1, got %d", v.Cursor)
	}
}

// TestCursorUpAdvances verifies that CursorUp (k / arrow-up) decrements the
// cursor index by one.
func TestCursorUpAdvances(t *testing.T) {
	idx := buildIndex(sampleTracks)
	v := View{Index: idx, Width: 80, Height: 0, Cursor: 2}
	v = v.CursorUp()
	if v.Cursor != 1 {
		t.Errorf("CursorUp: expected cursor 1, got %d", v.Cursor)
	}
}

// TestCursorClampAtBottom verifies that CursorDown stops at the last track
// without wrapping.
func TestCursorClampAtBottom(t *testing.T) {
	idx := buildIndex(sampleTracks)
	last := len(sampleTracks) - 1
	v := View{Index: idx, Width: 80, Height: 0, Cursor: last}
	v = v.CursorDown()
	if v.Cursor != last {
		t.Errorf("CursorDown at last: expected cursor %d (clamped), got %d", last, v.Cursor)
	}
}

// TestCursorClampAtTop verifies that CursorUp stops at the first track without
// wrapping.
func TestCursorClampAtTop(t *testing.T) {
	idx := buildIndex(sampleTracks)
	v := View{Index: idx, Width: 80, Height: 0, Cursor: 0}
	v = v.CursorUp()
	if v.Cursor != 0 {
		t.Errorf("CursorUp at first: expected cursor 0 (clamped), got %d", v.Cursor)
	}
}

// TestFilterInactiveOnInit verifies that the filter bar is not active when a
// View is first constructed.
func TestFilterInactiveOnInit(t *testing.T) {
	v := View{}
	if v.FilterActive {
		t.Error("FilterActive should be false on a zero-value View")
	}
	if v.FilterText != "" {
		t.Errorf("FilterText should be empty on a zero-value View, got %q", v.FilterText)
	}
}

// TestSlashActivatesFilter verifies that calling ActivateFilter sets
// FilterActive to true (models the '/' keypress).
func TestSlashActivatesFilter(t *testing.T) {
	v := View{}
	v = v.ActivateFilter()
	if !v.FilterActive {
		t.Error("FilterActive should be true after ActivateFilter")
	}
}

// TestAppendFilterCharBuildsQuery verifies that key presses after activation
// accumulate into FilterText.
func TestAppendFilterCharBuildsQuery(t *testing.T) {
	v := View{}
	v = v.ActivateFilter()
	v = v.AppendFilterChar('r')
	v = v.AppendFilterChar('o')
	v = v.AppendFilterChar('c')
	if v.FilterText != "roc" {
		t.Errorf("FilterText: expected %q, got %q", "roc", v.FilterText)
	}
}

// TestAppendFilterCharIgnoredWhenInactive verifies that AppendFilterChar is a
// no-op when the filter bar has not been activated.
func TestAppendFilterCharIgnoredWhenInactive(t *testing.T) {
	v := View{}
	v = v.AppendFilterChar('x')
	if v.FilterText != "" {
		t.Errorf("FilterText should remain empty when filter is inactive, got %q", v.FilterText)
	}
}

// highlightFake is a test-local FuzzySearcher that returns predetermined results.
type highlightFake struct {
	results []musiclib.Match
}

func (h highlightFake) Search(_ string, _ []musiclib.Track) []musiclib.Match {
	return h.results
}

// TestFuzzyFilterUpdatesDisplayedTracks verifies that displayed tracks narrow
// to those matching the filter query after each character is typed.
func TestFuzzyFilterUpdatesDisplayedTracks(t *testing.T) {
	idx := buildIndex(sampleTracks)
	v := View{
		Index:  idx,
		Width:  80,
		Height: 24,
		Fuzzy:  musiclib.FakeFuzzySearcher{},
	}
	v = v.ActivateFilter()
	for _, ch := range "Alice" {
		v = v.AppendFilterChar(ch)
	}
	got := v.Render()
	if !strings.Contains(got, "Alice") {
		t.Errorf("filtered render should contain 'Alice', got:\n%s", got)
	}
	if strings.Contains(stripANSI(got), "Bob") {
		t.Errorf("filtered render should not contain 'Bob' when filtering for 'Alice', got:\n%s", got)
	}
}

// TestFuzzyFilterUpdatesOnEachKeystroke verifies that results narrow
// incrementally with each additional character typed.
func TestFuzzyFilterUpdatesOnEachKeystroke(t *testing.T) {
	idx := buildIndex(sampleTracks)
	v := View{
		Index:  idx,
		Width:  80,
		Height: 24,
		Fuzzy:  musiclib.FakeFuzzySearcher{},
	}
	v = v.ActivateFilter()
	v = v.AppendFilterChar('B')
	got := v.Render()
	if !strings.Contains(got, "Bob") {
		t.Errorf("filtering for 'B' should show 'Bob', got:\n%s", got)
	}
	if strings.Contains(stripANSI(got), "Alice") {
		t.Errorf("filtering for 'B' should not show 'Alice', got:\n%s", got)
	}

	v = v.AppendFilterChar('o')
	got = v.Render()
	if !strings.Contains(got, "Bob") {
		t.Errorf("filtering for 'Bo' should still show 'Bob', got:\n%s", got)
	}

	v = v.AppendFilterChar('z')
	got = v.Render()
	if strings.Contains(stripANSI(got), "Bob") {
		t.Errorf("filtering for 'Boz' should have no matches, got:\n%s", got)
	}
}

// TestFuzzyFilterShowsFilterBar verifies that the filter bar text is visible
// in the rendered output when the filter is active.
func TestFuzzyFilterShowsFilterBar(t *testing.T) {
	idx := buildIndex(sampleTracks)
	v := View{
		Index:  idx,
		Width:  80,
		Height: 24,
		Fuzzy:  musiclib.FakeFuzzySearcher{},
	}
	v = v.ActivateFilter()
	v = v.AppendFilterChar('A')
	got := v.Render()
	if !strings.Contains(got, "Filter: > A") {
		t.Errorf("render should contain filter bar text 'Filter: > A', got:\n%s", got)
	}
}

// TestFuzzyFilterCursorResetOnKeystroke verifies that the cursor resets to 0
// when a character is appended to the filter query.
func TestFuzzyFilterCursorResetOnKeystroke(t *testing.T) {
	idx := buildIndex(sampleTracks)
	v := View{
		Index:  idx,
		Width:  80,
		Height: 24,
		Fuzzy:  musiclib.FakeFuzzySearcher{},
		Cursor: 2,
	}
	v = v.ActivateFilter()
	v = v.AppendFilterChar('A')
	if v.Cursor != 0 {
		t.Errorf("cursor should reset to 0 on filter keystroke, got %d", v.Cursor)
	}
}

// TestHighlightedRowContainsBoldANSI verifies that a rendered row with
// non-nil MatchedIndexes contains bold ANSI codes at the matched positions.
func TestHighlightedRowContainsBoldANSI(t *testing.T) {
	tracks := []musiclib.Track{
		{Artist: "Alice", Title: "Song One", Album: "Debut", Duration: 185},
	}
	idx := buildIndex(tracks)
	// Positions 0 and 1 in the concatenated "Alice Song One …" string are
	// 'A' and 'l' in the Artist field.
	fake := highlightFake{
		results: []musiclib.Match{
			{TrackIndex: 0, MatchedIndexes: []int{0, 1}},
		},
	}
	v := View{
		Index:  idx,
		Width:  80,
		Height: 24,
		Fuzzy:  fake,
	}
	v = v.ActivateFilter()
	v = v.AppendFilterChar('A')
	v = v.AppendFilterChar('l')
	got := v.Render()
	if !strings.Contains(got, ansiBold) {
		t.Errorf("rendered row should contain ANSI bold code for match highlights, got:\n%q", got)
	}
}

// TestHighlightedPositionsWithinArtistColumn verifies that bold codes appear
// before the correct rune positions within the artist column.
func TestHighlightedPositionsWithinArtistColumn(t *testing.T) {
	tracks := []musiclib.Track{
		{Artist: "Alice", Title: "Song One", Album: "Debut", Duration: 185},
	}
	idx := buildIndex(tracks)
	// Match position 0 → 'A' in Artist.
	fake := highlightFake{
		results: []musiclib.Match{
			{TrackIndex: 0, MatchedIndexes: []int{0}},
		},
	}
	v := View{
		Index:  idx,
		Width:  80,
		Height: 24,
		Fuzzy:  fake,
	}
	v = v.ActivateFilter()
	v = v.AppendFilterChar('A')
	got := v.Render()
	// The bold sequence should precede 'A' somewhere in the output.
	boldA := ansiBold + "A" + ansiReset
	if !strings.Contains(got, boldA) {
		t.Errorf("expected bold-wrapped 'A' (%q) in rendered output, got:\n%q", boldA, got)
	}
}

// TestCursorStoredOnModel verifies that the cursor index is retained on the
// View value after multiple moves.
func TestCursorStoredOnModel(t *testing.T) {
	idx := buildIndex(sampleTracks)
	v := View{Index: idx, Width: 80, Height: 0, Cursor: 0}
	v = v.CursorDown()
	v = v.CursorDown()
	if v.Cursor != 2 {
		t.Errorf("cursor after two CursorDown calls: expected 2, got %d", v.Cursor)
	}
}

// TestCursorRowHighlighted verifies that the focused row is wrapped with ANSI
// accent codes in the rendered output.
func TestCursorRowHighlighted(t *testing.T) {
	idx := buildIndex(sampleTracks)
	v := View{Index: idx, Width: 80, Height: 0, Cursor: 0}
	got := v.Render()
	lines := strings.Split(got, "\n")
	// Track rows begin after the heading, blank line, and header row (index 3).
	if len(lines) < 4 {
		t.Fatalf("expected at least 4 lines, got %d", len(lines))
	}
	focusedRow := lines[3]
	if !strings.Contains(focusedRow, ansiAccent) {
		t.Errorf("focused row should contain ANSI accent code, got: %q", focusedRow)
	}
	// Non-focused rows must not carry the accent code.
	for _, line := range lines[4:] {
		if strings.Contains(line, ansiAccent) {
			t.Errorf("non-focused row should not contain ANSI accent code, got: %q", line)
		}
	}
}
