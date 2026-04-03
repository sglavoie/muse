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
	m.activeView = viewFavorites
	m.favorites.level = favtui.ListLevel
	m.favorites.category = favtui.CategoryArtists
	m.favorites.artists = []string{"Bach", "Coltrane", "Davis", "Ellington"}
	m.favorites.artistCount = 4

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

	if len(nm.favorites.artists) != 2 {
		t.Fatalf("expected 2 artists after bulk removal, got %d: %v", len(nm.favorites.artists), nm.favorites.artists)
	}
	if nm.favorites.artists[0] != "Coltrane" || nm.favorites.artists[1] != "Ellington" {
		t.Fatalf("expected [Coltrane, Ellington], got %v", nm.favorites.artists)
	}
}

// TestBulkFavoriteRemoval_TracksAtTrackLevel verifies that bulk removal of
// track favorites from the TrackLevel correctly removes tracks from
// drilldownTracks.
func TestBulkFavoriteRemoval_TracksAtTrackLevel(t *testing.T) {
	r := &stubFavoriteReader{favorites: map[string][]string{
		"track": {"h1", "h4"},
	}}
	m := New(context.Background(), WithFavoriteReader(r))
	m.activeView = viewFavorites
	m.favorites.level = favtui.TrackLevel

	t1 := library.Track{Hash: "h1", Title: "T1"}
	t2 := library.Track{Hash: "h2", Title: "T2"}
	t3 := library.Track{Hash: "h3", Title: "T3"}
	t4 := library.Track{Hash: "h4", Title: "T4"}
	m.favorites.drilldownTracks = []library.Track{t1, t2, t3, t4}

	msg := favoriteToggledMsg{
		typ:           "track",
		count:         2,
		added:         false,
		removedValues: []string{"h1", "h3"},
		favLevel:      favtui.TrackLevel,
	}
	nm := m.handleFavoriteToggledMsg(msg)

	if len(nm.favorites.drilldownTracks) != 2 {
		t.Fatalf("expected 2 tracks after bulk removal, got %d", len(nm.favorites.drilldownTracks))
	}
	if nm.favorites.drilldownTracks[0].Hash != "h2" || nm.favorites.drilldownTracks[1].Hash != "h4" {
		t.Fatalf("expected [h2, h4], got [%s, %s]",
			nm.favorites.drilldownTracks[0].Hash, nm.favorites.drilldownTracks[1].Hash)
	}
}

// TestSingleFavoriteRemoval_StillWorksWithValue verifies that single removal
// (msg.value populated, msg.removedValues empty) still updates the in-view state.
func TestSingleFavoriteRemoval_StillWorksWithValue(t *testing.T) {
	r := &stubFavoriteReader{favorites: map[string][]string{
		"album": {"Jazz", "Rock"},
	}}
	m := New(context.Background(), WithFavoriteReader(r))
	m.activeView = viewFavorites
	m.favorites.level = favtui.ListLevel
	m.favorites.category = favtui.CategoryAlbums
	m.favorites.albums = []string{"Jazz", "Rock", "Blues"}
	m.favorites.albumCount = 3

	msg := favoriteToggledMsg{
		typ:         "album",
		value:       "Rock",
		count:       1,
		added:       false,
		favLevel:    favtui.ListLevel,
		favCategory: favtui.CategoryAlbums,
	}
	nm := m.handleFavoriteToggledMsg(msg)

	if len(nm.favorites.albums) != 2 {
		t.Fatalf("expected 2 albums after single removal, got %d: %v", len(nm.favorites.albums), nm.favorites.albums)
	}
	if nm.favorites.albums[0] != "Jazz" || nm.favorites.albums[1] != "Blues" {
		t.Fatalf("expected [Jazz, Blues], got %v", nm.favorites.albums)
	}
}

// TestFavoritesLoadedMsg_SavesCategoryCursor verifies that transitioning from
// CategoryLevel to ListLevel via handleFavoritesLoadedMsg saves the category
// cursor so that back() restores it correctly. Without the fix, categoryCursor
// remained 0 and back() always returned to the first category regardless of
// which one was opened.
func TestFavoritesLoadedMsg_SavesCategoryCursor(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewFavorites
	m.favorites.level = favtui.CategoryLevel
	m.favorites.cursor = 1

	msg := favoritesLoadedMsg{
		category: int(favtui.CategoryAlbums),
		albums:   []string{"Jazz", "Rock"},
	}
	m = m.handleFavoritesLoadedMsg(msg)

	if m.favorites.level != favtui.ListLevel {
		t.Fatalf("expected ListLevel, got %d", m.favorites.level)
	}
	if m.favorites.categoryCursor != 1 {
		t.Fatalf("expected categoryCursor=1 (Albums), got %d", m.favorites.categoryCursor)
	}

	m.favorites = m.favorites.back()

	if m.favorites.level != favtui.CategoryLevel {
		t.Fatalf("expected CategoryLevel after back(), got %d", m.favorites.level)
	}
	if m.favorites.cursor != 1 {
		t.Fatalf("expected cursor=1 (Albums) after back(), got %d", m.favorites.cursor)
	}
}

// TestFavoritesLoadedMsg_SavesCategoryCursor_Tracks verifies the fix for
// cursor=2 (Tracks category).
func TestFavoritesLoadedMsg_SavesCategoryCursor_Tracks(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewFavorites
	m.favorites.level = favtui.CategoryLevel
	m.favorites.cursor = 2

	msg := favoritesLoadedMsg{
		category: int(favtui.CategoryTracks),
		tracks:   []library.Track{{Hash: "h1", Title: "Song"}},
	}
	m = m.handleFavoritesLoadedMsg(msg)

	if m.favorites.categoryCursor != 2 {
		t.Fatalf("expected categoryCursor=2 (Tracks), got %d", m.favorites.categoryCursor)
	}

	m.favorites = m.favorites.back()
	if m.favorites.cursor != 2 {
		t.Fatalf("expected cursor=2 after back(), got %d", m.favorites.cursor)
	}
}
