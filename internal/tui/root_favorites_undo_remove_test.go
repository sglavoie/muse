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
	m.activeView = viewFavorites
	m.favorites.level = favtui.ListLevel
	m.favorites.category = favtui.CategoryArtists
	// Simulate state after "Bach" (index 0) was removed.
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

	if len(nm.favorites.artists) != 3 {
		t.Fatalf("expected 3 artists after undo, got %d: %v", len(nm.favorites.artists), nm.favorites.artists)
	}
	if nm.favorites.artists[0] != "Bach" {
		t.Fatalf("expected 'Bach' at index 0, got %q", nm.favorites.artists[0])
	}
	if len(w.added) != 1 || w.added[0].typ != "artist" || w.added[0].value != "Bach" {
		t.Fatalf("expected AddFavorite(artist, Bach), got %v", w.added)
	}
	if nm.undo.slot.action != nil {
		t.Fatal("undo slot must be cleared after successful undo")
	}
	if !strings.Contains(nm.statusBar.topNotification().msg, "artist") {
		t.Errorf("expected notification mentioning 'artist', got %q", nm.statusBar.topNotification().msg)
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

// TestUndoRemoveFavorite_FromNonFavoritesView verifies that undo from a
// non-Favorites view (idx == -1) still calls AddFavorite on the DB
// but does not modify any in-view lists.
func TestUndoRemoveFavorite_FromNonFavoritesView(t *testing.T) {
	w := &stubFavoriteWriter{}
	m := New(context.Background(), WithFavoriteWriter(w))
	m.activeView = viewLibrary // not the Favorites view

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
	if len(nm.favorites.artists) != 0 {
		t.Fatalf("expected no artists to be modified, got %v", nm.favorites.artists)
	}
}

// TestUndoBulkRemoveFavorites verifies that undoing a bulk removal of 3 tracks
// (at original indices 1, 2, 4) re-inserts them at their correct positions.
func TestUndoBulkRemoveFavorites(t *testing.T) {
	w := &stubFavoriteWriter{}
	r := &stubFavoriteReader{favorites: map[string][]string{
		"track": {"h0", "h3"},
	}}
	m := New(context.Background(), WithFavoriteReader(r), WithFavoriteWriter(w))
	m.activeView = viewFavorites
	m.favorites.level = favtui.ListLevel
	m.favorites.category = favtui.CategoryTracks

	// After removing 3 tracks (original indices 1, 2, 4), only h0 and h3 remain.
	t0 := library.Track{Hash: "h0", Title: "T0"}
	t3 := library.Track{Hash: "h3", Title: "T3"}
	m.favorites.tracks = []library.Track{t0, t3}
	m.favorites.trackCount = 2

	t1 := library.Track{Hash: "h1", Title: "T1"}
	t2 := library.Track{Hash: "h2", Title: "T2"}
	t4 := library.Track{Hash: "h4", Title: "T4"}

	m.undo.slot = undoSlot{
		action: undoBulkRemoveFavoritesAction{
			typ:      "track",
			values:   []string{"h1", "h2", "h4"},
			tracks:   []library.Track{t1, t2, t4},
			indices:  []int{1, 2, 4},
			level:    favtui.ListLevel,
			category: favtui.CategoryTracks,
		},
		expiresAt: time.Now().Add(undoWindowDuration),
	}

	next, _ := m.Update(UndoMsg{})
	nm := next.(Model)

	if len(nm.favorites.tracks) != 5 {
		t.Fatalf("expected 5 tracks after undo, got %d: %v", len(nm.favorites.tracks), nm.favorites.tracks)
	}
	expected := []string{"h0", "h1", "h2", "h3", "h4"}
	for i, h := range expected {
		if nm.favorites.tracks[i].Hash != h {
			t.Errorf("index %d: got hash %q, want %q", i, nm.favorites.tracks[i].Hash, h)
		}
	}
	if len(w.added) != 3 {
		t.Fatalf("expected 3 AddFavorite calls, got %d: %v", len(w.added), w.added)
	}
	if nm.undo.slot.action != nil {
		t.Fatal("undo slot must be cleared after successful undo")
	}
}

// TestUndoRemoveFavorite_Expired verifies that UndoMsg after the window has
// expired shows "Nothing to undo" and does not call AddFavorite.
func TestUndoRemoveFavorite_Expired(t *testing.T) {
	w := &stubFavoriteWriter{}
	m := New(context.Background(), WithFavoriteWriter(w))
	m.activeView = viewFavorites
	m.favorites.level = favtui.ListLevel
	m.favorites.category = favtui.CategoryArtists
	m.favorites.artists = []string{"Coltrane"}

	m.undo.slot = undoSlot{
		action: undoRemoveFavoriteAction{
			typ:   "artist",
			value: "Bach",
			idx:   0,
		},
		expiresAt: time.Now().Add(-1 * time.Second),
	}

	next, _ := m.Update(UndoMsg{})
	nm := next.(Model)

	if len(w.added) != 0 {
		t.Fatalf("AddFavorite must not be called after expiry, got %v", w.added)
	}
	if len(nm.favorites.artists) != 1 {
		t.Fatalf("artists list must be unchanged, got %v", nm.favorites.artists)
	}
	if !strings.Contains(nm.statusBar.topNotification().msg, "Nothing to undo") {
		t.Errorf("expected 'Nothing to undo' notification, got %q", nm.statusBar.topNotification().msg)
	}
}

// TestUndoRemoveFavorite_Superseded verifies that performing two consecutive
// removals only allows undoing the second (most recent) one.
func TestUndoRemoveFavorite_Superseded(t *testing.T) {
	w := &stubFavoriteWriter{}
	r := &stubFavoriteReader{favorites: map[string][]string{"artist": {}}}
	m := New(context.Background(), WithFavoriteReader(r), WithFavoriteWriter(w))
	m.activeView = viewFavorites
	m.favorites.level = favtui.ListLevel
	m.favorites.category = favtui.CategoryArtists

	m.favorites.artists = []string{}
	m.undo.slot = undoSlot{
		action: undoRemoveFavoriteAction{
			typ:      "artist",
			value:    "Coltrane",
			idx:      0,
			level:    favtui.ListLevel,
			category: favtui.CategoryArtists,
		},
		expiresAt: time.Now().Add(undoWindowDuration),
	}

	next, _ := m.Update(UndoMsg{})
	nm := next.(Model)

	if len(w.added) != 1 {
		t.Fatalf("expected exactly 1 AddFavorite call, got %d: %v", len(w.added), w.added)
	}
	if w.added[0].value != "Coltrane" {
		t.Fatalf("expected AddFavorite for 'Coltrane', got %q", w.added[0].value)
	}
	if nm.undo.slot.action != nil {
		t.Fatal("undo slot must be cleared after successful undo")
	}
}

// TestBulkFavoritesUndoReinsertionOrder verifies that bulk undo correctly
// re-inserts items at their original positions when indices are sorted.
func TestBulkFavoritesUndoReinsertionOrder(t *testing.T) {
	m := Model{
		appCtx:     context.Background(),
		activeView: viewFavorites,
		favorites: FavoritesView{
			level:    favtui.ListLevel,
			category: favtui.CategoryArtists,
			artists:  []string{"A", "C", "E"},
		},
	}

	action := undoBulkRemoveFavoritesAction{
		typ:      "artist",
		values:   []string{"B", "D"},
		indices:  []int{1, 3},
		level:    favtui.ListLevel,
		category: favtui.CategoryArtists,
	}
	m, _ = action.perform(m)

	want := []string{"A", "B", "C", "D", "E"}
	if len(m.favorites.artists) != len(want) {
		t.Fatalf("got %d artists, want %d", len(m.favorites.artists), len(want))
	}
	for i, w := range want {
		if m.favorites.artists[i] != w {
			t.Errorf("artists[%d] = %q, want %q", i, m.favorites.artists[i], w)
		}
	}
}

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

// TestBulkFavoritesUndoReinsertionOrder_Unsorted verifies that without sorting,
// the undo produces incorrect results (validating the fix is necessary).
func TestBulkFavoritesUndoReinsertionOrder_Unsorted(t *testing.T) {
	m := Model{
		appCtx:     context.Background(),
		activeView: viewFavorites,
		favorites: FavoritesView{
			level:    favtui.ListLevel,
			category: favtui.CategoryArtists,
			artists:  []string{"A", "C", "E"},
		},
	}

	action := undoBulkRemoveFavoritesAction{
		typ:      "artist",
		values:   []string{"D", "B"},
		indices:  []int{3, 1},
		level:    favtui.ListLevel,
		category: favtui.CategoryArtists,
	}
	m, _ = action.perform(m)

	correct := []string{"A", "B", "C", "D", "E"}
	isCorrect := len(m.favorites.artists) == len(correct)
	if isCorrect {
		for i, w := range correct {
			if m.favorites.artists[i] != w {
				isCorrect = false
				break
			}
		}
	}
	if isCorrect {
		t.Error("expected incorrect result with unsorted indices, but got correct result")
	}
}
