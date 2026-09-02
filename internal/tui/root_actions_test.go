package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/keys"
)

// TestActionGoToWithEmptyOptionsNotifiesUser verifies that pressing the Go To
// key on a focused track with no jump targets (e.g. empty artist/album/genre)
// produces a status-bar notification instead of silently doing nothing.
// Regression for a UX bug where the keypress was swallowed without feedback.
func TestActionGoToWithEmptyOptionsNotifiesUser(t *testing.T) {
	// Track with all empty metadata: artist, album, genre. Library FocusedTrack
	// returns it (found=true), but buildGoToOptions yields an empty slice
	// because addArtist/addAlbum/addGenres only append when the value is set.
	tracks := []library.Track{{Title: "Solo", Hash: "h1", Path: "/x.flac"}}
	m := New(context.Background(), WithLibraryTracks(tracks))

	if m.activeView != viewLibrary {
		t.Fatalf("precondition: expected viewLibrary, got %d", m.activeView)
	}
	// Confirm precondition: ctx is found and options is empty.
	ctx := m.focusedGoToContext()
	if !ctx.found {
		t.Fatal("precondition: expected focusedGoToContext to report found=true")
	}
	if opts := m.buildGoToOptions(ctx); len(opts) != 0 {
		t.Fatalf("precondition: expected zero jump options, got %d", len(opts))
	}

	nextModel, _ := m.handleKeyAction(keys.ActionGoTo)
	nm := nextModel.(Model)

	if nm.overlay.goTo != nil {
		t.Fatal("expected overlay.goTo to remain nil when no jump targets apply")
	}
	if msg := nm.statusBar.TopNotification().Msg; !strings.Contains(msg, "No jump targets") {
		t.Fatalf("expected status notification mentioning 'No jump targets', got %q", msg)
	}
}

// TestActionGoToNotFoundNotifies confirms the existing "No track focused" path
// still works (i.e., the new empty-options branch did not regress it).
func TestActionGoToNotFoundNotifies(t *testing.T) {
	m := New(context.Background()) // empty library, library FocusedTrack returns !ok

	nextModel, _ := m.handleKeyAction(keys.ActionGoTo)
	nm := nextModel.(Model)

	if nm.overlay.goTo != nil {
		t.Fatal("expected overlay.goTo to remain nil when no track focused")
	}
	if msg := nm.statusBar.TopNotification().Msg; !strings.Contains(msg, "No track focused") {
		t.Fatalf("expected status notification 'No track focused', got %q", msg)
	}
}

// TestActionHelpHandlesZeroTermHeight verifies that opening the help overlay
// with an uninitialized terminal height (0) does not produce a viewport with
// a negative height. Regression for inconsistent height clamping that diverged
// from the actionDebug branch.
func TestActionHelpHandlesZeroTermHeight(t *testing.T) {
	m := New(context.Background())
	m.termWidth = 80
	m.termHeight = 0 // simulate WindowSizeMsg not yet received

	nextModel, _ := m.handleKeyAction(keys.ActionHelp)
	nm := nextModel.(Model)

	if !nm.helpVisible {
		t.Fatal("expected helpVisible to be true after actionHelp")
	}
	if h := nm.helpViewport.Height(); h < 0 {
		t.Fatalf("helpViewport height should be clamped to >= 0, got %d", h)
	}
}
