package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/library"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
)

// TestUndoRemoveFavorite_SingleFromFavoritesView verifies that undoing a single
// artist removal from the Favorites view re-inserts the artist at its original
// index and calls AddFavorite on the writer.
func TestUndoRemoveFavorite_SingleFromFavoritesView(t *testing.T) {
	w := &stubFavoriteWriter{}
	r := &stubFavoriteReader{favorites: map[string][]string{
		"artist": {"Bach", "Coltrane", "Davis"},
	}}
	m := New(context.Background(), WithFavoriteReader(r), WithFavoriteWriter(w))
	m.ActiveView = viewFavorites
	m.favorites.Level = favtui.ListLevel
	m.favorites.Category = favtui.CategoryArtists
	// Simulate state after "Bach" (index 0) was removed.
	m.favorites.Artists = []string{"Coltrane", "Davis"}
	m.favorites.ArtistCount = 2

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

	if len(nm.favorites.Artists) != 3 {
		t.Fatalf("expected 3 artists after undo, got %d: %v", len(nm.favorites.Artists), nm.favorites.Artists)
	}
	if nm.favorites.Artists[0] != "Bach" {
		t.Fatalf("expected 'Bach' at index 0, got %q", nm.favorites.Artists[0])
	}
	if len(w.added) != 1 || w.added[0].typ != "artist" || w.added[0].value != "Bach" {
		t.Fatalf("expected AddFavorite(artist, Bach), got %v", w.added)
	}
	if nm.undo.slot.action != nil {
		t.Fatal("undo slot must be cleared after successful undo")
	}
	if !strings.Contains(nm.statusBar.TopNotification().Msg, "artist") {
		t.Errorf("expected notification mentioning 'artist', got %q", nm.statusBar.TopNotification().Msg)
	}
}

// TestUndoRemoveFavorite_TrackHashRestored verifies that undoing a track
// removal restores the hash in the in-memory favoriteHashes set.
func TestUndoRemoveFavorite_TrackHashRestored(t *testing.T) {
	w := &stubFavoriteWriter{}
	r := &stubFavoriteReader{favorites: map[string][]string{
		"track": {"hash-1", "hash-2"},
	}}
	m := New(context.Background(), WithFavoriteReader(r), WithFavoriteWriter(w))
	// Simulate that hash-1 was removed from the in-memory set.
	m.favoriteHashes = map[string]struct{}{"hash-2": {}}

	m.undo.slot = undoSlot{
		action: undoRemoveFavoriteAction{
			typ:   "track",
			value: "hash-1",
			idx:   -1, // not from Favorites view
		},
		expiresAt: time.Now().Add(undoWindowDuration),
	}

	next, _ := m.Update(UndoMsg{})
	nm := next.(Model)

	if _, ok := nm.favoriteHashes["hash-1"]; !ok {
		t.Fatal("expected hash-1 to be restored in favoriteHashes")
	}
	if len(w.added) != 1 || w.added[0].value != "hash-1" {
		t.Fatalf("expected AddFavorite called with hash-1, got %v", w.added)
	}
}

func TestUndoRemoveFavorite_TrackLevelKeepsDrilldownTrack(t *testing.T) {
	w := &stubFavoriteWriter{}
	r := &stubFavoriteReader{favorites: map[string][]string{
		"track": {"h1", "h2"},
	}}
	m := New(context.Background(), WithFavoriteReader(r), WithFavoriteWriter(w))
	m.ActiveView = viewFavorites
	m.favorites.Level = favtui.TrackLevel
	m.favorites.Category = favtui.CategoryArtists
	m.favorites.DrilldownTracks = []library.Track{
		{Hash: "h1", Title: "T1"},
		{Hash: "h2", Title: "T2"},
	}
	m.favoriteHashes = map[string]struct{}{"h2": {}}

	m.undo.slot = undoSlot{
		action: undoRemoveFavoriteAction{
			typ:   "track",
			value: "h1",
			track: library.Track{Hash: "h1", Title: "T1"},
			idx:   0,
			level: favtui.TrackLevel,
		},
		expiresAt: time.Now().Add(undoWindowDuration),
	}

	next, _ := m.Update(UndoMsg{})
	nm := next.(Model)

	if len(nm.favorites.DrilldownTracks) != 2 {
		t.Fatalf("expected 2 drilldown tracks after undo, got %d", len(nm.favorites.DrilldownTracks))
	}
	if nm.favorites.DrilldownTracks[0].Hash != "h1" || nm.favorites.DrilldownTracks[1].Hash != "h2" {
		t.Fatalf("expected [h1 h2], got [%s %s]", nm.favorites.DrilldownTracks[0].Hash, nm.favorites.DrilldownTracks[1].Hash)
	}
	if _, ok := nm.favoriteHashes["h1"]; !ok {
		t.Fatal("expected h1 to be restored in favoriteHashes")
	}
}

// TestUndoRemoveFavorite_FromNonFavoritesView verifies that undo from a
// non-Favorites view (idx == -1) still calls AddFavorite on the DB
// but does not modify any in-view lists.
func TestUndoRemoveFavorite_FromNonFavoritesView(t *testing.T) {
	w := &stubFavoriteWriter{}
	m := New(context.Background(), WithFavoriteWriter(w))
	m.ActiveView = viewLibrary // not the Favorites view

	m.undo.slot = undoSlot{
		action: undoRemoveFavoriteAction{
			typ:   "artist",
			value: "Miles Davis",
			idx:   -1,
		},
		expiresAt: time.Now().Add(undoWindowDuration),
	}

	next, _ := m.Update(UndoMsg{})
	nm := next.(Model)

	if len(w.added) != 1 || w.added[0].value != "Miles Davis" {
		t.Fatalf("expected AddFavorite(artist, Miles Davis), got %v", w.added)
	}
	if len(nm.favorites.Artists) != 0 {
		t.Fatalf("expected no artists to be modified, got %v", nm.favorites.Artists)
	}
}
