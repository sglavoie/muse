package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
)

func TestArtistsFilterActivateAndType(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "4")

	m = pressSlash(m)
	if !m.artists.filter.Active {
		t.Fatal("expected filter to be active after /")
	}

	m = typeText(m, "bob")
	if m.artists.filter.Text != "bob" {
		t.Fatalf("expected filterText='bob', got %q", m.artists.filter.Text)
	}

	artist, ok := m.artists.focusedArtist()
	if !ok || artist != "Bob" {
		t.Errorf("expected focused artist='Bob', got %q (ok=%v)", artist, ok)
	}
}

func TestArtistsFilterEscClears(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "4")
	m = pressSlash(m)
	m = typeText(m, "ali")

	m = pressEsc(m)
	if m.artists.filter.Active {
		t.Error("expected filter deactivated after Esc")
	}
	if m.artists.filter.Text != "" {
		t.Errorf("expected filterText cleared, got %q", m.artists.filter.Text)
	}
	if len(arttui.FilterStrings(m.artists.artists, m.artists.filter.Text)) != 3 {
		t.Error("expected full artist list after filter cleared")
	}
}

func TestArtistsFilterEnterCommits(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "4")
	m = pressSlash(m)
	m = typeText(m, "ali")

	m = pressEnter(m)
	if m.artists.filter.Active {
		t.Error("expected filter not active after Enter")
	}
	if m.artists.filter.Text != "ali" {
		t.Errorf("expected filterText='ali' after commit, got %q", m.artists.filter.Text)
	}
}

func TestArtistsFilterDrillDownClearsFilter(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "4")
	m = pressSlash(m)
	m = typeText(m, "ali")
	m = pressEnter(m)
	if m.artists.filter.Text != "ali" {
		t.Fatalf("expected filterText='ali', got %q", m.artists.filter.Text)
	}

	m = pressEnter(m)
	if m.artists.level != arttui.AlbumLevel {
		t.Fatalf("expected AlbumLevel, got %v", m.artists.level)
	}
	if m.artists.selectedArtist != "Alice" {
		t.Errorf("expected selectedArtist='Alice', got %q", m.artists.selectedArtist)
	}
	if m.artists.filter.Text != "" {
		t.Errorf("expected filterText cleared after drill-down, got %q", m.artists.filter.Text)
	}
}

func TestArtistsFilterBackClearsFilter(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "4")
	m = pressEnter(m)

	m = pressSlash(m)
	m = typeText(m, "al")
	m = pressEnter(m)

	m = pressEsc(m)
	if m.artists.filter.Text != "" {
		t.Errorf("expected filterText cleared after back, got %q", m.artists.filter.Text)
	}
}

func TestArtistsFilterRenderOutput(t *testing.T) {
	m := newModelWithTracks()
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)
	m = pressKey(m, "4")
	m = pressSlash(m)
	m = typeText(m, "bob")

	v := m.View()
	if !strings.Contains(v.Content, "Filter: ") {
		t.Errorf("expected filter bar in view output, got:\n%s", v.Content)
	}
	if !strings.Contains(v.Content, "Bob") {
		t.Error("expected 'Bob' in filtered view output")
	}
}

func TestArtistsFilterBackspace(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "4")
	m = pressSlash(m)
	m = typeText(m, "bob")

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	m = next.(Model)
	if m.artists.filter.Text != "bo" {
		t.Errorf("expected filterText='bo' after backspace, got %q", m.artists.filter.Text)
	}
}
