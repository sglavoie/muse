package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/library"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
)

// TestBulkFavoriteRemoval_UpdatesInViewState verifies that bulk removal of
// favorites from the Favorites view correctly removes all items from the
// in-memory view lists, not just the single msg.value (which is empty for
// bulk operations).
func TestBulkFavoriteRemoval_UpdatesInViewState(t *testing.T) {
	w := &stubFavoriteWriter{}
	r := &stubFavoriteReader{favorites: map[string][]string{
		"artist": {"Bach", "Davis"},
	}}
	m := New(context.Background(), WithFavoriteReader(r), WithFavoriteWriter(w))
	m.ActiveView = viewFavorites
	m.favorites.Level = favtui.ListLevel
	m.favorites.Category = favtui.CategoryArtists
	m.favorites.Artists = []string{"Bach", "Coltrane", "Davis", "Ellington"}
	m.favorites.ArtistCount = 4

	msg := favoriteToggledMsg{
		typ:           "artist",
		value:         "",
		count:         2,
		added:         false,
		removedValues: []string{"Bach", "Davis"},
		favLevel:      favtui.ListLevel,
		favCategory:   favtui.CategoryArtists,
	}
	nm := m.handleFavoriteToggledMsg(msg)

	if len(nm.favorites.Artists) != 2 {
		t.Fatalf("expected 2 artists after bulk removal, got %d: %v", len(nm.favorites.Artists), nm.favorites.Artists)
	}
	if nm.favorites.Artists[0] != "Coltrane" || nm.favorites.Artists[1] != "Ellington" {
		t.Fatalf("expected [Coltrane, Ellington], got %v", nm.favorites.Artists)
	}
}

// TestBulkFavoriteRemoval_TrackLevelPreservesDrilldownTracks verifies that
// removing track favorites from an artist/album drilldown only clears the
// favorite marker state and does not remove rows from drilldownTracks.
func TestBulkFavoriteRemoval_TrackLevelPreservesDrilldownTracks(t *testing.T) {
	r := &stubFavoriteReader{favorites: map[string][]string{
		"track": {"h2", "h4"},
	}}
	m := New(context.Background(), WithFavoriteReader(r))
	m.ActiveView = viewFavorites
	m.favorites.Level = favtui.TrackLevel
	m.favorites.Category = favtui.CategoryArtists
	m.favoriteHashes = map[string]struct{}{"h1": {}, "h2": {}, "h3": {}, "h4": {}}

	t1 := library.Track{Hash: "h1", Title: "T1"}
	t2 := library.Track{Hash: "h2", Title: "T2"}
	t3 := library.Track{Hash: "h3", Title: "T3"}
	t4 := library.Track{Hash: "h4", Title: "T4"}
	m.favorites.DrilldownTracks = []library.Track{t1, t2, t3, t4}

	msg := favoriteToggledMsg{
		typ:           "track",
		count:         2,
		added:         false,
		removedValues: []string{"h1", "h3"},
		favLevel:      favtui.TrackLevel,
	}
	nm := m.handleFavoriteToggledMsg(msg)

	if len(nm.favorites.DrilldownTracks) != 4 {
		t.Fatalf("expected 4 drilldown tracks after bulk removal, got %d", len(nm.favorites.DrilldownTracks))
	}
	expected := []string{"h1", "h2", "h3", "h4"}
	for i, hash := range expected {
		if nm.favorites.DrilldownTracks[i].Hash != hash {
			t.Fatalf("index %d: expected %q, got %q", i, hash, nm.favorites.DrilldownTracks[i].Hash)
		}
	}
	if _, ok := nm.favoriteHashes["h1"]; ok {
		t.Fatal("expected h1 to be removed from favoriteHashes")
	}
	if _, ok := nm.favoriteHashes["h3"]; ok {
		t.Fatal("expected h3 to be removed from favoriteHashes")
	}
	if _, ok := nm.favoriteHashes["h2"]; !ok {
		t.Fatal("expected h2 to remain in favoriteHashes")
	}
	if _, ok := nm.favoriteHashes["h4"]; !ok {
		t.Fatal("expected h4 to remain in favoriteHashes")
	}
}

