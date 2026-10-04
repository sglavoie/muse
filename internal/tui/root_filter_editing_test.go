package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/tui/browse"
)

func TestFilterEditingFooterAcrossViews(t *testing.T) {
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
		t.Run(tc.name, func(t *testing.T) {
			m := newRichFixture()
			tc.setup(&m)
			m.showFooterHints = true
			m.termWidth = 60
			m = pressSlash(m)
			if m.activeFilterOps() == nil {
				t.Fatal("filter did not open")
			}
			got := ansi.Strip(m.renderFooterHint())
			if got != "  enter apply · esc clear · ↑/↓ history" || ansi.StringWidth(got) > 60 {
				t.Fatalf("editing footer = %q", got)
			}
			m = pressEnter(m)
			if strings.Contains(ansi.Strip(m.renderFooterHint()), "enter apply") {
				t.Fatal("editing footer survived commit")
			}
			m = pressSlash(m)
			m.showFooterHints = false
			if m.renderFooterHint() != "" {
				t.Fatal("hidden hints must stay hidden while editing")
			}
		})
	}
}

func TestFilterFooterTakesPriorityOverSelection(t *testing.T) {
	m := newRichFixture()
	m.showFooterHints = true
	m.termWidth = 60
	m = pressSlash(m)
	m.library.SelectedHashes = map[string]struct{}{"h1": {}}
	m.visual = visualState{active: true, view: viewLibrary}
	if got := ansi.Strip(m.renderFooterHint()); !strings.Contains(got, "enter apply") || strings.Contains(got, "extend") {
		t.Fatalf("selection displaced filter hints: %q", got)
	}
}

func TestLibraryEmptyPrefixLiveAndCommitted(t *testing.T) {
	for _, query := range []string{"a:", "artist: ", "a:Alice l:"} {
		m := newRichFixture()
		want := len(m.library.Panel().VisibleTracks())
		if strings.Contains(query, "Alice") {
			want = 3
		}
		m = typeText(pressSlash(m), query)
		if got := len(m.library.Panel().VisibleTracks()); got != want {
			t.Fatalf("live %q: %d tracks, want %d", query, got, want)
		}
		m = pressEnter(m)
		if got := len(m.library.Panel().VisibleTracks()); got != want {
			t.Fatalf("committed %q: %d tracks, want %d", query, got, want)
		}
	}
}
