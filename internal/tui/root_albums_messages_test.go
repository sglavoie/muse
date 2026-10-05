package tui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
)

// TestScanBatchRefreshesAlbumList verifies that receiving a ScanBatchMsg
// while at AlbumLevel re-derives the album list.
func TestScanBatchRefreshesAlbumList(t *testing.T) {
	m := New(context.Background(), WithLibraryTracks([]library.Track{{Artist: "Alice", Album: "Alpha", Title: "T1", Hash: "h1"}}))

	// Switch to albums view.
	m = switchToAlbumsMode(m)
	if len(m.browse.Albums.Albums) != 1 {
		t.Fatalf("expected 1 album initially, got %d", len(m.browse.Albums.Albums))
	}

	// Simulate a scan batch adding a new album.
	next, _ := m.Update(library.ScanBatchMsg{Tracks: []library.Track{{Artist: "Bob", Album: "Beta", Title: "T2", Hash: "h2"}}})
	m = next.(Model)

	if len(m.browse.Albums.Albums) != 2 {
		t.Fatalf("expected 2 albums after scan batch, got %d", len(m.browse.Albums.Albums))
	}
}

// TestWatchBatchRefreshesAlbumList verifies that receiving a watchBatchMsg
// while at AlbumLevel re-derives the album list.
func TestWatchBatchRefreshesAlbumList(t *testing.T) {
	m := New(context.Background(), WithLibraryTracks([]library.Track{
		{Artist: "Alice", Album: "Alpha", Title: "T1", Hash: "h1", Path: "/a.flac"},
		{Artist: "Bob", Album: "Beta", Title: "T2", Hash: "h2", Path: "/b.flac"},
	}))

	// Switch to albums view.
	m = switchToAlbumsMode(m)
	if len(m.browse.Albums.Albums) != 2 {
		t.Fatalf("expected 2 albums initially, got %d", len(m.browse.Albums.Albums))
	}

	// Simulate the watch deleting Bob's track.
	next, _ := m.Update(watchBatchMsg{deleted: []string{"/b.flac"}})
	m = next.(Model)

	if len(m.browse.Albums.Albums) != 1 {
		t.Fatalf("expected 1 album after watch batch, got %d", len(m.browse.Albums.Albums))
	}
	if library.AlbumTitle(m.browse.Albums.Albums[0]) != "Alpha" {
		t.Errorf("expected remaining album='Alpha', got %q", m.browse.Albums.Albums[0])
	}
}

// TestAlbumsClampCursorAfterShrink verifies that the cursor is clamped when
// the album list shrinks during a library update.
func TestAlbumsClampCursorAfterShrink(t *testing.T) {
	m := New(context.Background(), WithLibraryTracks([]library.Track{
		{Artist: "Alice", Album: "Alpha", Title: "T1", Hash: "h1", Path: "/a.flac"},
		{Artist: "Bob", Album: "Beta", Title: "T2", Hash: "h2", Path: "/b.flac"},
		{Artist: "Charlie", Album: "Gamma", Title: "T3", Hash: "h3", Path: "/c.flac"},
	}))

	// Switch to albums view and move cursor to Gamma (index 2).
	m = switchToAlbumsMode(m)
	m = pressKey(m, "j")
	m = pressKey(m, "j")
	if m.browse.Albums.Cursor != 2 {
		t.Fatalf("expected cursor=2, got %d", m.browse.Albums.Cursor)
	}

	// Delete Charlie — cursor should clamp to index 1.
	next, _ := m.Update(watchBatchMsg{deleted: []string{"/c.flac"}})
	m = next.(Model)

	if m.browse.Albums.Cursor != 1 {
		t.Errorf("expected cursor clamped to 1, got %d", m.browse.Albums.Cursor)
	}
}

// TestRescanClearsAlbumsView verifies that actionRescan resets the albums
// view state so it gets re-derived from the new library.
func TestRescanClearsAlbumsView(t *testing.T) {
	m := newModelWithTracks()
	m = switchToAlbumsMode(m)
	if len(m.browse.Albums.Albums) != 4 {
		t.Fatalf("expected 4 albums, got %d", len(m.browse.Albums.Albums))
	}

	// Simulate rescan: set libraryRoot and extractor to enable rescan.
	m.libraryRoots = []string{"/tmp/music"}
	m.extractor = stubExtractor{}

	next, _ := m.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	m = next.(Model)

	if len(m.browse.Albums.Albums) != 0 {
		t.Errorf("expected empty album list after rescan, got %d", len(m.browse.Albums.Albums))
	}
	if m.browse.Albums.Level != albtui.AlbumLevel {
		t.Errorf("expected AlbumLevel after rescan, got %v", m.browse.Albums.Level)
	}
}
