package tui

import (
	"testing"

	"github.com/sglavoie/muse/internal/tui/keys"
)

func TestVisualModeEnterSelectsAnchor(t *testing.T) {
	m := newLibraryModel(5)
	m.library.Cursor = 2

	m = doVisual(m)

	if !m.visual.active {
		t.Fatal("expected visual mode to be active")
	}
	if m.visual.anchor != 2 {
		t.Errorf("anchor: expected 2, got %d", m.visual.anchor)
	}
	assertSelectedHashesExact(t, m.library.SelectedHashes, "h2")
}

func TestVisualModeExtendDownward(t *testing.T) {
	m := newLibraryModel(6)
	m.library.Cursor = 0

	m = doVisual(m)
	m = doNav(m, keys.ActionCursorDown)
	m = doNav(m, keys.ActionCursorDown)
	m = doNav(m, keys.ActionCursorDown)

	if m.library.Cursor != 3 {
		t.Fatalf("expected cursor at 3, got %d", m.library.Cursor)
	}
	assertSelectedHashesExact(t, m.library.SelectedHashes, "h0", "h1", "h2", "h3")
}

func TestVisualModeRangeShrinks(t *testing.T) {
	m := newLibraryModel(6)
	m.library.Cursor = 0

	m = doVisual(m)
	m = doNav(m, keys.ActionCursorDown)
	m = doNav(m, keys.ActionCursorDown)
	m = doNav(m, keys.ActionCursorDown)
	m = doNav(m, keys.ActionCursorUp)

	assertSelectedHashesExact(t, m.library.SelectedHashes, "h0", "h1", "h2")
}

func TestVisualModeExitPreservesSelections(t *testing.T) {
	m := newLibraryModel(5)
	m.library.Cursor = 0

	m = doVisual(m)
	m = doNav(m, keys.ActionCursorDown)
	m = doNav(m, keys.ActionCursorDown)
	m = doVisual(m)

	if m.visual.active {
		t.Fatal("expected visual mode to be inactive after second v")
	}
	assertSelectedHashesExact(t, m.library.SelectedHashes, "h0", "h1", "h2")
}

func TestVisualModeClearSelectionExitsMode(t *testing.T) {
	m := newLibraryModel(5)
	m.library.Cursor = 0

	m = doVisual(m)
	m = doNav(m, keys.ActionCursorDown)
	m, _, _ = m.handleNavigationAction(keys.ActionClearSelection)

	if m.visual.active {
		t.Fatal("expected visual mode to exit on X")
	}
	if len(m.library.SelectedHashes) != 0 {
		t.Errorf("expected 0 selected after X, got %d", len(m.library.SelectedHashes))
	}
}

func TestVisualModeViewSwitchExits(t *testing.T) {
	m := newLibraryModel(5)
	m.library.Cursor = 0

	m = doVisual(m)
	m = doNav(m, keys.ActionCursorDown)
	m = m.switchToView(viewQueue)

	if m.visual.active {
		t.Fatal("expected visual mode to exit on view switch")
	}
	assertSelectedHashesExact(t, m.library.SelectedHashes, "h0", "h1")
}

func TestVisualModePreexistingSelectionsPreserved(t *testing.T) {
	m := newLibraryModel(6)
	m.library.SelectedHashes = map[string]struct{}{"h5": {}}
	m.library.Cursor = 1

	m = doVisual(m)
	m = doNav(m, keys.ActionCursorDown)

	assertSelectedHashesExact(t, m.library.SelectedHashes, "h1", "h2", "h5")
}

func TestVisualModeAnchorClamp(t *testing.T) {
	m := newLibraryModel(5)
	m.library.Cursor = 4

	m = doVisual(m)

	if m.visual.anchor != 4 {
		t.Fatalf("expected anchor 4, got %d", m.visual.anchor)
	}

	m.library.SourceTracks = makeNTracks(3)
	m.library.Cursor = 2
	m = doNav(m, keys.ActionCursorUp)

	if m.visual.anchor > 2 {
		t.Errorf("anchor should be clamped to ≤2 after list shrink, got %d", m.visual.anchor)
	}
}

func TestVisualModeFilterActivationExits(t *testing.T) {
	m := newLibraryModel(5)
	m.library.Cursor = 0

	m = doVisual(m)
	if !m.visual.active {
		t.Fatal("expected visual mode to be active before filter")
	}

	m, _ = m.handleFilterBarMsg()

	if m.visual.active {
		t.Fatal("expected visual mode to exit when filter bar is activated")
	}
}
