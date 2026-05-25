package tui

import "testing"

func TestVisualModeEnterSelectsAnchor(t *testing.T) {
	m := newLibraryModel(5)
	m.library.cursor = 2

	m = doVisual(m)

	if !m.visual.active {
		t.Fatal("expected visual mode to be active")
	}
	if m.visual.anchor != 2 {
		t.Errorf("anchor: expected 2, got %d", m.visual.anchor)
	}
	assertSelectedHashesExact(t, m.library.selectedHashes, "h2")
}

func TestVisualModeExtendDownward(t *testing.T) {
	m := newLibraryModel(6)
	m.library.cursor = 0

	m = doVisual(m)
	m = doNav(m, actionCursorDown)
	m = doNav(m, actionCursorDown)
	m = doNav(m, actionCursorDown)

	if m.library.cursor != 3 {
		t.Fatalf("expected cursor at 3, got %d", m.library.cursor)
	}
	assertSelectedHashesExact(t, m.library.selectedHashes, "h0", "h1", "h2", "h3")
}

func TestVisualModeRangeShrinks(t *testing.T) {
	m := newLibraryModel(6)
	m.library.cursor = 0

	m = doVisual(m)
	m = doNav(m, actionCursorDown)
	m = doNav(m, actionCursorDown)
	m = doNav(m, actionCursorDown)
	m = doNav(m, actionCursorUp)

	assertSelectedHashesExact(t, m.library.selectedHashes, "h0", "h1", "h2")
}

func TestVisualModeExitPreservesSelections(t *testing.T) {
	m := newLibraryModel(5)
	m.library.cursor = 0

	m = doVisual(m)
	m = doNav(m, actionCursorDown)
	m = doNav(m, actionCursorDown)
	m = doVisual(m)

	if m.visual.active {
		t.Fatal("expected visual mode to be inactive after second v")
	}
	assertSelectedHashesExact(t, m.library.selectedHashes, "h0", "h1", "h2")
}

func TestVisualModeClearSelectionExitsMode(t *testing.T) {
	m := newLibraryModel(5)
	m.library.cursor = 0

	m = doVisual(m)
	m = doNav(m, actionCursorDown)
	m, _, _ = m.handleNavigationAction(actionClearSelection)

	if m.visual.active {
		t.Fatal("expected visual mode to exit on X")
	}
	if len(m.library.selectedHashes) != 0 {
		t.Errorf("expected 0 selected after X, got %d", len(m.library.selectedHashes))
	}
}

func TestVisualModeViewSwitchExits(t *testing.T) {
	m := newLibraryModel(5)
	m.library.cursor = 0

	m = doVisual(m)
	m = doNav(m, actionCursorDown)
	m = m.switchToView(viewQueue)

	if m.visual.active {
		t.Fatal("expected visual mode to exit on view switch")
	}
	assertSelectedHashesExact(t, m.library.selectedHashes, "h0", "h1")
}

func TestVisualModePreexistingSelectionsPreserved(t *testing.T) {
	m := newLibraryModel(6)
	m.library.selectedHashes = map[string]struct{}{"h5": {}}
	m.library.cursor = 1

	m = doVisual(m)
	m = doNav(m, actionCursorDown)

	assertSelectedHashesExact(t, m.library.selectedHashes, "h1", "h2", "h5")
}

func TestVisualModeAnchorClamp(t *testing.T) {
	m := newLibraryModel(5)
	m.library.cursor = 4

	m = doVisual(m)

	if m.visual.anchor != 4 {
		t.Fatalf("expected anchor 4, got %d", m.visual.anchor)
	}

	m.library.sourceTracks = makeNTracks(3)
	m.library.cursor = 2
	m = doNav(m, actionCursorUp)

	if m.visual.anchor > 2 {
		t.Errorf("anchor should be clamped to ≤2 after list shrink, got %d", m.visual.anchor)
	}
}

func TestVisualModeFilterActivationExits(t *testing.T) {
	m := newLibraryModel(5)
	m.library.cursor = 0

	m = doVisual(m)
	if !m.visual.active {
		t.Fatal("expected visual mode to be active before filter")
	}

	m, _ = m.handleFilterBarMsg()

	if m.visual.active {
		t.Fatal("expected visual mode to exit when filter bar is activated")
	}
}
