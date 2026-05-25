package tui

import (
	"testing"
)

// TestBulkMetadataEditFromArtistsTrackLevel verifies that bulkSelectedHashes
// returns hashes from artists.selectedHashes when the Artists view is active.
func TestBulkMetadataEditFromArtistsTrackLevel(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "4")
	m = pressEnter(m)
	m = pressEnter(m)

	m.browse.artists.selectedHashes = map[string]struct{}{"h1": {}, "h2": {}}

	hashes := m.bulkSelectedHashes()
	if len(hashes) != 2 {
		t.Fatalf("bulkSelectedHashes: expected 2, got %d", len(hashes))
	}

	m = pressMKey(m)
	if m.overlay.metadataEditor == nil {
		t.Fatal("expected bulk metadataEditor to open with 2 selected tracks")
	}
	if !m.overlay.metadataEditor.IsBulk() {
		t.Error("expected IsBulk=true for 2 selected tracks")
	}
	if len(m.overlay.metadataEditor.Hashes()) != 2 {
		t.Errorf("expected 2 hashes in editor, got %d", len(m.overlay.metadataEditor.Hashes()))
	}
}

// TestBulkMetadataEditFromAlbumsTrackLevel verifies that bulkSelectedHashes
// returns hashes from albums.selectedHashes when the Albums view is active.
func TestBulkMetadataEditFromAlbumsTrackLevel(t *testing.T) {
	m := newModelWithTracks()
	m = switchToAlbumsMode(m)
	m = pressEnter(m)

	m.browse.albums.selectedHashes = map[string]struct{}{"h1": {}, "h2": {}}

	hashes := m.bulkSelectedHashes()
	if len(hashes) != 2 {
		t.Fatalf("bulkSelectedHashes (albums): expected 2, got %d", len(hashes))
	}

	m = pressMKey(m)
	if m.overlay.metadataEditor == nil {
		t.Fatal("expected bulk metadataEditor to open with 2 selected tracks")
	}
	if !m.overlay.metadataEditor.IsBulk() {
		t.Error("expected IsBulk=true for 2 selected tracks")
	}
}

// TestBulkSelectedHashesEmptyWhenNoSelection verifies that bulkSelectedHashes
// returns nil when artists/albums have no selection.
func TestBulkSelectedHashesEmptyWhenNoSelection(t *testing.T) {
	m := newModelWithTracks()

	m = pressKey(m, "4")
	if hashes := m.bulkSelectedHashes(); hashes != nil {
		t.Errorf("artists with no selection: expected nil, got %v", hashes)
	}

	m = switchToAlbumsMode(m)
	if hashes := m.bulkSelectedHashes(); hashes != nil {
		t.Errorf("albums with no selection: expected nil, got %v", hashes)
	}
}

// TestBulkMetadataEditFromRecentlyAdded verifies that pressing M with
// multiple selected tracks in the Recently Added view opens the bulk
// metadata editor.
func TestBulkMetadataEditFromRecentlyAdded(t *testing.T) {
	m := newModelWithTracks()
	m.library.mode = LibraryModeRecent
	m.activeView = viewLibrary
	m.library.selectedHashes = map[string]struct{}{"h1": {}, "h2": {}}

	m = pressMKey(m)
	if m.overlay.metadataEditor == nil {
		t.Fatal("expected bulk metadataEditor to open with 2 selected tracks in Recently Added")
	}
	if !m.overlay.metadataEditor.IsBulk() {
		t.Error("expected IsBulk=true for 2 selected tracks")
	}
	if len(m.overlay.metadataEditor.Hashes()) != 2 {
		t.Errorf("expected 2 hashes in editor, got %d", len(m.overlay.metadataEditor.Hashes()))
	}
}
