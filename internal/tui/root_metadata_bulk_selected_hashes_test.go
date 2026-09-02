package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/library"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	libtui "github.com/sglavoie/muse/internal/tui/library"
)

// TestBulkSelectedHashesFavoritesListLevelTracks verifies that
// bulkSelectedHashes returns hashes for selected tracks at favorites ListLevel.
func TestBulkSelectedHashesFavoritesListLevelTracks(t *testing.T) {
	m := New(context.Background(), WithLibraryTracks(sampleLibraryTracks()))
	m.ActiveView = viewFavorites
	m.favorites.Level = favtui.ListLevel
	m.favorites.Category = favtui.CategoryTracks
	m.favorites.Tracks = []library.Track{
		{Hash: "h1", Artist: "Alice", Title: "Track A1"},
		{Hash: "h2", Artist: "Alice", Title: "Track A2"},
		{Hash: "h3", Artist: "Alice", Title: "Track B1"},
	}
	m.favorites.SelectedItems = map[int]struct{}{0: {}, 2: {}}

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
	m.ActiveView = viewFavorites
	m.favorites.Level = favtui.TrackLevel
	m.favorites.DrilldownTracks = []library.Track{
		{Hash: "h4", Artist: "Bob", Title: "Track G1"},
		{Hash: "h5", Artist: "Bob", Title: "Track G2"},
	}
	m.favorites.SelectedItems = map[int]struct{}{1: {}}

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
	m.ActiveView = viewFavorites
	m.favorites.Level = favtui.ListLevel
	m.favorites.Category = favtui.CategoryArtists
	m.favorites.Artists = []string{"Alice", "Bob"}
	m.favorites.SelectedItems = map[int]struct{}{0: {}}

	if hashes := m.bulkSelectedHashes(); hashes != nil {
		t.Errorf("expected nil for artist level, got %v", hashes)
	}
}

// TestBulkSelectedHashesRecentlyAdded verifies that bulkSelectedHashes
// returns hashes from library.selectedHashes when the Library view is active
// in Recently Added mode.
func TestBulkSelectedHashesRecentlyAdded(t *testing.T) {
	m := newModelWithTracks()
	m.library.Mode = libtui.LibraryModeRecent
	m.ActiveView = viewLibrary

	m.library.SelectedHashes = map[string]struct{}{"h1": {}, "h2": {}}

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
	m.library.Mode = libtui.LibraryModeRecent
	m.ActiveView = viewLibrary

	if hashes := m.bulkSelectedHashes(); hashes != nil {
		t.Errorf("recentlyAdded with no selection: expected nil, got %v", hashes)
	}
}

func TestBulkSelectedHashesTags(t *testing.T) {
	m := newModelWithTracks()
	m.ActiveView = viewTags
	m.tagsView.SelectedHashes = map[string]struct{}{"h1": {}, "h2": {}}

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
	m.ActiveView = viewTags

	if hashes := m.bulkSelectedHashes(); hashes != nil {
		t.Errorf("tags with no selection: expected nil, got %v", hashes)
	}
}
