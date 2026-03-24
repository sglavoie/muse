package library

import (
	"math"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	musiclib "github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/filter"
	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

// fakeQueueAppender records every AppendTrack call and returns a
// monotonically-increasing fake queue ID. If err is non-nil it is returned
// from every call without recording the track.
type fakeQueueAppender struct {
	tracks []musiclib.Track
	nextID int64
	err    error
}

func (f *fakeQueueAppender) AppendTrack(t musiclib.Track, _ string) (int64, error) {
	if f.err != nil {
		return 0, f.err
	}
	f.nextID++
	f.tracks = append(f.tracks, t)
	return f.nextID, nil
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

// TestRenderColumnsDoNotOverflow verifies that each row in the rendered output
// has exactly the expected byte length at a given terminal width.
func TestRenderColumnsDoNotOverflow(t *testing.T) {
	idx := buildIndex(sampleTracks)

	for _, width := range []int{40, 80, 120, 160} {
		v := View{Index: idx, Width: width, Height: 0}
		rendered := v.Render()
		lines := strings.Split(rendered, "\n")

		// The header row is lines[0], track rows follow.
		if len(lines) < 1 {
			t.Fatalf("width=%d: expected at least 1 line, got %d", width, len(lines))
		}

		for _, line := range lines {
			got := ansi.StringWidth(line)
			if got != width {
				t.Errorf("width=%d: row has %d display columns, want %d: %q", width, got, width, line)
			}
		}
	}
}

// TestRenderEmptyIndex returns the empty placeholder.
func TestRenderEmptyIndex(t *testing.T) {
	v := View{Width: 80, Height: 24}
	got := v.Render()
	if got != "(no tracks)" {
		t.Errorf("unexpected empty render: %q", got)
	}
}

// TestRenderZeroWidthFallback verifies that a zero Width renders a simple
// "artist – title" format without panic.
func TestRenderZeroWidthFallback(t *testing.T) {
	idx := buildIndex(sampleTracks)
	v := View{Index: idx, Width: 0, Height: 0}
	got := v.Render()
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
// includes an ellipsis. The result is right-padded to the requested width,
// so the ellipsis may not be the final character.
func TestPadTruncation(t *testing.T) {
	got := text.Pad("Hello World", 7)
	if ansi.StringWidth(got) != 7 {
		t.Errorf("pad: expected 7 display columns, got %d: %q", ansi.StringWidth(got), got)
	}
	if !strings.Contains(got, "…") {
		t.Errorf("pad: expected ellipsis in output, got %q", got)
	}
}

// TestPadPadding verifies that pad right-pads short strings with spaces.
func TestPadPadding(t *testing.T) {
	got := text.Pad("Hi", 5)
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
	if v.Filter.Active {
		t.Error("Filter.Active should be false on a zero-value View")
	}
	if v.Filter.Text != "" {
		t.Errorf("Filter.Text should be empty on a zero-value View, got %q", v.Filter.Text)
	}
}

// TestSlashActivatesFilter verifies that calling ActivateFilter sets
// Filter.Active to true (models the '/' keypress).
func TestSlashActivatesFilter(t *testing.T) {
	v := View{}
	v = v.ActivateFilter()
	if !v.Filter.Active {
		t.Error("Filter.Active should be true after ActivateFilter")
	}
}

// TestAppendFilterCharBuildsQuery verifies that key presses after activation
// accumulate into Filter.Text.
func TestAppendFilterCharBuildsQuery(t *testing.T) {
	v := View{}
	v = v.ActivateFilter()
	v = v.AppendFilterChar('r')
	v = v.AppendFilterChar('o')
	v = v.AppendFilterChar('c')
	if v.Filter.Text != "roc" {
		t.Errorf("Filter.Text: expected %q, got %q", "roc", v.Filter.Text)
	}
}

// TestAppendFilterCharIgnoredWhenInactive verifies that AppendFilterChar is a
// no-op when the filter bar has not been activated.
func TestAppendFilterCharIgnoredWhenInactive(t *testing.T) {
	v := View{}
	v = v.AppendFilterChar('x')
	if v.Filter.Text != "" {
		t.Errorf("Filter.Text should remain empty when filter is inactive, got %q", v.Filter.Text)
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
	if strings.Contains(ansi.Strip(got), "Bob") {
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
	v = v.AppendFilterChar('o')
	v = v.AppendFilterChar('b')
	got := v.Render()
	if !strings.Contains(got, "Bob") {
		t.Errorf("filtering for 'Bob' should show 'Bob', got:\n%s", got)
	}
	if strings.Contains(ansi.Strip(got), "Alice") {
		t.Errorf("filtering for 'Bob' should not show 'Alice', got:\n%s", got)
	}

	v = v.AppendFilterChar('z')
	got = v.Render()
	if !strings.Contains(got, "No matches") {
		t.Errorf("filtering for 'Bobz' should show 'No matches', got:\n%s", got)
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
	if !strings.Contains(got, styledText(lipgloss.NewStyle().Bold(true), "A")) && !strings.Contains(got, styledText(lipgloss.NewStyle().Bold(true), "l")) {
		t.Errorf("rendered row should contain bold-styled match highlights, got:\n%q", got)
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
	boldA := styledText(lipgloss.NewStyle().Bold(true), "A")
	if !strings.Contains(got, boldA) {
		t.Errorf("expected bold-wrapped 'A' (%q) in rendered output, got:\n%q", boldA, got)
	}
}

// TestDeactivateFilterClearsQueryAndResetsCursor verifies that DeactivateFilter
// (Esc handler) clears the filter text, deactivates the bar, and resets the
// cursor to 0.
func TestDeactivateFilterClearsQueryAndResetsCursor(t *testing.T) {
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
	v = v.AppendFilterChar('l')
	v = v.DeactivateFilter()
	if v.Filter.Active {
		t.Error("Filter.Active should be false after DeactivateFilter")
	}
	if v.Filter.Text != "" {
		t.Errorf("Filter.Text should be empty after DeactivateFilter, got %q", v.Filter.Text)
	}
	if v.Cursor != 0 {
		t.Errorf("Cursor should be 0 after DeactivateFilter, got %d", v.Cursor)
	}
}

// TestDeactivateFilterRestoresFullTrackList verifies that the full unfiltered
// list is rendered after DeactivateFilter is called.
func TestDeactivateFilterRestoresFullTrackList(t *testing.T) {
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
	v = v.DeactivateFilter()
	got := v.Render()
	if !strings.Contains(got, "Alice") {
		t.Errorf("restored render should contain 'Alice', got:\n%s", got)
	}
	if !strings.Contains(got, "Bob") {
		t.Errorf("restored render should contain 'Bob', got:\n%s", got)
	}
}

// TestNoMatchesEmptyStateCenteredInListArea verifies that when the fuzzy
// search returns zero results, "No matches" is displayed centered in the list
// area and the filter bar remains visible.
func TestNoMatchesEmptyStateCenteredInListArea(t *testing.T) {
	idx := buildIndex(sampleTracks)
	v := View{
		Index:  idx,
		Width:  80,
		Height: 24,
		Fuzzy:  musiclib.FakeFuzzySearcher{},
	}
	v = v.ActivateFilter()
	for _, ch := range "zzzzz" {
		v = v.AppendFilterChar(ch)
	}
	got := v.Render()

	if !strings.Contains(got, "No matches") {
		t.Errorf("expected 'No matches' in zero-result render, got:\n%s", got)
	}
	if !strings.Contains(got, "Filter: >") {
		t.Errorf("filter bar should remain visible in zero-result render, got:\n%s", got)
	}

	// Find the line containing "No matches" and verify it is centered.
	var noMatchesLine string
	for line := range strings.SplitSeq(got, "\n") {
		if strings.Contains(line, "No matches") {
			noMatchesLine = line
			break
		}
	}
	if noMatchesLine == "" {
		t.Fatal("could not find 'No matches' line in output")
	}
	if !strings.HasPrefix(noMatchesLine, " ") {
		t.Errorf("'No matches' should be indented for centering, got: %q", noMatchesLine)
	}
}

// TestNoMatchesZeroWidthFallback verifies that zero-width mode also renders
// "No matches" when the filter produces no results.
func TestNoMatchesZeroWidthFallback(t *testing.T) {
	idx := buildIndex(sampleTracks)
	v := View{
		Index:  idx,
		Width:  0,
		Height: 24,
		Fuzzy:  musiclib.FakeFuzzySearcher{},
	}
	v = v.ActivateFilter()
	for _, ch := range "zzzzz" {
		v = v.AppendFilterChar(ch)
	}
	got := v.Render()
	if !strings.Contains(got, "No matches") {
		t.Errorf("zero-width render should contain 'No matches' when filter has no results, got:\n%s", got)
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

// TestCursorRowHighlighted verifies that the focused row carries styling
// (i.e. ANSI escapes are present) while non-focused rows are plain text.
func TestCursorRowHighlighted(t *testing.T) {
	idx := buildIndex(sampleTracks)
	s := styles.NewStyles(true).Accent
	accentMarker := styledText(s, "x") // produces a reverse-video ANSI sequence
	if ansi.Strip(accentMarker) == accentMarker {
		t.Skip("lipgloss produced no ANSI output in this terminal environment")
	}

	v := View{Index: idx, Width: 80, Height: 0, Cursor: 0}
	v.Styles.Accent = s
	got := v.Render()
	lines := strings.Split(got, "\n")
	// Track rows begin after the header row (index 1).
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 lines, got %d", len(lines))
	}
	focusedRow := lines[1]
	if ansi.Strip(focusedRow) == focusedRow {
		t.Errorf("focused row should contain ANSI styling, got: %q", focusedRow)
	}
	// Non-focused rows must not carry styling.
	for _, line := range lines[2:] {
		if ansi.Strip(line) != line {
			t.Errorf("non-focused row should not contain ANSI styling, got: %q", line)
		}
	}
}

// TestFocusedTrackNoFilter verifies that FocusedTrack returns the track at
// the cursor position when no filter is active.
func TestFocusedTrackNoFilter(t *testing.T) {
	idx := buildIndex(sampleTracks)
	v := View{Index: idx, Cursor: 1}
	track, ok := v.FocusedTrack()
	if !ok {
		t.Fatal("FocusedTrack: expected ok=true, got false")
	}
	tracks := idx.Tracks()
	if track.Artist != tracks[1].Artist || track.Title != tracks[1].Title {
		t.Errorf("FocusedTrack: expected %q/%q, got %q/%q",
			tracks[1].Artist, tracks[1].Title, track.Artist, track.Title)
	}
}

// TestFocusedTrackEmptyLibrary verifies that FocusedTrack returns false when
// the library has no tracks.
func TestFocusedTrackEmptyLibrary(t *testing.T) {
	v := View{}
	_, ok := v.FocusedTrack()
	if ok {
		t.Error("FocusedTrack: expected ok=false for empty library, got true")
	}
}

// TestFocusedTrackWithActiveFilter verifies that FocusedTrack indexes into the
// filtered result set, not the full track list, when a filter is active.
func TestFocusedTrackWithActiveFilter(t *testing.T) {
	idx := buildIndex(sampleTracks)
	v := View{
		Index:  idx,
		Fuzzy:  musiclib.FakeFuzzySearcher{},
		Filter: filter.State{Active: true, Text: "Alice"},
		Cursor: 0,
	}
	track, ok := v.FocusedTrack()
	if !ok {
		t.Fatal("FocusedTrack with filter: expected ok=true, got false")
	}
	if track.Artist != "Alice" {
		t.Errorf("FocusedTrack with filter: expected Artist %q, got %q", "Alice", track.Artist)
	}
}

// TestEnqueueFocusedSendsCorrectTrack verifies that the tea.Cmd returned by
// EnqueueFocused calls AppendTrack with the focused track and returns a
// TrackEnqueuedMsg containing that track.
func TestEnqueueFocusedSendsCorrectTrack(t *testing.T) {
	idx := buildIndex(sampleTracks)
	v := View{Index: idx, Cursor: 0}
	focused := idx.Tracks()[0]

	qa := &fakeQueueAppender{}
	_, cmd := v.EnqueueFocused(qa, 0)
	if cmd == nil {
		t.Fatal("EnqueueFocused: expected non-nil Cmd, got nil")
	}

	msg := cmd()
	enqueued, ok := msg.(TrackEnqueuedMsg)
	if !ok {
		t.Fatalf("EnqueueFocused: cmd returned %T, want TrackEnqueuedMsg", msg)
	}
	if enqueued.Err != nil {
		t.Errorf("EnqueueFocused: unexpected error: %v", enqueued.Err)
	}
	if enqueued.Track.Artist != focused.Artist || enqueued.Track.Title != focused.Title {
		t.Errorf("EnqueueFocused: track mismatch: got %q/%q, want %q/%q",
			enqueued.Track.Artist, enqueued.Track.Title, focused.Artist, focused.Title)
	}
	if len(qa.tracks) != 1 || qa.tracks[0].Artist != focused.Artist {
		t.Errorf("EnqueueFocused: AppendTrack called with wrong track or wrong number of times")
	}
}

// TestEnqueueFocusedDuplicatesAccepted verifies that enqueueing the same track
// twice is accepted without error, so duplicate queue entries are permitted.
func TestEnqueueFocusedDuplicatesAccepted(t *testing.T) {
	idx := buildIndex(sampleTracks)
	v := View{Index: idx, Cursor: 0}
	qa := &fakeQueueAppender{}

	_, cmd1 := v.EnqueueFocused(qa, 0)
	_, cmd2 := v.EnqueueFocused(qa, 1)

	msg1 := cmd1()
	if e, ok := msg1.(TrackEnqueuedMsg); !ok || e.Err != nil {
		t.Fatalf("first enqueue failed: %v", msg1)
	}
	msg2 := cmd2()
	if e, ok := msg2.(TrackEnqueuedMsg); !ok || e.Err != nil {
		t.Fatalf("second enqueue of same track failed: %v", msg2)
	}
	if len(qa.tracks) != 2 {
		t.Errorf("expected 2 AppendTrack calls for duplicates, got %d", len(qa.tracks))
	}
}

// TestEnqueueFocusedWasEmptyTrueWhenQueueEmpty verifies that
// TrackEnqueuedMsg.WasEmpty is true when prevQueueLen is 0.
func TestEnqueueFocusedWasEmptyTrueWhenQueueEmpty(t *testing.T) {
	idx := buildIndex(sampleTracks)
	v := View{Index: idx, Cursor: 0}
	qa := &fakeQueueAppender{}

	_, cmd := v.EnqueueFocused(qa, 0)
	msg := cmd().(TrackEnqueuedMsg)
	if !msg.WasEmpty {
		t.Error("WasEmpty should be true when prevQueueLen is 0")
	}
}

// TestEnqueueFocusedWasEmptyFalseWhenQueueNonEmpty verifies that
// TrackEnqueuedMsg.WasEmpty is false when prevQueueLen is greater than zero.
func TestEnqueueFocusedWasEmptyFalseWhenQueueNonEmpty(t *testing.T) {
	idx := buildIndex(sampleTracks)
	v := View{Index: idx, Cursor: 0}
	qa := &fakeQueueAppender{}

	_, cmd := v.EnqueueFocused(qa, 3)
	msg := cmd().(TrackEnqueuedMsg)
	if msg.WasEmpty {
		t.Error("WasEmpty should be false when prevQueueLen > 0")
	}
}

// TestEnqueueFocusedNilCmdOnEmptyLibrary verifies that EnqueueFocused returns
// a nil Cmd when the library contains no tracks.
func TestEnqueueFocusedNilCmdOnEmptyLibrary(t *testing.T) {
	v := View{}
	qa := &fakeQueueAppender{}
	_, cmd := v.EnqueueFocused(qa, 0)
	if cmd != nil {
		t.Error("EnqueueFocused on empty library: expected nil Cmd, got non-nil")
	}
}

// TestEnqueueFocusedWithActiveFilterSelectsFilteredTrack verifies that when a
// filter is active, EnqueueFocused enqueues the track at cursor position in
// the filtered result set rather than the unfiltered full list. This locks in
// the FocusedTrack filter-branch behaviour and acts as a regression guard for
// any code that reconstructs a View for enqueue purposes.
func TestEnqueueFocusedWithActiveFilterSelectsFilteredTrack(t *testing.T) {
	idx := buildIndex(sampleTracks)
	// FakeFuzzySearcher matches "Alice" only against the Alice track (index 0
	// in the sorted index). Cursor 0 should therefore resolve to Alice, not to
	// whatever track sits at position 0 in the unfiltered list.
	v := View{
		Index:  idx,
		Fuzzy:  musiclib.FakeFuzzySearcher{},
		Filter: filter.State{Active: true, Text: "Alice"},
		Cursor: 0,
	}

	qa := &fakeQueueAppender{}
	_, cmd := v.EnqueueFocused(qa, 0)
	if cmd == nil {
		t.Fatal("EnqueueFocused with active filter: expected non-nil Cmd, got nil")
	}
	msg, ok := cmd().(TrackEnqueuedMsg)
	if !ok {
		t.Fatalf("EnqueueFocused: cmd returned unexpected type")
	}
	if msg.Err != nil {
		t.Fatalf("EnqueueFocused with filter: unexpected error: %v", msg.Err)
	}
	if msg.Track.Artist != "Alice" {
		t.Errorf("EnqueueFocused with active filter: expected Artist %q, got %q", "Alice", msg.Track.Artist)
	}
}

// --- navigation: GoToTop / GoToBottom ---

func TestGoToTopResetsCursorAndScroll(t *testing.T) {
	idx := buildIndex(sampleTracks)
	v := View{Index: idx, Width: 80, Height: 24, Cursor: 2, ScrollOffset: 1}
	v = v.GoToTop()
	if v.Cursor != 0 {
		t.Errorf("GoToTop: expected Cursor=0, got %d", v.Cursor)
	}
	if v.ScrollOffset != 0 {
		t.Errorf("GoToTop: expected ScrollOffset=0, got %d", v.ScrollOffset)
	}
}

func TestGoToBottomMovesCursorToLast(t *testing.T) {
	idx := buildIndex(sampleTracks)
	v := View{Index: idx, Width: 80, Height: 24, Cursor: 0}
	v = v.GoToBottom()
	want := len(sampleTracks) - 1
	if v.Cursor != want {
		t.Errorf("GoToBottom: expected Cursor=%d, got %d", want, v.Cursor)
	}
}

func TestGoToBottomEmptyLibrary(t *testing.T) {
	v := View{Width: 80, Height: 24, Cursor: 0}
	v = v.GoToBottom()
	if v.Cursor != 0 {
		t.Errorf("GoToBottom empty: expected Cursor=0, got %d", v.Cursor)
	}
}

func TestGoToBottomAdjustsScrollOffset(t *testing.T) {
	tracks := make([]musiclib.Track, 20)
	for i := range tracks {
		tracks[i] = musiclib.Track{Artist: "A", Title: strings.Repeat("T", i+1), Hash: strings.Repeat("h", i+1)}
	}
	idx := buildIndex(tracks)
	v := View{Index: idx, Width: 80, Height: 6, Cursor: 0} // visibleRows = 5
	v = v.GoToBottom()
	if v.Cursor != 19 {
		t.Errorf("GoToBottom: expected Cursor=19, got %d", v.Cursor)
	}
	if v.ScrollOffset != 15 {
		t.Errorf("GoToBottom: expected ScrollOffset=15, got %d", v.ScrollOffset)
	}
}

// --- navigation: page movements ---

func TestHalfPageDownMovesByHalfVisibleRows(t *testing.T) {
	tracks := make([]musiclib.Track, 20)
	for i := range tracks {
		tracks[i] = musiclib.Track{Artist: "A", Title: strings.Repeat("T", i+1), Hash: strings.Repeat("h", i+1)}
	}
	idx := buildIndex(tracks)
	v := View{Index: idx, Width: 80, Height: 13, Cursor: 0} // visibleRows = 12
	v = v.HalfPageDown()
	if v.Cursor != 6 {
		t.Errorf("HalfPageDown: expected Cursor=6, got %d", v.Cursor)
	}
}

func TestHalfPageUpMovesByHalfVisibleRows(t *testing.T) {
	tracks := make([]musiclib.Track, 20)
	for i := range tracks {
		tracks[i] = musiclib.Track{Artist: "A", Title: strings.Repeat("T", i+1), Hash: strings.Repeat("h", i+1)}
	}
	idx := buildIndex(tracks)
	v := View{Index: idx, Width: 80, Height: 13, Cursor: 12} // visibleRows = 12
	v = v.HalfPageUp()
	if v.Cursor != 6 {
		t.Errorf("HalfPageUp: expected Cursor=6, got %d", v.Cursor)
	}
}

func TestFullPageDownMovesByVisibleRows(t *testing.T) {
	tracks := make([]musiclib.Track, 20)
	for i := range tracks {
		tracks[i] = musiclib.Track{Artist: "A", Title: strings.Repeat("T", i+1), Hash: strings.Repeat("h", i+1)}
	}
	idx := buildIndex(tracks)
	v := View{Index: idx, Width: 80, Height: 13, Cursor: 0} // visibleRows = 12
	v = v.FullPageDown()
	if v.Cursor != 12 {
		t.Errorf("FullPageDown: expected Cursor=12, got %d", v.Cursor)
	}
}

func TestFullPageUpMovesByVisibleRows(t *testing.T) {
	tracks := make([]musiclib.Track, 20)
	for i := range tracks {
		tracks[i] = musiclib.Track{Artist: "A", Title: strings.Repeat("T", i+1), Hash: strings.Repeat("h", i+1)}
	}
	idx := buildIndex(tracks)
	v := View{Index: idx, Width: 80, Height: 13, Cursor: 15} // visibleRows = 12
	v = v.FullPageUp()
	if v.Cursor != 3 {
		t.Errorf("FullPageUp: expected Cursor=3, got %d", v.Cursor)
	}
}

func TestPageMovementsClampsToEnd(t *testing.T) {
	idx := buildIndex(sampleTracks) // 3 tracks
	v := View{Index: idx, Width: 80, Height: 13, Cursor: 2}
	v = v.FullPageDown()
	if v.Cursor != 2 {
		t.Errorf("FullPageDown clamp: expected Cursor=2, got %d", v.Cursor)
	}
}

func TestPageMovementsClampsToStart(t *testing.T) {
	idx := buildIndex(sampleTracks)
	v := View{Index: idx, Width: 80, Height: 13, Cursor: 1}
	v = v.FullPageUp()
	if v.Cursor != 0 {
		t.Errorf("FullPageUp clamp: expected Cursor=0, got %d", v.Cursor)
	}
}

func TestMoveCursorEmptyIsNoOp(t *testing.T) {
	v := View{Width: 80, Height: 24}
	v2 := v.moveCursor(5)
	if v2.Cursor != 0 {
		t.Errorf("moveCursor empty: expected Cursor=0, got %d", v2.Cursor)
	}
}

// --- CommitFilter ---

func TestCommitFilterDeactivatesButKeepsText(t *testing.T) {
	v := View{Filter: filter.State{Active: true, Text: "abc"}}
	v = v.CommitFilter()
	if v.Filter.Active {
		t.Error("CommitFilter: expected Filter.Active=false")
	}
	if v.Filter.Text != "abc" {
		t.Errorf("CommitFilter: expected Filter.Text='abc', got %q", v.Filter.Text)
	}
}

// --- FormatDurationSecs (via text package) ---

func TestFormatDurationMinutesSeconds(t *testing.T) {
	got := text.FormatDurationSecs(185)
	if got != "3:05" {
		t.Errorf("FormatDurationSecs(185): expected 3:05, got %s", got)
	}
}

func TestFormatDurationHours(t *testing.T) {
	got := text.FormatDurationSecs(3661)
	if got != "1:01:01" {
		t.Errorf("FormatDurationSecs(3661): expected 1:01:01, got %s", got)
	}
}

func TestFormatDurationZero(t *testing.T) {
	got := text.FormatDurationSecs(0)
	if got != "--:--" {
		t.Errorf("FormatDurationSecs(0): expected --:--, got %s", got)
	}
}

func TestFormatDurationNegative(t *testing.T) {
	got := text.FormatDurationSecs(-5)
	if got != "--:--" {
		t.Errorf("FormatDurationSecs(-5): expected --:--, got %s", got)
	}
}

// --- formatCount ---

func TestFormatCountSmall(t *testing.T) {
	got := formatCount(42)
	if got != "42" {
		t.Errorf("formatCount(42): expected 42, got %s", got)
	}
}

func TestFormatCountThousands(t *testing.T) {
	got := formatCount(1234)
	if got != "1,234" {
		t.Errorf("formatCount(1234): expected 1,234, got %s", got)
	}
}

func TestFormatCountMillions(t *testing.T) {
	got := formatCount(1234567)
	if got != "1,234,567" {
		t.Errorf("formatCount(1234567): expected 1,234,567, got %s", got)
	}
}

func TestFormatCountExactlyThousand(t *testing.T) {
	got := formatCount(1000)
	if got != "1,000" {
		t.Errorf("formatCount(1000): expected 1,000, got %s", got)
	}
}

// --- visibleRows ---

func TestVisibleRowsZeroHeight(t *testing.T) {
	v := View{Height: 0}
	if v.visibleRows() != 50 {
		t.Errorf("visibleRows Height=0: expected 50, got %d", v.visibleRows())
	}
}

func TestVisibleRowsSmallHeight(t *testing.T) {
	v := View{Height: 4}
	if v.visibleRows() != 3 {
		t.Errorf("visibleRows Height=4: expected 3, got %d", v.visibleRows())
	}
}

// --- pad edge cases ---

func TestPadZeroWidth(t *testing.T) {
	got := text.Pad("hello", 0)
	if got != "" {
		t.Errorf("pad width=0: expected empty, got %q", got)
	}
}

func TestPadWidthOne(t *testing.T) {
	got := text.Pad("hello", 1)
	if got != "…" {
		t.Errorf("pad width=1: expected '…', got %q", got)
	}
}

func TestPadExactFit(t *testing.T) {
	got := text.Pad("hi", 2)
	if got != "hi" {
		t.Errorf("pad exact fit: expected 'hi', got %q", got)
	}
}

// TestVisibleRowsSameWithAndWithoutFilter verifies that visibleRows returns
// the same value regardless of whether a filter is active. The column header
// always consumes exactly 1 line, so the number of data rows should not change.
func TestVisibleRowsSameWithAndWithoutFilter(t *testing.T) {
	h := 24
	noFilter := View{Height: h}
	withFilter := View{Height: h, Filter: filter.State{Text: "abc"}}
	withActiveFilter := View{Height: h, Filter: filter.State{Active: true, Text: "abc"}}

	base := noFilter.visibleRows()
	if got := withFilter.visibleRows(); got != base {
		t.Errorf("visibleRows with committed filter: got %d, want %d (same as no filter)", got, base)
	}
	if got := withActiveFilter.visibleRows(); got != base {
		t.Errorf("visibleRows with active filter: got %d, want %d (same as no filter)", got, base)
	}
}

// --- field-specific filter rendering ---

// TestFieldSpecificAlbumHighlight verifies that a field-specific album query
// (l:debut) produces bold ANSI codes in the album column.
func TestFieldSpecificAlbumHighlight(t *testing.T) {
	tracks := []musiclib.Track{
		{Artist: "Alice", Title: "Song One", Album: "Debut", Duration: 185},
	}
	idx := buildIndex(tracks)
	fake := highlightFake{
		results: []musiclib.Match{
			{TrackIndex: 0, AlbumPositions: []int{0, 1, 2, 3, 4}},
		},
	}
	v := View{
		Index:  idx,
		Width:  80,
		Height: 24,
		Fuzzy:  fake,
	}
	v = v.ActivateFilter()
	v = v.AppendFilterChar('l')
	v = v.AppendFilterChar(':')
	v = v.AppendFilterChar('d')
	got := v.Render()
	boldD := styledText(lipgloss.NewStyle().Bold(true), "D")
	if !strings.Contains(got, boldD) {
		t.Errorf("expected bold 'D' in album column for field-specific query, got:\n%q", got)
	}
}

// TestFieldSpecificArtistNarrowsResults verifies that a:alice only shows
// tracks with "alice" in the artist field.
func TestFieldSpecificArtistNarrowsResults(t *testing.T) {
	idx := buildIndex(sampleTracks)
	v := View{
		Index:  idx,
		Width:  80,
		Height: 24,
		Fuzzy:  musiclib.FakeFuzzySearcher{},
	}
	v = v.ActivateFilter()
	for _, ch := range "a:Alice" {
		v = v.AppendFilterChar(ch)
	}
	got := v.Render()
	if !strings.Contains(got, "Alice") {
		t.Errorf("a:Alice should show Alice tracks, got:\n%s", got)
	}
	if strings.Contains(ansi.Strip(got), "Bob") {
		t.Errorf("a:Alice should not show Bob tracks, got:\n%s", got)
	}
}

// TestGeneralSearchMatchesAlbum verifies that a plain (unprefixed) query
// matches album text. This is new behaviour — previously album was excluded.
func TestGeneralSearchMatchesAlbum(t *testing.T) {
	idx := buildIndex(sampleTracks)
	v := View{
		Index:  idx,
		Width:  80,
		Height: 24,
		Fuzzy:  musiclib.FakeFuzzySearcher{},
	}
	v = v.ActivateFilter()
	for _, ch := range "Debut" {
		v = v.AppendFilterChar(ch)
	}
	got := v.Render()
	if !strings.Contains(got, "Alice") {
		t.Errorf("searching 'Debut' should match Alice's album, got:\n%s", got)
	}
}

// TestFilterBarHintShownWhenEmpty verifies that the dim hint text is shown
// when the filter bar is active but no text has been typed yet.
func TestFilterBarHintShownWhenEmpty(t *testing.T) {
	idx := buildIndex(sampleTracks)
	dimStyle := styles.NewStyles(true).Dim
	v := View{
		Index:  idx,
		Width:  80,
		Height: 24,
		Fuzzy:  musiclib.FakeFuzzySearcher{},
	}
	v.Styles.Dim = dimStyle
	v = v.ActivateFilter()
	got := v.Render()
	// The hint should contain the prefix shortcuts.
	if !strings.Contains(got, "a: artist") {
		t.Errorf("filter bar hint should show 'a: artist' when empty, got:\n%s", got)
	}
	if !strings.Contains(got, "t: title") {
		t.Errorf("filter bar hint should show 't: title' when empty, got:\n%s", got)
	}
	if !strings.Contains(got, "l: album") {
		t.Errorf("filter bar hint should show 'l: album' when empty, got:\n%s", got)
	}
	// The hint text should be wrapped in ANSI dim styling.
	hint := "a: artist  t: title  l: album"
	styledHint := styledText(dimStyle, hint)
	if ansi.Strip(styledHint) == styledHint {
		t.Skip("lipgloss produced no ANSI output in this terminal environment")
	}
	if !strings.Contains(got, styledHint) {
		t.Errorf("filter bar hint should use dim styling, got:\n%q", got)
	}
}

// TestFilterBarHintDisappearsOnInput verifies that the dim hint text is
// replaced by the actual filter text once the user starts typing.
func TestFilterBarHintDisappearsOnInput(t *testing.T) {
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
	if strings.Contains(got, "a: artist") {
		t.Errorf("filter bar hint should not appear once text is entered, got:\n%s", got)
	}
	if !strings.Contains(got, "Filter: > A") {
		t.Errorf("filter bar should show typed text, got:\n%s", got)
	}
}

// TestFieldSpecificMatchedIndexesNil verifies that for field-specific queries,
// the general MatchedIndexes path is not used (album is highlighted via per-field
// positions, not via the concatenated string offsets).
func TestFieldSpecificMatchedIndexesNil(t *testing.T) {
	tracks := []musiclib.Track{
		{Artist: "Radiohead", Title: "Creep", Album: "Pablo Honey", Duration: 238},
	}
	idx := buildIndex(tracks)
	fake := highlightFake{
		results: []musiclib.Match{
			{
				TrackIndex:      0,
				ArtistPositions: []int{0, 1, 2, 3, 4, 5, 6, 7, 8},
				// MatchedIndexes intentionally nil for field-specific
			},
		},
	}
	v := View{
		Index:  idx,
		Width:  120,
		Height: 24,
		Fuzzy:  fake,
	}
	v = v.ActivateFilter()
	for _, ch := range "a:radiohead" {
		v = v.AppendFilterChar(ch)
	}
	got := v.Render()
	// Artist should be highlighted
	boldR := styledText(lipgloss.NewStyle().Bold(true), "R")
	if !strings.Contains(got, boldR) {
		t.Errorf("expected bold 'R' in artist column for field-specific query, got:\n%q", got)
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

// TestGeneralSearchHighlightsAlbumColumn verifies that when a general search
// matches text within the album field, the album column now shows bold
// highlights (previously album was not highlighted at all).
func TestGeneralSearchHighlightsAlbumColumn(t *testing.T) {
	tracks := []musiclib.Track{
		{Artist: "Alice", Title: "Song One", Album: "Debut", Duration: 185},
	}
	idx := buildIndex(tracks)
	// Positions 15-19 in "Alice Song One Debut file" correspond to "Debut".
	fake := highlightFake{
		results: []musiclib.Match{
			{TrackIndex: 0, MatchedIndexes: []int{15, 16, 17, 18, 19}},
		},
	}
	v := View{
		Index:  idx,
		Width:  80,
		Height: 24,
		Fuzzy:  fake,
	}
	v = v.ActivateFilter()
	for _, ch := range "Debut" {
		v = v.AppendFilterChar(ch)
	}
	got := v.Render()
	boldD := styledText(lipgloss.NewStyle().Bold(true), "D")
	if !strings.Contains(got, boldD) {
		t.Errorf("expected bold 'D' in album column for general search matching album, got:\n%q", got)
	}
}
