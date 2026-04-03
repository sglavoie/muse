package tui

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestUndoAddFavorite_SingleFromLibraryView verifies that undoing a single
// favorite add calls RemoveFavorite and clears the undo slot.
func TestUndoAddFavorite_SingleFromLibraryView(t *testing.T) {
	w := &stubFavoriteWriter{}
	m := New(context.Background(), WithFavoriteWriter(w))
	m.activeView = viewLibrary
	m.favoriteHashes = map[string]struct{}{"hash-1": {}}

	m.undo.slot = undoSlot{
		action:    undoAddFavoriteAction{typ: "track", value: "hash-1"},
		expiresAt: time.Now().Add(undoWindowDuration),
	}

	next, _ := m.Update(UndoMsg{})
	nm := next.(Model)

	if len(w.removed) != 1 || w.removed[0].typ != "track" || w.removed[0].value != "hash-1" {
		t.Fatalf("expected RemoveFavorite(track, hash-1), got %v", w.removed)
	}
	if nm.undo.slot.action != nil {
		t.Fatal("undo slot must be cleared after successful undo")
	}
	if !strings.Contains(nm.statusBar.notification.msg, "removed") {
		t.Errorf("expected notification mentioning 'removed', got %q", nm.statusBar.notification.msg)
	}
}

// TestUndoAddFavorite_TrackHashRemoved verifies that undoing a single track
// add deletes the hash from the in-memory favoriteHashes set.
func TestUndoAddFavorite_TrackHashRemoved(t *testing.T) {
	w := &stubFavoriteWriter{}
	m := New(context.Background(), WithFavoriteWriter(w))
	m.favoriteHashes = map[string]struct{}{"hash-1": {}, "hash-2": {}}

	m.undo.slot = undoSlot{
		action:    undoAddFavoriteAction{typ: "track", value: "hash-1"},
		expiresAt: time.Now().Add(undoWindowDuration),
	}

	next, _ := m.Update(UndoMsg{})
	nm := next.(Model)

	if _, ok := nm.favoriteHashes["hash-1"]; ok {
		t.Fatal("expected hash-1 to be removed from favoriteHashes after undo")
	}
	if _, ok := nm.favoriteHashes["hash-2"]; !ok {
		t.Fatal("expected hash-2 to remain in favoriteHashes")
	}
}

// TestUndoBulkAddFavorites verifies that undoing a bulk favorite add calls
// RemoveFavoritesByTypeAndValues and removes all hashes from favoriteHashes.
func TestUndoBulkAddFavorites(t *testing.T) {
	w := &stubFavoriteWriter{}
	m := New(context.Background(), WithFavoriteWriter(w))
	m.favoriteHashes = map[string]struct{}{"h1": {}, "h2": {}, "h3": {}, "h4": {}}

	m.undo.slot = undoSlot{
		action: undoBulkAddFavoritesAction{
			typ:    "track",
			values: []string{"h1", "h2", "h3"},
		},
		expiresAt: time.Now().Add(undoWindowDuration),
	}

	next, _ := m.Update(UndoMsg{})
	nm := next.(Model)

	if len(w.removed) != 3 {
		t.Fatalf("expected 3 RemoveFavorite calls, got %d: %v", len(w.removed), w.removed)
	}
	for _, v := range []string{"h1", "h2", "h3"} {
		if _, ok := nm.favoriteHashes[v]; ok {
			t.Errorf("expected %s to be removed from favoriteHashes after undo", v)
		}
	}
	if _, ok := nm.favoriteHashes["h4"]; !ok {
		t.Fatal("expected h4 to remain in favoriteHashes")
	}
	if nm.undo.slot.action != nil {
		t.Fatal("undo slot must be cleared after successful undo")
	}
}
