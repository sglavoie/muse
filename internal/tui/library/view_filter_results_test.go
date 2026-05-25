package library

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	musiclib "github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/filter"
)

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

func TestAppendFilterCharInvalidatesCachedMatches(t *testing.T) {
	idx := buildIndex(sampleTracks)
	v := View{
		Index:           idx,
		Width:           80,
		Height:          24,
		Fuzzy:           musiclib.FakeFuzzySearcher{},
		Filter:          filter.State{Active: true, Text: "Bob"},
		FilteredMatches: []musiclib.Match{{TrackIndex: 1}},
	}

	v = v.AppendFilterChar('z')
	got := v.Render()
	if !strings.Contains(got, "No matches") {
		t.Fatalf("new query should not reuse cached matches, got:\n%s", got)
	}
	if strings.Contains(ansi.Strip(got), "Track Two") {
		t.Fatalf("stale cached match should not render after query change, got:\n%s", got)
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
