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
	m = switchToAlbumsMode(m)

	// Activate filter bar.
	m = pressSlash(m)
	if !m.browse.Albums.Filter.Active {
		t.Fatal("expected filter to be active after /")
	}

	// Type "gam" — should filter to just Gamma.
	m = typeText(m, "gam")
	if m.browse.Albums.Filter.Text != "gam" {
		t.Fatalf("expected filterText='gam', got %q", m.browse.Albums.Filter.Text)
	}

	// focusedAlbum should be Gamma.
	album, ok := m.browse.Albums.FocusedAlbum()
	if !ok || album != "Gamma" {
		t.Errorf("expected focused album='Gamma', got %q (ok=%v)", album, ok)
	}
}

func TestAlbumsFilterEscClears(t *testing.T) {
	m := newModelWithTracks()
	m = switchToAlbumsMode(m)
	m = pressSlash(m)
	m = typeText(m, "alp")

	// Esc should deactivate and clear the filter.
	m = pressEsc(m)
	if m.browse.Albums.Filter.Active {
		t.Error("expected filter deactivated after Esc")
	}
	if m.browse.Albums.Filter.Text != "" {
		t.Errorf("expected filterText cleared, got %q", m.browse.Albums.Filter.Text)
	}
	// All 4 albums should be visible again.
	if len(arttui.FilterStrings(m.browse.Albums.Albums, m.browse.Albums.Filter.Text)) != 4 {
		t.Error("expected full album list after filter cleared")
	}
}

func TestAlbumsFilterEnterCommits(t *testing.T) {
	m := newModelWithTracks()
	m = switchToAlbumsMode(m)
	m = pressSlash(m)
	m = typeText(m, "alp")

	// Enter commits: filter stays but input mode exits.
	m = pressEnter(m)
	if m.browse.Albums.Filter.Active {
		t.Error("expected filter not active after Enter")
	}
	if m.browse.Albums.Filter.Text != "alp" {
		t.Errorf("expected filterText='alp' after commit, got %q", m.browse.Albums.Filter.Text)
	}
}

func TestAlbumsFilterDrillDownClearsFilter(t *testing.T) {
	m := newModelWithTracks()
	m = switchToAlbumsMode(m)

	// Filter to "Alp" and commit.
	m = pressSlash(m)
	m = typeText(m, "alp")
	m = pressEnter(m) // commit filter
	if m.browse.Albums.Filter.Text != "alp" {
		t.Fatalf("expected filterText='alp', got %q", m.browse.Albums.Filter.Text)
	}

	// Drill down into Alpha's tracks.
	m = pressEnter(m)
	if m.browse.Albums.Level != albtui.TrackLevel {
		t.Fatalf("expected TrackLevel, got %v", m.browse.Albums.Level)
	}
	if m.browse.Albums.SelectedAlbum != "Alpha" {
		t.Errorf("expected selectedAlbum='Alpha', got %q", m.browse.Albums.SelectedAlbum)
	}
	// Filter should be cleared on drill-down.
	if m.browse.Albums.Filter.Text != "" {
		t.Errorf("expected filterText cleared after drill-down, got %q", m.browse.Albums.Filter.Text)
	}
}

func TestAlbumsFilterBackClearsFilter(t *testing.T) {
	m := newModelWithTracks()
	m = switchToAlbumsMode(m)
	m = pressEnter(m) // drill down to tracks

	// Filter tracks.
	m = pressSlash(m)
	m = typeText(m, "a1")
	m = pressEnter(m) // commit

	// Go back — filter should be cleared.
	m = pressEsc(m)
	if m.browse.Albums.Filter.Text != "" {
		t.Errorf("expected filterText cleared after back, got %q", m.browse.Albums.Filter.Text)
	}
}

func TestAlbumsFilterRenderOutput(t *testing.T) {
	m := newModelWithTracks()
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)
	m = switchToAlbumsMode(m)
	m = pressSlash(m)
	m = typeText(m, "gam")

	v := m.View()
	if !strings.Contains(v.Content, `filter "`) {
		t.Errorf("expected filter chunk in view output, got:\n%s", v.Content)
	}
	if !strings.Contains(v.Content, "Gamma") {
		t.Error("expected 'Gamma' in filtered view output")
	}
}

func TestAlbumsFilterBackspace(t *testing.T) {
	m := newModelWithTracks()
	m = switchToAlbumsMode(m)
	m = pressSlash(m)
	m = typeText(m, "gam")

	// Backspace should remove last char.
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	m = next.(Model)
	if m.browse.Albums.Filter.Text != "ga" {
		t.Errorf("expected filterText='ga' after backspace, got %q", m.browse.Albums.Filter.Text)
	}
}
