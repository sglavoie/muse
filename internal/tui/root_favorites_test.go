package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/library"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
)

// TestInsertStringAt verifies insertStringAt for various insertion positions.
func TestInsertStringAt(t *testing.T) {
	tests := []struct {
		name  string
		items []string
		idx   int
		s     string
		want  []string
	}{
		{"insert at start", []string{"b", "c"}, 0, "a", []string{"a", "b", "c"}},
		{"insert in middle", []string{"a", "c"}, 1, "b", []string{"a", "b", "c"}},
		{"insert at end", []string{"a", "b"}, 2, "c", []string{"a", "b", "c"}},
		{"clamp negative idx", []string{"a", "b"}, -1, "z", []string{"z", "a", "b"}},
		{"clamp idx beyond len", []string{"a", "b"}, 10, "z", []string{"a", "b", "z"}},
		{"empty slice", nil, 0, "x", []string{"x"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := insertStringAt(tc.items, tc.idx, tc.s)
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Fatalf("index %d: got %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// TestInsertTrackAt verifies insertTrackAt for various insertion positions.
func TestInsertTrackAt(t *testing.T) {
	a := library.Track{Hash: "aaa", Title: "A"}
	b := library.Track{Hash: "bbb", Title: "B"}
	c := library.Track{Hash: "ccc", Title: "C"}

	tests := []struct {
		name   string
		tracks []library.Track
		idx    int
		t      library.Track
		want   []library.Track
	}{
		{"insert at start", []library.Track{b, c}, 0, a, []library.Track{a, b, c}},
		{"insert in middle", []library.Track{a, c}, 1, b, []library.Track{a, b, c}},
		{"insert at end", []library.Track{a, b}, 2, c, []library.Track{a, b, c}},
		{"clamp negative idx", []library.Track{a, b}, -1, c, []library.Track{c, a, b}},
		{"clamp idx beyond len", []library.Track{a, b}, 10, c, []library.Track{a, b, c}},
		{"empty slice", nil, 0, a, []library.Track{a}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := insertTrackAt(tc.tracks, tc.idx, tc.t)
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range tc.want {
				if got[i].Hash != tc.want[i].Hash {
					t.Fatalf("index %d: got hash %q, want %q", i, got[i].Hash, tc.want[i].Hash)
				}
			}
		})
	}
}

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
		kind:              undoKindRemoveFavorite,
		expiresAt:         time.Now().Add(undoWindowDuration),
		removeFavType:     "artist",
		removeFavValue:    "Bach",
		removeFavIdx:      0,
		removeFavLevel:    favtui.ListLevel,
		removeFavCategory: favtui.CategoryArtists,
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
	if nm.undo.slot.kind != undoNone {
		t.Fatal("undo slot must be cleared after successful undo")
	}
	if !strings.Contains(nm.statusBar.notification.msg, "artist") {
		t.Errorf("expected notification mentioning 'artist', got %q", nm.statusBar.notification.msg)
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
		kind:           undoKindRemoveFavorite,
		expiresAt:      time.Now().Add(undoWindowDuration),
		removeFavType:  "track",
		removeFavValue: "hash-1",
		removeFavIdx:   -1, // not from Favorites view
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
// non-Favorites view (removeFavIdx == -1) still calls AddFavorite on the DB
// but does not modify any in-view lists.
func TestUndoRemoveFavorite_FromNonFavoritesView(t *testing.T) {
	w := &stubFavoriteWriter{}
	m := New(context.Background(), WithFavoriteWriter(w))
	m.activeView = viewLibrary // not the Favorites view

	m.undo.slot = undoSlot{
		kind:           undoKindRemoveFavorite,
		expiresAt:      time.Now().Add(undoWindowDuration),
		removeFavType:  "artist",
		removeFavValue: "Miles Davis",
		removeFavIdx:   -1,
	}

	next, _ := m.Update(UndoMsg{})
	nm := next.(Model)

	if len(w.added) != 1 || w.added[0].value != "Miles Davis" {
		t.Fatalf("expected AddFavorite(artist, Miles Davis), got %v", w.added)
	}
	// No in-view list manipulation should have happened.
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
		kind:                  undoKindBulkRemoveFavorites,
		expiresAt:             time.Now().Add(undoWindowDuration),
		bulkRemoveFavType:     "track",
		bulkRemoveFavValues:   []string{"h1", "h2", "h4"},
		bulkRemoveFavTracks:   []library.Track{t1, t2, t4},
		bulkRemoveFavIndices:  []int{1, 2, 4},
		bulkRemoveFavLevel:    favtui.ListLevel,
		bulkRemoveFavCategory: favtui.CategoryTracks,
	}

	next, _ := m.Update(UndoMsg{})
	nm := next.(Model)

	if len(nm.favorites.tracks) != 5 {
		t.Fatalf("expected 5 tracks after undo, got %d: %v", len(nm.favorites.tracks), nm.favorites.tracks)
	}
	// Verify each position.
	expected := []string{"h0", "h1", "h2", "h3", "h4"}
	for i, h := range expected {
		if nm.favorites.tracks[i].Hash != h {
			t.Errorf("index %d: got hash %q, want %q", i, nm.favorites.tracks[i].Hash, h)
		}
	}
	if len(w.added) != 3 {
		t.Fatalf("expected 3 AddFavorite calls, got %d: %v", len(w.added), w.added)
	}
	if nm.undo.slot.kind != undoNone {
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
		kind:           undoKindRemoveFavorite,
		expiresAt:      time.Now().Add(-1 * time.Second), // already expired
		removeFavType:  "artist",
		removeFavValue: "Bach",
		removeFavIdx:   0,
	}

	next, _ := m.Update(UndoMsg{})
	nm := next.(Model)

	if len(w.added) != 0 {
		t.Fatalf("AddFavorite must not be called after expiry, got %v", w.added)
	}
	if len(nm.favorites.artists) != 1 {
		t.Fatalf("artists list must be unchanged, got %v", nm.favorites.artists)
	}
	if !strings.Contains(nm.statusBar.notification.msg, "Nothing to undo") {
		t.Errorf("expected 'Nothing to undo' notification, got %q", nm.statusBar.notification.msg)
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

	// Set up state: two artists were removed; only second slot remains.
	m.favorites.artists = []string{}
	m.undo.slot = undoSlot{
		kind:              undoKindRemoveFavorite,
		expiresAt:         time.Now().Add(undoWindowDuration),
		removeFavType:     "artist",
		removeFavValue:    "Coltrane", // second removal (supersedes first)
		removeFavIdx:      0,
		removeFavLevel:    favtui.ListLevel,
		removeFavCategory: favtui.CategoryArtists,
	}

	next, _ := m.Update(UndoMsg{})
	nm := next.(Model)

	// Only "Coltrane" should have been restored.
	if len(w.added) != 1 {
		t.Fatalf("expected exactly 1 AddFavorite call, got %d: %v", len(w.added), w.added)
	}
	if w.added[0].value != "Coltrane" {
		t.Fatalf("expected AddFavorite for 'Coltrane', got %q", w.added[0].value)
	}
	// Slot must be cleared — "Bach" cannot be undone.
	if nm.undo.slot.kind != undoNone {
		t.Fatal("undo slot must be cleared after successful undo")
	}
}
