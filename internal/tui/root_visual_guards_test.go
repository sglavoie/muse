package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/tui/browse"
	"github.com/sglavoie/muse/internal/tui/keys"
	libtui "github.com/sglavoie/muse/internal/tui/library"
)

func TestVisualModeHierarchicalViewGuard(t *testing.T) {
	m := New(context.Background())
	m.ActiveView = viewBrowse
	m.browse.Mode = browse.BrowseModeArtists

	m = doVisual(m)

	if m.visual.active {
		t.Fatal("expected visual mode to be blocked at artist list level")
	}
}

func TestVisualModeMixesGuardListLevel(t *testing.T) {
	m := New(context.Background())
	m.ActiveView = viewMixes
	m.mixes.Level = 0

	m = doVisual(m)

	if m.visual.active {
		t.Fatal("expected visual mode to be blocked at mix list level")
	}
}

func TestVisualModeMixesTrackLevel(t *testing.T) {
	m := New(context.Background())
	m.ActiveView = viewMixes
	m.mixes.Level = 1
	m.mixes.TrackView = libtui.View{SourceTracks: makeNTracks(3)}

	m = doVisual(m)

	if !m.visual.active || m.activeViewSelectionCount() != 1 {
		t.Fatal("expected the focused mix track to start a visual range")
	}
	m, _, _ = m.handleNavigationAction(keys.ActionCursorDown)
	if m.activeViewSelectionCount() != 2 {
		t.Fatal("range did not grow")
	}
	m, _, _ = m.handleNavigationAction(keys.ActionCursorUp)
	if m.activeViewSelectionCount() != 1 {
		t.Fatal("range did not shrink")
	}
	m, _, _ = m.handleNavigationAction(keys.ActionClearSelection)
	if m.visual.active || m.activeViewSelectionCount() != 0 {
		t.Fatal("range did not clear")
	}
}

func TestMixVisualEscapeKeepsTrackListThenReturns(t *testing.T) {
	m := setupMixesAtTrackLevel()
	m = pressKey(m, "v")
	m = pressKey(m, "j")
	m = pressEsc(m)
	if m.visual.active || m.mixes.Level != 1 || m.activeViewSelectionCount() != 2 {
		t.Fatal("escape should leave the range and retain the selected tracks")
	}
	m = pressEsc(m)
	if m.mixes.Level != 0 || m.visual.active {
		t.Fatal("second escape should return to mixes")
	}
}
