package tui

import (
	"context"
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/library"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
)

// TestUndoBulkRemoveFavorites verifies that undoing a bulk removal of 3 tracks
// (at original indices 1, 2, 4) re-inserts them at their correct positions.
func TestUndoBulkRemoveFavorites(t *testing.T) {
	w := &stubFavoriteWriter{}
	r := &stubFavoriteReader{favorites: map[string][]string{
		"track": {"h0", "h3"},
	}}
	m := New(context.Background(), WithFavoriteReader(r), WithFavoriteWriter(w))
	m.ActiveView = viewFavorites
	m.favorites.Level = favtui.ListLevel
	m.favorites.Category = favtui.CategoryTracks

	// After removing 3 tracks (original indices 1, 2, 4), only h0 and h3 remain.
	t0 := library.Track{Hash: "h0", Title: "T0"}
	t3 := library.Track{Hash: "h3", Title: "T3"}
	m.favorites.Tracks = []library.Track{t0, t3}
	m.favorites.TrackCount = 2

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

	if len(nm.favorites.Tracks) != 5 {
		t.Fatalf("expected 5 tracks after undo, got %d: %v", len(nm.favorites.Tracks), nm.favorites.Tracks)
	}
	expected := []string{"h0", "h1", "h2", "h3", "h4"}
	for i, h := range expected {
		if nm.favorites.Tracks[i].Hash != h {
			t.Errorf("index %d: got hash %q, want %q", i, nm.favorites.Tracks[i].Hash, h)
		}
	}
	if len(w.added) != 3 {
		t.Fatalf("expected 3 AddFavorite calls, got %d: %v", len(w.added), w.added)
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
		ActiveView: viewFavorites,
		favorites: favtui.FavoritesView{
			Level:    favtui.ListLevel,
			Category: favtui.CategoryArtists,
			Artists:  []string{"A", "C", "E"},
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
	if len(m.favorites.Artists) != len(want) {
		t.Fatalf("got %d artists, want %d", len(m.favorites.Artists), len(want))
	}
	for i, w := range want {
		if m.favorites.Artists[i] != w {
			t.Errorf("artists[%d] = %q, want %q", i, m.favorites.Artists[i], w)
		}
	}
}

// TestBulkFavoritesUndoReinsertionOrder_Unsorted verifies that without sorting,
// the undo produces incorrect results (validating the fix is necessary).
func TestBulkFavoritesUndoReinsertionOrder_Unsorted(t *testing.T) {
	m := Model{
		appCtx:     context.Background(),
		ActiveView: viewFavorites,
		favorites: favtui.FavoritesView{
			Level:    favtui.ListLevel,
			Category: favtui.CategoryArtists,
			Artists:  []string{"A", "C", "E"},
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
	isCorrect := len(m.favorites.Artists) == len(correct)
	if isCorrect {
		for i, w := range correct {
			if m.favorites.Artists[i] != w {
				isCorrect = false
				break
			}
		}
	}
	if isCorrect {
		t.Error("expected incorrect result with unsorted indices, but got correct result")
	}
}
