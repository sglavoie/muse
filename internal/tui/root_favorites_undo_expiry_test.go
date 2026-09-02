package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	favtui "github.com/sglavoie/muse/internal/tui/favorites"
)

// TestUndoRemoveFavorite_Expired verifies that UndoMsg after the window has
// expired shows "Nothing to undo" and does not call AddFavorite.
func TestUndoRemoveFavorite_Expired(t *testing.T) {
	w := &stubFavoriteWriter{}
	m := New(context.Background(), WithFavoriteWriter(w))
	m.ActiveView = viewFavorites
	m.favorites.Level = favtui.ListLevel
	m.favorites.Category = favtui.CategoryArtists
	m.favorites.Artists = []string{"Coltrane"}

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
	if len(nm.favorites.Artists) != 1 {
		t.Fatalf("artists list must be unchanged, got %v", nm.favorites.Artists)
	}
	if !strings.Contains(nm.statusBar.TopNotification().Msg, "Nothing to undo") {
		t.Errorf("expected 'Nothing to undo' notification, got %q", nm.statusBar.TopNotification().Msg)
	}
}

// TestUndoRemoveFavorite_Superseded verifies that performing two consecutive
// removals only allows undoing the second (most recent) one.
func TestUndoRemoveFavorite_Superseded(t *testing.T) {
	w := &stubFavoriteWriter{}
	r := &stubFavoriteReader{favorites: map[string][]string{"artist": {}}}
	m := New(context.Background(), WithFavoriteReader(r), WithFavoriteWriter(w))
	m.ActiveView = viewFavorites
	m.favorites.Level = favtui.ListLevel
	m.favorites.Category = favtui.CategoryArtists

	m.favorites.Artists = []string{}
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
