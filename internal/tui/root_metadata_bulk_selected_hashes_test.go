package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/library"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
)

// TestBulkSelectedHashesFavoritesListLevelTracks verifies that
// bulkSelectedHashes returns hashes for selected tracks at favorites ListLevel.
func TestBulkSelectedHashesFavoritesListLevelTracks(t *testing.T) {
	m := New(context.Background(), WithLibraryTracks(sampleLibraryTracks()))
	m.activeView = viewFavorites
	m.favorites.level = favtui.ListLevel
	m.favorites.category = favtui.CategoryTracks
	m.favorites.tracks = []library.Track{
		{Hash: "h1", Artist: "Alice", Title: "Track A1"},
		{Hash: "h2", Artist: "Alice", Title: "Track A2"},
		{Hash: "h3", Artist: "Alice", Title: "Track B1"},
	}
	m.favorites.selectedItems = map[int]struct{}{0: {}, 2: {}}

	hashes := m.bulkSelectedHashes()
	if len(hashes) != 2 {
		t.Fatalf("expected 2 hashes, got %d", len(hashes))
	}
	hashSet := make(map[string]struct{}, len(hashes))
	for _, h := range hashes {
		hashSet[h] = struct{}{}
	}
	if _, ok := hashSet["h1"]; !ok {
		t.Error("expected h1 in hashes")
	}
	if _, ok := hashSet["h3"]; !ok {
		t.Error("expected h3 in hashes")
	}
}

// TestBulkSelectedHashesFavoritesTrackLevel verifies that bulkSelectedHashes
// returns hashes for selected tracks at favorites TrackLevel.
func TestBulkSelectedHashesFavoritesTrackLevel(t *testing.T) {
	m := New(context.Background(), WithLibraryTracks(sampleLibraryTracks()))
	m.activeView = viewFavorites
	m.favorites.level = favtui.TrackLevel
	m.favorites.drilldownTracks = []library.Track{
		{Hash: "h4", Artist: "Bob", Title: "Track G1"},
		{Hash: "h5", Artist: "Bob", Title: "Track G2"},
	}
	m.favorites.selectedItems = map[int]struct{}{1: {}}

	hashes := m.bulkSelectedHashes()
	if len(hashes) != 1 {
		t.Fatalf("expected 1 hash, got %d", len(hashes))
	}
	if hashes[0] != "h5" {
		t.Errorf("expected h5, got %s", hashes[0])
	}
}

// TestBulkSelectedHashesFavoritesArtistLevelReturnsNil verifies that
// bulkSelectedHashes returns nil at favorites ListLevel with CategoryArtists.
func TestBulkSelectedHashesFavoritesArtistLevelReturnsNil(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewFavorites
	m.favorites.level = favtui.ListLevel
	m.favorites.category = favtui.CategoryArtists
	m.favorites.artists = []string{"Alice", "Bob"}
	m.favorites.selectedItems = map[int]struct{}{0: {}}

	if hashes := m.bulkSelectedHashes(); hashes != nil {
		t.Errorf("expected nil for artist level, got %v", hashes)
	}
}

// TestBulkSelectedHashesRecentlyAdded verifies that bulkSelectedHashes
// returns hashes from library.selectedHashes when the Library view is active
// in Recently Added mode.
func TestBulkSelectedHashesRecentlyAdded(t *testing.T) {
	m := newModelWithTracks()
	m.library.mode = LibraryModeRecent
	m.activeView = viewLibrary

	m.library.selectedHashes = map[string]struct{}{"h1": {}, "h2": {}}

	hashes := m.bulkSelectedHashes()
	if len(hashes) != 2 {
		t.Fatalf("bulkSelectedHashes (recentlyAdded): expected 2, got %d", len(hashes))
	}
	hashSet := make(map[string]struct{}, len(hashes))
	for _, h := range hashes {
		hashSet[h] = struct{}{}
	}
	if _, ok := hashSet["h1"]; !ok {
		t.Error("expected h1 in hashes")
	}
	if _, ok := hashSet["h2"]; !ok {
		t.Error("expected h2 in hashes")
	}
}

// TestBulkSelectedHashesRecentlyAddedEmptyReturnsNil verifies that
// bulkSelectedHashes returns nil when no tracks are selected in the
// Recently Added view.
func TestBulkSelectedHashesRecentlyAddedEmptyReturnsNil(t *testing.T) {
	m := newModelWithTracks()
	m.library.mode = LibraryModeRecent
	m.activeView = viewLibrary

	if hashes := m.bulkSelectedHashes(); hashes != nil {
		t.Errorf("recentlyAdded with no selection: expected nil, got %v", hashes)
	}
}

func TestBulkSelectedHashesTags(t *testing.T) {
	m := newModelWithTracks()
	m.activeView = viewTags
	m.tagsView.selectedHashes = map[string]struct{}{"h1": {}, "h2": {}}

	hashes := m.bulkSelectedHashes()
	if len(hashes) != 2 {
		t.Fatalf("bulkSelectedHashes (tags): expected 2, got %d", len(hashes))
	}
	hashSet := make(map[string]struct{}, len(hashes))
	for _, h := range hashes {
		hashSet[h] = struct{}{}
	}
	if _, ok := hashSet["h1"]; !ok {
		t.Error("expected h1 in hashes")
	}
	if _, ok := hashSet["h2"]; !ok {
		t.Error("expected h2 in hashes")
	}
}

func TestBulkSelectedHashesTagsEmptyReturnsNil(t *testing.T) {
	m := newModelWithTracks()
	m.activeView = viewTags

	if hashes := m.bulkSelectedHashes(); hashes != nil {
		t.Errorf("tags with no selection: expected nil, got %v", hashes)
	}
}