func TestSingleFavoriteRemoval_TrackLevelPreservesDrilldownTrack(t *testing.T) {
	r := &stubFavoriteReader{favorites: map[string][]string{
		"track": {"h2", "h3"},
	}}
	m := New(context.Background(), WithFavoriteReader(r))
	m.ActiveView = viewFavorites
	m.favorites.Level = favtui.TrackLevel
	m.favorites.Category = favtui.CategoryAlbums
	m.favorites.DrilldownTracks = []library.Track{
		{Hash: "h1", Title: "T1"},
		{Hash: "h2", Title: "T2"},
		{Hash: "h3", Title: "T3"},
	}
	m.favoriteHashes = map[string]struct{}{"h1": {}, "h2": {}, "h3": {}}
	m.favorites.SelectedItems = map[int]struct{}{0: {}}

	msg := favoriteToggledMsg{
		typ:         "track",
		value:       "h1",
		count:       1,
		added:       false,
		favLevel:    favtui.TrackLevel,
		favCategory: favtui.CategoryAlbums,
	}
	nm := m.handleFavoriteToggledMsg(msg)

	if len(nm.favorites.DrilldownTracks) != 3 {
		t.Fatalf("expected 3 drilldown tracks after single removal, got %d", len(nm.favorites.DrilldownTracks))
	}
	expected := []string{"h1", "h2", "h3"}
	for i, hash := range expected {
		if nm.favorites.DrilldownTracks[i].Hash != hash {
			t.Fatalf("index %d: expected %q, got %q", i, hash, nm.favorites.DrilldownTracks[i].Hash)
		}
	}
	if _, ok := nm.favoriteHashes["h1"]; ok {
		t.Fatal("expected h1 to be removed from favoriteHashes")
	}
	if nm.favorites.SelectedItems != nil {
		t.Fatalf("expected selectedItems to be nil after removal, got %v", nm.favorites.SelectedItems)
	}
}

// TestSingleFavoriteRemoval_StillWorksWithValue verifies that single removal
// (msg.value populated, msg.removedValues empty) still updates the in-view state.
func TestSingleFavoriteRemoval_StillWorksWithValue(t *testing.T) {
	r := &stubFavoriteReader{favorites: map[string][]string{
		"album": {"Jazz", "Rock"},
	}}
	m := New(context.Background(), WithFavoriteReader(r))
	m.ActiveView = viewFavorites
	m.favorites.Level = favtui.ListLevel
	m.favorites.Category = favtui.CategoryAlbums
	m.favorites.Albums = []string{"Jazz", "Rock", "Blues"}
	m.favorites.AlbumCount = 3

	msg := favoriteToggledMsg{
		typ:         "album",
		value:       "Rock",
		count:       1,
		added:       false,
		favLevel:    favtui.ListLevel,
		favCategory: favtui.CategoryAlbums,
	}
	nm := m.handleFavoriteToggledMsg(msg)

	if len(nm.favorites.Albums) != 2 {
		t.Fatalf("expected 2 albums after single removal, got %d: %v", len(nm.favorites.Albums), nm.favorites.Albums)
	}
	if nm.favorites.Albums[0] != "Jazz" || nm.favorites.Albums[1] != "Blues" {
		t.Fatalf("expected [Jazz, Blues], got %v", nm.favorites.Albums)
	}
}

// TestFavoritesLoadedMsg_SavesCategoryCursor verifies that transitioning from
// CategoryLevel to ListLevel via handleFavoritesLoadedMsg saves the category
// cursor so that back() restores it correctly. Without the fix, categoryCursor
// remained 0 and back() always returned to the first category regardless of
// which one was opened.
func TestFavoritesLoadedMsg_SavesCategoryCursor(t *testing.T) {
	m := New(context.Background())
	m.ActiveView = viewFavorites
	m.favorites.Level = favtui.CategoryLevel
	m.favorites.Cursor = 1

	msg := favoritesLoadedMsg{
		category: int(favtui.CategoryAlbums),
		albums:   []string{"Jazz", "Rock"},
	}
	m = m.handleFavoritesLoadedMsg(msg)

	if m.favorites.Level != favtui.ListLevel {
		t.Fatalf("expected ListLevel, got %d", m.favorites.Level)
	}
	if m.favorites.CategoryCursor != 1 {
		t.Fatalf("expected categoryCursor=1 (Albums), got %d", m.favorites.CategoryCursor)
	}

	m.favorites = m.favorites.Back()

	if m.favorites.Level != favtui.CategoryLevel {
		t.Fatalf("expected CategoryLevel after back(), got %d", m.favorites.Level)
	}
	if m.favorites.Cursor != 1 {
		t.Fatalf("expected cursor=1 (Albums) after back(), got %d", m.favorites.Cursor)
	}
}

