package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
)

func TestAlbumsFilterActivateAndType(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "5")

	// Activate filter bar.
	m = pressSlash(m)
	if !m.albums.filter.Active {
		t.Fatal("expected filter to be active after /")
	}

	// Type "gam" — should filter to just Gamma.
	m = typeText(m, "gam")
	if m.albums.filter.Text != "gam" {
		t.Fatalf("expected filterText='gam', got %q", m.albums.filter.Text)
	}

	// focusedAlbum should be Gamma.
	album, ok := m.albums.focusedAlbum()
	if !ok || album != "Gamma" {
		t.Errorf("expected focused album='Gamma', got %q (ok=%v)", album, ok)
	}
}

func TestAlbumsFilterEscClears(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "5")
	m = pressSlash(m)
	m = typeText(m, "alp")

	// Esc should deactivate and clear the filter.
	m = pressEsc(m)
	if m.albums.filter.Active {
		t.Error("expected filter deactivated after Esc")
	}
	if m.albums.filter.Text != "" {
		t.Errorf("expected filterText cleared, got %q", m.albums.filter.Text)
	}
	// All 4 albums should be visible again.
	if len(arttui.FilterStrings(m.albums.albums, m.albums.filter.Text)) != 4 {
		t.Error("expected full album list after filter cleared")
	}
}

func TestAlbumsFilterEnterCommits(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "5")
	m = pressSlash(m)
	m = typeText(m, "alp")

	// Enter commits: filter stays but input mode exits.
	m = pressEnter(m)
	if m.albums.filter.Active {
		t.Error("expected filter not active after Enter")
	}
	if m.albums.filter.Text != "alp" {
		t.Errorf("expected filterText='alp' after commit, got %q", m.albums.filter.Text)
	}
}

func TestAlbumsFilterDrillDownClearsFilter(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "5")

	// Filter to "Alp" and commit.
	m = pressSlash(m)
	m = typeText(m, "alp")
	m = pressEnter(m) // commit filter
	if m.albums.filter.Text != "alp" {
		t.Fatalf("expected filterText='alp', got %q", m.albums.filter.Text)
	}

	// Drill down into Alpha's tracks.
	m = pressEnter(m)
	if m.albums.level != albtui.TrackLevel {
		t.Fatalf("expected TrackLevel, got %v", m.albums.level)
	}
	if m.albums.selectedAlbum != "Alpha" {
		t.Errorf("expected selectedAlbum='Alpha', got %q", m.albums.selectedAlbum)
	}
	// Filter should be cleared on drill-down.
	if m.albums.filter.Text != "" {
		t.Errorf("expected filterText cleared after drill-down, got %q", m.albums.filter.Text)
	}
}

func TestAlbumsFilterBackClearsFilter(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "5")
	m = pressEnter(m) // drill down to tracks

	// Filter tracks.
	m = pressSlash(m)
	m = typeText(m, "a1")
	m = pressEnter(m) // commit

	// Go back — filter should be cleared.
	m = pressEsc(m)
	if m.albums.filter.Text != "" {
		t.Errorf("expected filterText cleared after back, got %q", m.albums.filter.Text)
	}
}

func TestAlbumsFilterRenderOutput(t *testing.T) {
	m := newModelWithTracks()
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)
	m = pressKey(m, "5")
	m = pressSlash(m)
	m = typeText(m, "gam")

	v := m.View()
	if !strings.Contains(v.Content, "Filter: ") {
		t.Errorf("expected filter bar in view output, got:\n%s", v.Content)
	}
	if !strings.Contains(v.Content, "Gamma") {
		t.Error("expected 'Gamma' in filtered view output")
	}
}

func TestAlbumsFilterBackspace(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "5")
	m = pressSlash(m)
	m = typeText(m, "gam")

	// Backspace should remove last char.
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	m = next.(Model)
	if m.albums.filter.Text != "ga" {
		t.Errorf("expected filterText='ga' after backspace, got %q", m.albums.filter.Text)
	}
}
