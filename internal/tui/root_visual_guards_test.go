package tui

import (
	"context"
	"testing"
)

func TestVisualModeHierarchicalViewGuard(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewBrowse
	m.browse.mode = BrowseModeArtists

	m = doVisual(m)

	if m.visual.active {
		t.Fatal("expected visual mode to be blocked at artist list level")
	}
}

func TestVisualModeMixesGuardListLevel(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewMixes
	m.mixes.level = 0

	m = doVisual(m)

	if m.visual.active {
		t.Fatal("expected visual mode to be blocked at mix list level")
	}
}

func TestVisualModeMixesGuardTrackLevel(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewMixes
	m.mixes.level = 1
	m.mixes.trackView = LibraryView{sourceTracks: makeNTracks(3)}

	m = doVisual(m)

	if m.visual.active {
		t.Fatal("expected visual mode to be blocked at mix track level until full wiring lands")
	}
}
