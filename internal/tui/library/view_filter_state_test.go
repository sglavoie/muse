package library

import (
	"strings"
	"testing"

	musiclib "github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/filter"
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

// TestAppendFilterCharResetsScrollOffset verifies that scrolling offset is
// reset to 0 when a character is appended. Without this reset, a stale scroll
// position above the filtered result count would render "No matches" even
// though matches exist.
func TestAppendFilterCharResetsScrollOffset(t *testing.T) {
	// Build a large enough library that we can scroll past the filter result count.
	tracks := make([]musiclib.Track, 100)
	for i := range tracks {
		tracks[i] = musiclib.Track{Artist: "Other", Title: "Track", Hash: string(rune('a' + i%26))}
	}
	tracks[0] = musiclib.Track{Artist: "Alice", Title: "Song", Hash: "alice-hash"}
	idx := buildIndex(tracks)
	v := View{
		Index:        idx,
		Width:        80,
		Height:       24,
		Fuzzy:        musiclib.FakeFuzzySearcher{},
		Cursor:       50,
		ScrollOffset: 50,
	}
	v = v.ActivateFilter()
	v = v.AppendFilterChar('A')
	if v.ScrollOffset != 0 {
		t.Errorf("ScrollOffset should reset to 0 on filter keystroke (was %d, want 0)", v.ScrollOffset)
	}
	// Confirm the rendered output actually shows the matching row instead of "No matches".
	got := v.Render()
	if strings.Contains(got, "No matches") {
		t.Errorf("filtered render should not show 'No matches' when matches exist; got:\n%s", got)
	}
	if !strings.Contains(got, "Alice") {
		t.Errorf("filtered render should show Alice; got:\n%s", got)
	}
}

// TestDeactivateFilterResetsScrollOffset verifies that ScrollOffset is reset
// to 0 when the filter is deactivated. Otherwise a scroll position from the
// filtered view could leave the cursor below the visible window after the
// list expands back.
func TestDeactivateFilterResetsScrollOffset(t *testing.T) {
	idx := buildIndex(sampleTracks)
	v := View{
		Index:        idx,
		Width:        80,
		Height:       24,
		Fuzzy:        musiclib.FakeFuzzySearcher{},
		Cursor:       2,
		ScrollOffset: 2,
	}
	v = v.ActivateFilter()
	v = v.AppendFilterChar('A')
	v = v.DeactivateFilter()
	if v.ScrollOffset != 0 {
		t.Errorf("ScrollOffset should reset to 0 after DeactivateFilter, got %d", v.ScrollOffset)
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

func TestDeactivateFilterClearsCachedMatches(t *testing.T) {
	v := View{
		Filter:          filter.State{Active: true, Text: "Alice"},
		FilteredMatches: []musiclib.Match{{TrackIndex: 0}},
	}

	v = v.DeactivateFilter()

	if v.FilteredMatches != nil {
		t.Fatalf("DeactivateFilter should clear cached matches, got %#v", v.FilteredMatches)
	}
}

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
