package tui

import (
	"testing"

	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
)

// TestBulkSelectionToggleArtistsTrackLevel verifies that pressing x at
// TrackLevel in the Artists view adds the focused track hash to selectedHashes,
// and pressing x again removes it (toggle behaviour).
func TestBulkSelectionToggleArtistsTrackLevel(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "4") // Artists view
	m = pressEnter(m)    // ArtistLevel → AlbumLevel (Alice)
	m = pressEnter(m)    // AlbumLevel → TrackLevel (Alpha)

	if m.browse.Artists.Level != arttui.TrackLevel {
		t.Fatalf("expected TrackLevel, got %v", m.browse.Artists.Level)
	}
	// Cursor at 0: Track A1 (hash h1).
	m = pressKey(m, "x")

	if len(m.browse.Artists.SelectedHashes) != 1 {
		t.Fatalf("after first x: expected 1 selected hash, got %d", len(m.browse.Artists.SelectedHashes))
	}
	if _, ok := m.browse.Artists.SelectedHashes["h1"]; !ok {
		t.Errorf("expected hash h1 selected, got: %v", m.browse.Artists.SelectedHashes)
	}

	// Press x again: deselects.
	m = pressKey(m, "x")
	if len(m.browse.Artists.SelectedHashes) != 0 {
		t.Errorf("after second x: expected 0 selected hashes, got %d", len(m.browse.Artists.SelectedHashes))
	}
}

// TestBulkSelectionToggleArtistsNoOpAtArtistLevel verifies that pressing x at
// ArtistLevel (not TrackLevel) does not add any hash to selectedHashes.
func TestBulkSelectionToggleArtistsNoOpAtArtistLevel(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "4") // Artists view, ArtistLevel

	if m.browse.Artists.Level != arttui.ArtistLevel {
		t.Fatalf("expected ArtistLevel, got %v", m.browse.Artists.Level)
	}

	m = pressKey(m, "x")
	if len(m.browse.Artists.SelectedHashes) != 0 {
		t.Errorf("expected no selection at ArtistLevel, got %d", len(m.browse.Artists.SelectedHashes))
	}
}

// TestClearSelectionArtists verifies that pressing X clears selectedHashes in
// the Artists view.
func TestClearSelectionArtists(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "4")
	m = pressEnter(m) // AlbumLevel
	m = pressEnter(m) // TrackLevel

	// Select a track.
	m = pressKey(m, "x")
	if len(m.browse.Artists.SelectedHashes) == 0 {
		t.Fatal("precondition: expected selection after x")
	}

	// Clear selection.
	m = pressKey(m, "X")
	if len(m.browse.Artists.SelectedHashes) != 0 {
		t.Errorf("after X: expected 0 selected hashes, got %d", len(m.browse.Artists.SelectedHashes))
	}
}

// TestBulkSelectionClearedOnBackArtists verifies that going back from
// TrackLevel to AlbumLevel clears selectedHashes.
func TestBulkSelectionClearedOnBackArtists(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "4")
	m = pressEnter(m) // AlbumLevel
	m = pressEnter(m) // TrackLevel

	m = pressKey(m, "x")
	if len(m.browse.Artists.SelectedHashes) == 0 {
		t.Fatal("precondition: expected selection after x")
	}

	m = pressEsc(m) // back to AlbumLevel
	if len(m.browse.Artists.SelectedHashes) != 0 {
		t.Errorf("after back: expected selectedHashes cleared, got %d", len(m.browse.Artists.SelectedHashes))
	}
}

// TestBulkSelectionToggleAlbumsTrackLevel verifies that pressing x at
// TrackLevel in the Albums view adds/removes the focused track hash.
func TestBulkSelectionToggleAlbumsTrackLevel(t *testing.T) {
	m := newModelWithTracks()
	m = switchToAlbumsMode(m) // Albums view
	m = pressEnter(m)         // AlbumLevel → TrackLevel (Alpha)

	if m.browse.Albums.Level != albtui.TrackLevel {
		t.Fatalf("expected TrackLevel, got %v", m.browse.Albums.Level)
	}

	m = pressKey(m, "x")
	if len(m.browse.Albums.SelectedHashes) != 1 {
		t.Fatalf("after first x: expected 1 selected hash, got %d", len(m.browse.Albums.SelectedHashes))
	}

	m = pressKey(m, "x")
	if len(m.browse.Albums.SelectedHashes) != 0 {
		t.Errorf("after second x: expected 0 selected hashes, got %d", len(m.browse.Albums.SelectedHashes))
	}
}

