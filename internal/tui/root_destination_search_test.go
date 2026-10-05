package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/tui/playlists"
)

func TestDestinationSearchReceivesPasteWithoutFilteringUnderlyingView(t *testing.T) {
	m := newRichFixture()
	m = pressEnter(typeText(pressSlash(m), "Alice"))
	picker := playlists.NewDestinationPicker([]db.PlaylistRow{{Name: "AC/DC"}, {Name: "Jazz"}})
	m.overlay.destPicker = &picker
	m = pressSlash(m)
	next, _ := m.Update(tea.PasteMsg{Content: "AC/DC"})
	m = next.(Model)
	if m.library.Filter.Text != "Alice" {
		t.Fatalf("paste reached underlying view: %q", m.library.Filter.Text)
	}
	out := m.overlay.destPicker.Render(60, 15, *m.styles)
	if !strings.Contains(out, "AC/DC") || strings.Contains(out, "Jazz") {
		t.Fatalf("paste did not filter picker:\n%s", out)
	}
	m = pressEnter(m)
	_, cmd, consumed := m.handleDestPickerKeyPress(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !consumed || cmd == nil {
		t.Fatal("filtered destination not confirmed")
	}
	if got := cmd().(playlists.DestinationPickedMsg); got.PlaylistName != "AC/DC" {
		t.Fatalf("wrong destination: %+v", got)
	}
}

func TestLiteralSlashInViewFilters(t *testing.T) {
	for _, view := range []viewID{viewLibrary, viewQueue, viewPlaylists, viewBrowse, viewFavorites, viewTags} {
		m := newRichFixture()
		m.ActiveView = view
		m = typeText(pressSlash(m), "AC/DC")
		if m.activeFilterOps() == nil {
			t.Fatalf("view %v: slash committed the filter", view)
		}
		m = pressEnter(m)
		if m.activeFilterOps() != nil {
			t.Fatalf("view %v: Enter did not commit filter", view)
		}
		if len(m.filterHistory) == 0 || m.filterHistory[len(m.filterHistory)-1] != "AC/DC" {
			t.Fatalf("view %v: committed query = %v", view, m.filterHistory)
		}
	}
}