// TestFavoritesLoadedMsg_SavesCategoryCursor_Tracks verifies the fix for
// cursor=2 (Tracks category).
func TestFavoritesLoadedMsg_SavesCategoryCursor_Tracks(t *testing.T) {
	m := New(context.Background())
	m.ActiveView = viewFavorites
	m.favorites.Level = favtui.CategoryLevel
	m.favorites.Cursor = 2

	msg := favoritesLoadedMsg{
		category: int(favtui.CategoryTracks),
		tracks:   []library.Track{{Hash: "h1", Title: "Song"}},
	}
	m = m.handleFavoritesLoadedMsg(msg)

	if m.favorites.CategoryCursor != 2 {
		t.Fatalf("expected categoryCursor=2 (Tracks), got %d", m.favorites.CategoryCursor)
	}

	m.favorites = m.favorites.Back()
	if m.favorites.Cursor != 2 {
		t.Fatalf("expected cursor=2 after back(), got %d", m.favorites.Cursor)
	}
}

// TestSingleFavoriteRemoval_ClearsSelectedItems verifies that removing a
// single favorite from the view clears the index-based selectedItems map so
// stale indices do not point at wrong items after the list shifts.
func TestSingleFavoriteRemoval_ClearsSelectedItems(t *testing.T) {
	r := &stubFavoriteReader{favorites: map[string][]string{
		"track": {"h1", "h3"},
	}}
	m := New(context.Background(), WithFavoriteReader(r))
	m.ActiveView = viewFavorites
	m.favorites.Level = favtui.ListLevel
	m.favorites.Category = favtui.CategoryTracks
	m.favorites.Tracks = []library.Track{
		{Hash: "h1", Title: "T1"},
		{Hash: "h2", Title: "T2"},
		{Hash: "h3", Title: "T3"},
	}
	m.favorites.TrackCount = 3
	m.favorites.SelectedItems = map[int]struct{}{0: {}, 2: {}}

	msg := favoriteToggledMsg{
		typ:         "track",
		value:       "h2",
		count:       1,
		added:       false,
		favLevel:    favtui.ListLevel,
		favCategory: favtui.CategoryTracks,
	}
	nm := m.handleFavoriteToggledMsg(msg)

	if nm.favorites.SelectedItems != nil {
		t.Fatalf("expected selectedItems to be nil after removal, got %v", nm.favorites.SelectedItems)
	}
}

// TestBulkFavoriteRemoval_ClearsSelectedItems_TrackLevel verifies that bulk
// removal at TrackLevel clears selectedItems.
func TestBulkFavoriteRemoval_ClearsSelectedItems_TrackLevel(t *testing.T) {
	r := &stubFavoriteReader{favorites: map[string][]string{
		"track": {"h1"},
	}}
	m := New(context.Background(), WithFavoriteReader(r))
	m.ActiveView = viewFavorites
	m.favorites.Level = favtui.TrackLevel
	m.favorites.DrilldownTracks = []library.Track{
		{Hash: "h1", Title: "T1"},
		{Hash: "h2", Title: "T2"},
		{Hash: "h3", Title: "T3"},
	}
	m.favorites.SelectedItems = map[int]struct{}{0: {}, 1: {}}

	msg := favoriteToggledMsg{
		typ:           "track",
		count:         2,
		added:         false,
		removedValues: []string{"h1", "h3"},
		favLevel:      favtui.TrackLevel,
	}
	nm := m.handleFavoriteToggledMsg(msg)

	if nm.favorites.SelectedItems != nil {
		t.Fatalf("expected selectedItems to be nil after bulk removal, got %v", nm.favorites.SelectedItems)
	}
}
