package tui

import (
	"context"
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/library"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
)

// TestUndoRemoveFavorite_CrossCategoryCursorUnchanged verifies that undoing a
// single artist removal while viewing the albums category does NOT move the
// albums cursor to the saved artist index.
func TestUndoRemoveFavorite_CrossCategoryCursorUnchanged(t *testing.T) {
	w := &stubFavoriteWriter{}
	r := &stubFavoriteReader{favorites: map[string][]string{
		"artist": {"Bach", "Coltrane", "Davis"},
		"album":  {"AlbumA", "AlbumB"},
	}}
	m := New(context.Background(), WithFavoriteReader(r), WithFavoriteWriter(w))
	m.activeView = viewFavorites
	m.favorites.level = favtui.ListLevel
	// Currently viewing albums, but undo is for an artist removal.
	m.favorites.category = favtui.CategoryAlbums
	m.favorites.albums = []string{"AlbumA", "AlbumB"}
	m.favorites.albumCount = 2
	m.favorites.cursor = 1 // cursor on AlbumB

	// Simulate state after "Bach" (index 0) was removed from artists.
	m.favorites.artists = []string{"Coltrane", "Davis"}
	m.favorites.artistCount = 2

	m.undo.slot = undoSlot{
		action: undoRemoveFavoriteAction{
			typ:      "artist",
			value:    "Bach",
			idx:      0,
			level:    favtui.ListLevel,
			category: favtui.CategoryArtists,
		},
		expiresAt: time.Now().Add(undoWindowDuration),
	}

	next, _ := m.Update(UndoMsg{})
	nm := next.(Model)

	// Artist list should be restored.
	if len(nm.favorites.artists) != 3 {
		t.Fatalf("expected 3 artists after undo, got %d", len(nm.favorites.artists))
	}
	// Cursor should still be on AlbumB (index 1), NOT moved to 0.
	if nm.favorites.cursor != 1 {
		t.Fatalf("expected cursor to remain at 1 (AlbumB), got %d", nm.favorites.cursor)
	}
}

// TestUndoBulkRemoveFavorites_CrossCategoryCursorUnchanged verifies that
// bulk-undoing artist removals while viewing tracks does NOT clamp the tracks
// cursor based on the artist list length.
func TestUndoBulkRemoveFavorites_CrossCategoryCursorUnchanged(t *testing.T) {
	m := Model{
		appCtx:     context.Background(),
		activeView: viewFavorites,
		favorites: FavoritesView{
			level:    favtui.ListLevel,
			category: favtui.CategoryTracks, // viewing tracks
			tracks: []library.Track{
				{Hash: "t1", Title: "Track1"},
				{Hash: "t2", Title: "Track2"},
				{Hash: "t3", Title: "Track3"},
			},
			trackCount: 3,
			cursor:     2, // cursor on Track3
			artists:    []string{"A"},
		},
	}

	action := undoBulkRemoveFavoritesAction{
		typ:      "artist",
		values:   []string{"B", "C"},
		indices:  []int{1, 2},
		level:    favtui.ListLevel,
		category: favtui.CategoryArtists, // undo is for artists
	}
	m, _ = action.perform(m)

	// Artists should be restored.
	if len(m.favorites.artists) != 3 {
		t.Fatalf("expected 3 artists after undo, got %d", len(m.favorites.artists))
	}
	// Cursor should still be at 2 (Track3), not clamped by artist count.
	if m.favorites.cursor != 2 {
		t.Fatalf("expected cursor to remain at 2, got %d", m.favorites.cursor)
	}
}
