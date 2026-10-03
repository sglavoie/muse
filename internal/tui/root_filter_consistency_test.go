package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/tui/browse"
	"github.com/sglavoie/muse/internal/tui/keys"
)

// Filtering must have the same escape and palette behavior across views,
// without accidentally navigating back or clearing a different view's query.
func TestCommittedFilterClearAcrossViews(t *testing.T) {
	cases := []struct {
		name  string
		setup func(*Model)
	}{
		{"library", func(m *Model) { m.ActiveView = viewLibrary }},
		{"queue", func(m *Model) { m.ActiveView = viewQueue }},
		{"artists", func(m *Model) { m.ActiveView = viewBrowse; m.browse.Mode = browse.BrowseModeArtists }},
		{"albums", func(m *Model) { m.ActiveView = viewBrowse; m.browse.Mode = browse.BrowseModeAlbums }},
		{"genres", func(m *Model) { m.ActiveView = viewBrowse; m.browse.Mode = browse.BrowseModeGenres }},
		{"favorites", func(m *Model) { m.ActiveView = viewFavorites }},
		{"tags", func(m *Model) { m.ActiveView = viewTags }},
		{"mixes", func(m *Model) {
			*m = m.switchToView(viewMixes)
			m.mixes.OpenMix(m.mixes.Mixes[0].ID)
			*m = m.handleMixLoadedMsg(m.loadMixCmd(m.mixes.ActiveMix)().(mixLoadedMsg))
		}},
		{"insights", func(m *Model) { m.ActiveView = viewInsights; m.insights.OpenDrill("Tracks", m.library.Index.Tracks()) }},
	}
	for _, tc := range cases {
		for _, method := range []string{"escape", "palette"} {
			t.Run(tc.name+"/"+method, func(t *testing.T) {
				m := newRichFixture()
				tc.setup(&m)
				m = pressSlash(m)
				m = typeText(m, "zzzzzz")
				m = pressEnter(m)
				if !m.hasCommittedFilter() || !paletteCommandEnabledForTest(t, m, "Clear view filter") {
					t.Fatal("committed filter must enable the clear command and badge")
				}
				if !strings.Contains(ansi.Strip(m.titleBarFilterBadge()), "[filtered]") {
					t.Fatal("missing committed-filter badge")
				}
				if method == "escape" {
					m = pressEsc(m)
				} else {
					m = dispatchPaletteAction(t, m, commandPaletteActionMsg{action: keys.ActionLibraryFilterClear})
				}
				if m.hasCommittedFilter() || m.allFilterOps() == nil || m.allFilterOps().hasText() {
					t.Fatal("clear must remove the filter without leaving the filterable view")
				}
				if paletteCommandEnabledForTest(t, m, "Clear view filter") {
					t.Fatal("clear should be disabled without a query")
				}
				if tc.name == "mixes" && m.mixes.Level != 1 {
					t.Fatal("first escape must stay inside the mix")
				}
				if tc.name == "insights" && !m.insights.AtTracks() {
					t.Fatal("first escape must stay inside the drill-down")
				}
			})
		}
	}
}

func TestMixFilterEscapePrecedence(t *testing.T) {
	m := newRichFixture()
	m = m.switchToView(viewMixes)
	m.mixes.OpenMix(m.mixes.Mixes[0].ID)
	m = m.handleMixLoadedMsg(m.loadMixCmd(m.mixes.ActiveMix)().(mixLoadedMsg))
	m = pressSlash(m)
	m = typeText(m, "Alice")
	m = pressEnter(m)
	m = pressKey(m, "?")
	m = pressEsc(m)
	if m.helpVisible || !m.hasCommittedFilter() {
		t.Fatal("escape must close help before clearing the query")
	}
	m.scanning = true
	m = pressEsc(m)
	if m.scanning || !m.hasCommittedFilter() {
		t.Fatal("escape must cancel the scan before clearing the query")
	}
	m = pressEsc(m)
	if m.hasCommittedFilter() || m.mixes.Level != 1 || len(m.mixes.TrackView.Panel().VisibleTracks()) != 3 {
		t.Fatal("escape must restore the mix's full track list")
	}
	m = pressEsc(m)
	if m.mixes.Level != 0 {
		t.Fatal("second escape must return to the mix list")
	}
}
