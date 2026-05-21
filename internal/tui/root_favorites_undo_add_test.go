package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/library"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
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
	if !strings.Contains(nm.statusBar.topNotification().msg, "removed") {
		t.Errorf("expected notification mentioning 'removed', got %q", nm.statusBar.topNotification().msg)
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

func TestUndoAddFavorite_TrackLevelKeepsDrilldownTracks(t *testing.T) {
	w := &stubFavoriteWriter{}
	r := &stubFavoriteReader{favorites: map[string][]string{
		"track": {"h2"},
	}}
	m := New(context.Background(), WithFavoriteWriter(w), WithFavoriteReader(r))
	m.activeView = viewFavorites
	m.favorites.level = favtui.TrackLevel
	m.favorites.category = favtui.CategoryAlbums
	m.favorites.drilldownTracks = []library.Track{
		{Hash: "h1", Title: "T1"},
		{Hash: "h2", Title: "T2"},
	}
	m.favoriteHashes = map[string]struct{}{"h1": {}, "h2": {}}

	m.undo.slot = undoSlot{
		action:    undoAddFavoriteAction{typ: "track", value: "h1"},
		expiresAt: time.Now().Add(undoWindowDuration),
	}

	next, _ := m.Update(UndoMsg{})
	nm := next.(Model)

	if len(nm.favorites.drilldownTracks) != 2 {
		t.Fatalf("expected 2 drilldown tracks after undo, got %d", len(nm.favorites.drilldownTracks))
	}
	if nm.favorites.drilldownTracks[0].Hash != "h1" || nm.favorites.drilldownTracks[1].Hash != "h2" {
		t.Fatalf("expected [h1 h2], got [%s %s]", nm.favorites.drilldownTracks[0].Hash, nm.favorites.drilldownTracks[1].Hash)
	}
	if _, ok := nm.favoriteHashes["h1"]; ok {
		t.Fatal("expected h1 to be removed from favoriteHashes after undo")
	}
}

// TestUndoAddFavorite_RemovesFromViewList verifies that undoing a single
// track add while viewing favorites removes the track from the in-memory
// view list so it disappears immediately.
func TestUndoAddFavorite_RemovesFromViewList(t *testing.T) {
	w := &stubFavoriteWriter{}
	r := &stubFavoriteReader{favorites: map[string][]string{
		"track": {"h2"},
	}}
	m := New(context.Background(), WithFavoriteWriter(w), WithFavoriteReader(r))
	m.activeView = viewFavorites
	m.favorites.level = favtui.ListLevel
	m.favorites.category = favtui.CategoryTracks
	m.favorites.tracks = []library.Track{
		{Hash: "h1", Title: "T1"},
		{Hash: "h2", Title: "T2"},
	}
	m.favorites.trackCount = 2
	m.favoriteHashes = map[string]struct{}{"h1": {}, "h2": {}}

	m.undo.slot = undoSlot{
		action:    undoAddFavoriteAction{typ: "track", value: "h1"},
		expiresAt: time.Now().Add(undoWindowDuration),
	}

	next, _ := m.Update(UndoMsg{})
	nm := next.(Model)

	if len(nm.favorites.tracks) != 1 {
		t.Fatalf("expected 1 track after undo add, got %d", len(nm.favorites.tracks))
	}
	if nm.favorites.tracks[0].Hash != "h2" {
		t.Fatalf("expected remaining track h2, got %s", nm.favorites.tracks[0].Hash)
	}
}

// TestUndoAddFavorite_RemovesArtistFromViewList verifies that undoing a single
// artist add while viewing the artists list removes it from the view.
func TestUndoAddFavorite_RemovesArtistFromViewList(t *testing.T) {
	w := &stubFavoriteWriter{}
	r := &stubFavoriteReader{favorites: map[string][]string{
		"artist": {"Bach"},
	}}
	m := New(context.Background(), WithFavoriteWriter(w), WithFavoriteReader(r))
	m.activeView = viewFavorites
	m.favorites.level = favtui.ListLevel
	m.favorites.category = favtui.CategoryArtists
	m.favorites.artists = []string{"Bach", "Davis"}
	m.favorites.artistCount = 2
	m.favoriteArtists = map[string]struct{}{"Bach": {}, "Davis": {}}

	m.undo.slot = undoSlot{
		action:    undoAddFavoriteAction{typ: "artist", value: "Davis"},
		expiresAt: time.Now().Add(undoWindowDuration),
	}

	next, _ := m.Update(UndoMsg{})
	nm := next.(Model)

	if len(nm.favorites.artists) != 1 {
		t.Fatalf("expected 1 artist after undo add, got %d", len(nm.favorites.artists))
	}
	if nm.favorites.artists[0] != "Bach" {
		t.Fatalf("expected remaining artist Bach, got %s", nm.favorites.artists[0])
	}
}

// TestUndoBulkAddFavorites_RemovesFromViewList verifies that undoing a bulk
// track add while viewing favorites removes all tracks from the view.
func TestUndoBulkAddFavorites_RemovesFromViewList(t *testing.T) {
	w := &stubFavoriteWriter{}
	r := &stubFavoriteReader{favorites: map[string][]string{
		"track": {"h2"},
	}}
	m := New(context.Background(), WithFavoriteWriter(w), WithFavoriteReader(r))
	m.activeView = viewFavorites
	m.favorites.level = favtui.ListLevel
	m.favorites.category = favtui.CategoryTracks
	m.favorites.tracks = []library.Track{
		{Hash: "h1", Title: "T1"},
		{Hash: "h2", Title: "T2"},
		{Hash: "h3", Title: "T3"},
	}
	m.favorites.trackCount = 3
	m.favoriteHashes = map[string]struct{}{"h1": {}, "h2": {}, "h3": {}}

	m.undo.slot = undoSlot{
		action: undoBulkAddFavoritesAction{
			typ:    "track",
			values: []string{"h1", "h3"},
		},
		expiresAt: time.Now().Add(undoWindowDuration),
	}

	next, _ := m.Update(UndoMsg{})
	nm := next.(Model)

	if len(nm.favorites.tracks) != 1 {
		t.Fatalf("expected 1 track after undo bulk add, got %d", len(nm.favorites.tracks))
	}
	if nm.favorites.tracks[0].Hash != "h2" {
		t.Fatalf("expected remaining track h2, got %s", nm.favorites.tracks[0].Hash)
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
