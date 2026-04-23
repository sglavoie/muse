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

	if m.artists.level != arttui.TrackLevel {
		t.Fatalf("expected TrackLevel, got %v", m.artists.level)
	}
	// Cursor at 0: Track A1 (hash h1).
	m = pressKey(m, "x")

	if len(m.artists.selectedHashes) != 1 {
		t.Fatalf("after first x: expected 1 selected hash, got %d", len(m.artists.selectedHashes))
	}
	if _, ok := m.artists.selectedHashes["h1"]; !ok {
		t.Errorf("expected hash h1 selected, got: %v", m.artists.selectedHashes)
	}

	// Press x again: deselects.
	m = pressKey(m, "x")
	if len(m.artists.selectedHashes) != 0 {
		t.Errorf("after second x: expected 0 selected hashes, got %d", len(m.artists.selectedHashes))
	}
}

// TestBulkSelectionToggleArtistsNoOpAtArtistLevel verifies that pressing x at
// ArtistLevel (not TrackLevel) does not add any hash to selectedHashes.
func TestBulkSelectionToggleArtistsNoOpAtArtistLevel(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "4") // Artists view, ArtistLevel

	if m.artists.level != arttui.ArtistLevel {
		t.Fatalf("expected ArtistLevel, got %v", m.artists.level)
	}

	m = pressKey(m, "x")
	if len(m.artists.selectedHashes) != 0 {
		t.Errorf("expected no selection at ArtistLevel, got %d", len(m.artists.selectedHashes))
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
	if len(m.artists.selectedHashes) == 0 {
		t.Fatal("precondition: expected selection after x")
	}

	// Clear selection.
	m = pressKey(m, "X")
	if len(m.artists.selectedHashes) != 0 {
		t.Errorf("after X: expected 0 selected hashes, got %d", len(m.artists.selectedHashes))
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
	if len(m.artists.selectedHashes) == 0 {
		t.Fatal("precondition: expected selection after x")
	}

	m = pressEsc(m) // back to AlbumLevel
	if len(m.artists.selectedHashes) != 0 {
		t.Errorf("after back: expected selectedHashes cleared, got %d", len(m.artists.selectedHashes))
	}
}

// TestBulkSelectionToggleAlbumsTrackLevel verifies that pressing x at
// TrackLevel in the Albums view adds/removes the focused track hash.
func TestBulkSelectionToggleAlbumsTrackLevel(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "5") // Albums view
	m = pressEnter(m)    // AlbumLevel → TrackLevel (Alpha)

	if m.albums.level != albtui.TrackLevel {
		t.Fatalf("expected TrackLevel, got %v", m.albums.level)
	}

	m = pressKey(m, "x")
	if len(m.albums.selectedHashes) != 1 {
		t.Fatalf("after first x: expected 1 selected hash, got %d", len(m.albums.selectedHashes))
	}

	m = pressKey(m, "x")
	if len(m.albums.selectedHashes) != 0 {
		t.Errorf("after second x: expected 0 selected hashes, got %d", len(m.albums.selectedHashes))
	}
}

// TestBulkSelectionToggleAlbumsNoOpAtAlbumLevel verifies that pressing x at
// AlbumLevel (not TrackLevel) does not add any hash to selectedHashes.
func TestBulkSelectionToggleAlbumsNoOpAtAlbumLevel(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "5") // Albums view, AlbumLevel

	if m.albums.level != albtui.AlbumLevel {
		t.Fatalf("expected AlbumLevel, got %v", m.albums.level)
	}

	m = pressKey(m, "x")
	if len(m.albums.selectedHashes) != 0 {
		t.Errorf("expected no selection at AlbumLevel, got %d", len(m.albums.selectedHashes))
	}
}

// TestClearSelectionAlbums verifies that pressing X clears selectedHashes in
// the Albums view.
func TestClearSelectionAlbums(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "5")
	m = pressEnter(m) // TrackLevel

	m = pressKey(m, "x")
	if len(m.albums.selectedHashes) == 0 {
		t.Fatal("precondition: expected selection after x")
	}

	m = pressKey(m, "X")
	if len(m.albums.selectedHashes) != 0 {
		t.Errorf("after X: expected 0 selected hashes, got %d", len(m.albums.selectedHashes))
	}
}

// TestBulkSelectionClearedOnBackAlbums verifies that going back from TrackLevel
// to AlbumLevel clears selectedHashes in the Albums view.
func TestBulkSelectionClearedOnBackAlbums(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "5")
	m = pressEnter(m) // TrackLevel

	m = pressKey(m, "x")
	if len(m.albums.selectedHashes) == 0 {
		t.Fatal("precondition: expected selection after x")
	}

	m = pressEsc(m) // back to AlbumLevel
	if len(m.albums.selectedHashes) != 0 {
		t.Errorf("after back: expected selectedHashes cleared, got %d", len(m.albums.selectedHashes))
	}
}

// TestVisualModeExitedOnArtistBack verifies that pressing esc at TrackLevel
// in the Artists view exits visual mode.
func TestVisualModeExitedOnArtistBack(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "4") // Artists view
	m = pressEnter(m)    // AlbumLevel
	m = pressEnter(m)    // TrackLevel

	if m.artists.level != arttui.TrackLevel {
		t.Fatalf("expected TrackLevel, got %v", m.artists.level)
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
	m = pressKey(m, "5") // Albums view
	m = pressEnter(m)    // TrackLevel

	if m.albums.level != albtui.TrackLevel {
		t.Fatalf("expected TrackLevel, got %v", m.albums.level)
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

	if m.library.filter.Text != "Alice" {
		t.Fatalf("precondition: expected committed filter, got %q", m.library.filter.Text)
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
	if m.library.filter.Text != "" {
		t.Errorf("expected filter text cleared, got %q", m.library.filter.Text)
	}
}
