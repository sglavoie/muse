package tui

import (
	"testing"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/recommendation"
	"github.com/sglavoie/muse/internal/tui/keys"
)

func makeTestMixes(n int) []MixInfo {
	out := make([]MixInfo, n)
	for i := 0; i < n; i++ {
		out[i] = recommendation.MixInfo{
			ID:         db.MixID(i + 1),
			Name:       "mix",
			TrackCount: 0,
		}
	}
	return out
}

// TestMixesViewHandleCursorClampsToBounds verifies that no cursor action can
// drive selectedMix out of [0, len(mixes)-1], which would otherwise panic in
// render() or focusedMix().
func TestMixesViewHandleCursorClampsToBounds(t *testing.T) {
	v := MixesView{
		mixes:  makeTestMixes(3),
		height: 10,
	}
	actions := []keys.Action{
		keys.ActionCursorUp, keys.ActionCursorUp, keys.ActionCursorUp,
		keys.ActionCursorDown, keys.ActionCursorDown, keys.ActionCursorDown, keys.ActionCursorDown,
		keys.ActionHalfPageUp, keys.ActionHalfPageDown,
		keys.ActionFullPageUp, keys.ActionFullPageDown,
		keys.ActionGoToTop, keys.ActionGoToBottom,
	}
	for _, a := range actions {
		v = v.handleCursorAction(a)
		if v.selectedMix < 0 || v.selectedMix >= len(v.mixes) {
			t.Fatalf("after action %v: selectedMix=%d out of bounds (n=%d)", a, v.selectedMix, len(v.mixes))
		}
	}
}

// TestMixesViewHandleCursorEmptyDoesNotPanic ensures that delivering cursor
// actions to an empty mix list does not panic and does not leave selectedMix in
// an invalid state.
func TestMixesViewHandleCursorEmptyDoesNotPanic(t *testing.T) {
	v := MixesView{}
	for _, a := range []keys.Action{keys.ActionCursorDown, keys.ActionGoToBottom, keys.ActionFullPageDown} {
		v = v.handleCursorAction(a)
	}
	if v.selectedMix != 0 {
		t.Fatalf("expected selectedMix=0 on empty list, got %d", v.selectedMix)
	}
	if got := v.focusedMix(); got != nil {
		t.Fatalf("expected focusedMix=nil on empty list, got %+v", got)
	}
}

// TestMixesViewFocusedMixHonorsLevel ensures focusedMix returns nil whenever the
// view is at track level, even if the mix list is non-empty — preventing
// accidental access to a stale level-0 selection while drilled in.
func TestMixesViewFocusedMixHonorsLevel(t *testing.T) {
	v := MixesView{
		mixes: makeTestMixes(2),
		level: 1,
	}
	if got := v.focusedMix(); got != nil {
		t.Fatalf("expected focusedMix=nil at level 1, got %+v", got)
	}
}

// TestMixesViewOpenMixSeedsTrackView ensures openMix transitions to level 1
// and populates the embedded track view from the cached map, including when
// the cache has no entry for the requested mix (empty slice fallback).
func TestMixesViewOpenMixSeedsTrackView(t *testing.T) {
	tracks := []library.Track{{Hash: "h1"}, {Hash: "h2"}}
	v := MixesView{
		mixes:       makeTestMixes(1),
		width:       80,
		height:      24,
		cachedMixes: map[db.MixID][]library.Track{db.MixID(1): tracks},
	}
	v.openMix(db.MixID(1))
	if v.level != 1 {
		t.Fatalf("expected level=1 after openMix, got %d", v.level)
	}
	if v.activeMix != db.MixID(1) {
		t.Fatalf("expected activeMix=1, got %d", v.activeMix)
	}
	if len(v.trackView.SourceTracks) != 2 {
		t.Fatalf("expected 2 source tracks, got %d", len(v.trackView.SourceTracks))
	}
	if v.trackView.Width != 80 || v.trackView.Height != 24 {
		t.Fatalf("expected trackView to inherit width/height, got %dx%d", v.trackView.Width, v.trackView.Height)
	}

	// Uncached mix: openMix should still transition without panicking.
	v.back()
	v.openMix(db.MixID(99))
	if v.level != 1 {
		t.Fatalf("expected level=1 after openMix with uncached id, got %d", v.level)
	}
	if len(v.trackView.SourceTracks) != 0 {
		t.Fatalf("expected zero source tracks for uncached mix, got %d", len(v.trackView.SourceTracks))
	}
}

// TestMixesViewBackResetsTrackView ensures returning from level 1 clears the
// trackView so a subsequent openMix starts from a clean state (no leaked
// filter/selection from the previous drill-in).
func TestMixesViewBackResetsTrackView(t *testing.T) {
	tracks := []library.Track{{Hash: "h1"}}
	v := MixesView{
		mixes:       makeTestMixes(1),
		cachedMixes: map[db.MixID][]library.Track{db.MixID(1): tracks},
	}
	v.openMix(db.MixID(1))
	if len(v.trackView.SourceTracks) == 0 {
		t.Fatalf("setup: expected non-empty trackView after openMix")
	}
	v.back()
	if v.level != 0 {
		t.Fatalf("expected level=0 after back, got %d", v.level)
	}
	if len(v.trackView.SourceTracks) != 0 {
		t.Fatalf("expected trackView reset after back, got %d tracks", len(v.trackView.SourceTracks))
	}
}
