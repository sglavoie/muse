package library

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	musiclib "github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/filter"
	"github.com/sglavoie/muse/internal/tui/styles"
)

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
	if !strings.Contains(ansi.Strip(got), `filter "`) {
		t.Errorf("render should contain filter chunk 'filter \"...\"', got:\n%s", got)
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
	if !strings.Contains(ansi.Strip(got), `filter "`) {
		t.Errorf("filter chunk should remain visible in zero-result render, got:\n%s", got)
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
	boldDebut := styledText(lipgloss.NewStyle().Bold(true), "Debut")
	if !strings.Contains(got, boldDebut) {
		t.Errorf("expected bold 'Debut' in album column for field-specific query, got:\n%q", got)
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
	hint := "a: artist  t: title  l: album  g: genre"
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
	if !strings.Contains(ansi.Strip(got), `filter "`) {
		t.Errorf("filter chunk should show typed text, got:\n%s", got)
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
	// Artist should be highlighted (all 9 chars grouped into one bold run)
	boldRadiohead := styledText(lipgloss.NewStyle().Bold(true), "Radiohead")
	if !strings.Contains(got, boldRadiohead) {
		t.Errorf("expected bold 'Radiohead' in artist column for field-specific query, got:\n%q", got)
	}
}
