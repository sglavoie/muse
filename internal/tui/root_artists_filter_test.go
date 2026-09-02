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
	if !m.browse.Artists.Filter.Active {
		t.Fatal("expected filter to be active after /")
	}

	m = typeText(m, "bob")
	if m.browse.Artists.Filter.Text != "bob" {
		t.Fatalf("expected filterText='bob', got %q", m.browse.Artists.Filter.Text)
	}

	artist, ok := m.browse.Artists.FocusedArtist()
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
	if m.browse.Artists.Filter.Active {
		t.Error("expected filter deactivated after Esc")
	}
	if m.browse.Artists.Filter.Text != "" {
		t.Errorf("expected filterText cleared, got %q", m.browse.Artists.Filter.Text)
	}
	if len(arttui.FilterStrings(m.browse.Artists.Artists, m.browse.Artists.Filter.Text)) != 3 {
		t.Error("expected full artist list after filter cleared")
	}
}

func TestArtistsFilterEnterCommits(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "4")
	m = pressSlash(m)
	m = typeText(m, "ali")

	m = pressEnter(m)
	if m.browse.Artists.Filter.Active {
		t.Error("expected filter not active after Enter")
	}
	if m.browse.Artists.Filter.Text != "ali" {
		t.Errorf("expected filterText='ali' after commit, got %q", m.browse.Artists.Filter.Text)
	}
}

func TestArtistsFilterDrillDownClearsFilter(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "4")
	m = pressSlash(m)
	m = typeText(m, "ali")
	m = pressEnter(m)
	if m.browse.Artists.Filter.Text != "ali" {
		t.Fatalf("expected filterText='ali', got %q", m.browse.Artists.Filter.Text)
	}

	m = pressEnter(m)
	if m.browse.Artists.Level != arttui.AlbumLevel {
		t.Fatalf("expected AlbumLevel, got %v", m.browse.Artists.Level)
	}
	if m.browse.Artists.SelectedArtist != "Alice" {
		t.Errorf("expected selectedArtist='Alice', got %q", m.browse.Artists.SelectedArtist)
	}
	if m.browse.Artists.Filter.Text != "" {
		t.Errorf("expected filterText cleared after drill-down, got %q", m.browse.Artists.Filter.Text)
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
	if m.browse.Artists.Filter.Text != "" {
		t.Errorf("expected filterText cleared after back, got %q", m.browse.Artists.Filter.Text)
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
	if !strings.Contains(v.Content, `filter "`) {
		t.Errorf("expected filter chunk in view output, got:\n%s", v.Content)
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
	if m.browse.Artists.Filter.Text != "bo" {
		t.Errorf("expected filterText='bo' after backspace, got %q", m.browse.Artists.Filter.Text)
	}
}
