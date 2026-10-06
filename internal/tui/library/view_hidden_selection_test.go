package library

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	musiclib "github.com/sglavoie/muse/internal/library"
)

func TestHiddenSelectionTracksFiltersNotViewport(t *testing.T) {
	tracks := []musiclib.Track{{Hash: "a", Title: "Alpha"}, {Hash: "b", Title: "Beta"}, {Hash: "c", Title: "Gamma"}}
	v := View{Index: buildIndex(tracks), Width: 100, Height: 3, Fuzzy: musiclib.FakeFuzzySearcher{},
		SelectedHashes: map[string]struct{}{"a": {}, "b": {}}}
	check := func(want int) {
		t.Helper()
		panel := v.Panel()
		if got := panel.hiddenSelectionCount(); got != want {
			t.Fatalf("hidden selection=%d, want %d", got, want)
		}
		if got := ansi.Strip(panel.renderStateStrip(3, len(panel.VisibleTracks()))); want == 0 && strings.Contains(got, "hidden") {
			t.Fatalf("no hidden selection should mean no label: %q", got)
		}
	}
	check(0) // A short viewport alone never hides selected tracks from bulk actions.
	v.Filter.Text = "Alpha"
	v = v.RebuildFilterCache()
	check(1)
	v.TagFilterHashes = map[string]struct{}{"c": {}}
	v = v.RefreshView()
	check(2) // Text and tag filters intersect to an empty list.
	v = v.ClearFilter()
	check(2) // Clearing text leaves the tag filter in force.
	v.TagFilterHashes = nil
	v = v.RefreshView()
	check(0)
	v.SourceTracks = []musiclib.Track{tracks[0], tracks[0]}
	v = v.RefreshView()
	check(1) // Repeated History rows count a selected hash only once.
	if len(v.SelectedHashes) != 2 {
		t.Fatal("rendering changed the selection")
	}
}