// TestBulkSelectionToggleAlbumsNoOpAtAlbumLevel verifies that pressing x at
// AlbumLevel (not TrackLevel) does not add any hash to selectedHashes.
func TestBulkSelectionToggleAlbumsNoOpAtAlbumLevel(t *testing.T) {
	m := newModelWithTracks()
	m = switchToAlbumsMode(m) // Albums view, AlbumLevel

	if m.browse.Albums.Level != albtui.AlbumLevel {
		t.Fatalf("expected AlbumLevel, got %v", m.browse.Albums.Level)
	}

	m = pressKey(m, "x")
	if len(m.browse.Albums.SelectedHashes) != 0 {
		t.Errorf("expected no selection at AlbumLevel, got %d", len(m.browse.Albums.SelectedHashes))
	}
}

// TestClearSelectionAlbums verifies that pressing X clears selectedHashes in
// the Albums view.
func TestClearSelectionAlbums(t *testing.T) {
	m := newModelWithTracks()
	m = switchToAlbumsMode(m)
	m = pressEnter(m) // TrackLevel

	m = pressKey(m, "x")
	if len(m.browse.Albums.SelectedHashes) == 0 {
		t.Fatal("precondition: expected selection after x")
	}

	m = pressKey(m, "X")
	if len(m.browse.Albums.SelectedHashes) != 0 {
		t.Errorf("after X: expected 0 selected hashes, got %d", len(m.browse.Albums.SelectedHashes))
	}
}

// TestBulkSelectionClearedOnBackAlbums verifies that going back from TrackLevel
// to AlbumLevel clears selectedHashes in the Albums view.
func TestBulkSelectionClearedOnBackAlbums(t *testing.T) {
	m := newModelWithTracks()
	m = switchToAlbumsMode(m)
	m = pressEnter(m) // TrackLevel

	m = pressKey(m, "x")
	if len(m.browse.Albums.SelectedHashes) == 0 {
		t.Fatal("precondition: expected selection after x")
	}

	m = pressEsc(m) // back to AlbumLevel
	if len(m.browse.Albums.SelectedHashes) != 0 {
		t.Errorf("after back: expected selectedHashes cleared, got %d", len(m.browse.Albums.SelectedHashes))
	}
}

// TestVisualModeExitedOnArtistBack verifies that pressing esc at TrackLevel
// in the Artists view exits visual mode.
func TestVisualModeExitedOnArtistBack(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "4") // Artists view
	m = pressEnter(m)    // AlbumLevel
	m = pressEnter(m)    // TrackLevel

	if m.browse.Artists.Level != arttui.TrackLevel {
		t.Fatalf("expected TrackLevel, got %v", m.browse.Artists.Level)
	}

	m = pressKey(m, "v") // enter visual mode
	if !m.visual.active {
		t.Fatal("precondition: expected visual mode active")
	}

	m = pressEsc(m) // back to AlbumLevel
	if m.visual.active {
		t.Error("expected visual mode exited on artist back")
	}
}

// TestVisualModeExitedOnAlbumBack verifies that pressing esc at TrackLevel
// in the Albums view exits visual mode.
func TestVisualModeExitedOnAlbumBack(t *testing.T) {
	m := newModelWithTracks()
	m = switchToAlbumsMode(m) // Albums view
	m = pressEnter(m)         // TrackLevel

	if m.browse.Albums.Level != albtui.TrackLevel {
		t.Fatalf("expected TrackLevel, got %v", m.browse.Albums.Level)
	}

	m = pressKey(m, "v")
	if !m.visual.active {
		t.Fatal("precondition: expected visual mode active")
	}

	m = pressEsc(m)
	if m.visual.active {
		t.Error("expected visual mode exited on album back")
	}
}

// TestVisualModeExitedOnFilterClear verifies that clearing a committed filter
// with esc in the library view exits visual mode.
func TestVisualModeExitedOnFilterClear(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "1") // Library view

	// Activate filter, type text, commit.
	m = pressSlash(m)
	m = typeText(m, "Alice")
	m = pressEnter(m) // commit filter

	if m.library.Filter.Text != "Alice" {
		t.Fatalf("precondition: expected committed filter, got %q", m.library.Filter.Text)
	}

	// Enter visual mode on the filtered list.
	m = pressKey(m, "v")
	if !m.visual.active {
		t.Fatal("precondition: expected visual mode active")
	}

	// Press esc to clear the committed filter.
	m = pressEsc(m)
	if m.visual.active {
		t.Error("expected visual mode exited on filter clear")
	}
	if m.library.Filter.Text != "" {
		t.Errorf("expected filter text cleared, got %q", m.library.Filter.Text)
	}
}
