package library

import (
	"regexp"
	"strings"
	"testing"

	musiclib "github.com/sglavoie/muse/internal/library"
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
	if v.FilterActive {
		t.Error("FilterActive should be false after DeactivateFilter")
	}
	if v.FilterText != "" {
		t.Errorf("FilterText should be empty after DeactivateFilter, got %q", v.FilterText)
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
		Index:        idx,
		Fuzzy:        musiclib.FakeFuzzySearcher{},
		FilterActive: true,
		FilterText:   "Alice",
		Cursor:       0,
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
		Index:        idx,
		Fuzzy:        musiclib.FakeFuzzySearcher{},
		FilterActive: true,
		FilterText:   "Alice",
		Cursor:       0,
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
