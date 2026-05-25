package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/library"
)

func TestToggleFavorite_HistorySingle(t *testing.T) {
	w := &stubFavoriteWriter{}
	r := &stubFavoriteReader{favorites: map[string][]string{"track": {}}}
	m := New(context.Background(), WithFavoriteReader(r), WithFavoriteWriter(w))
	m.library.mode = LibraryModeHistory
	m.activeView = viewLibrary
	m.library.sourceTracks = []library.Track{
		{Hash: "h1", Title: "Song A"},
		{Hash: "h2", Title: "Song B"},
	}
	m.library = m.library.recomputeSortedTracks()
	m.library.cursor = 0

	_, cmd, handled := m.handleToggleFavorite()
	if !handled {
		t.Fatal("handleToggleFavorite should handle viewLibrary (history mode)")
	}
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}
	msg := cmd().(favoriteToggledMsg)
	if msg.typ != "track" {
		t.Errorf("expected typ='track', got %q", msg.typ)
	}
	if !msg.added {
		t.Error("expected added=true")
	}
}

func TestToggleFavorite_HistoryBulk(t *testing.T) {
	w := &stubFavoriteWriter{}
	r := &stubFavoriteReader{favorites: map[string][]string{"track": {}}}
	m := New(context.Background(), WithFavoriteReader(r), WithFavoriteWriter(w))
	m.library.mode = LibraryModeHistory
	m.activeView = viewLibrary
	m.library.sourceTracks = []library.Track{
		{Hash: "h1", Title: "Song A"},
		{Hash: "h2", Title: "Song B"},
	}
	m.library = m.library.recomputeSortedTracks()
	m.library.selectedHashes = map[string]struct{}{"h1": {}, "h2": {}}

	nm, cmd, handled := m.handleToggleFavorite()
	if !handled {
		t.Fatal("handleToggleFavorite should handle viewLibrary (history mode) with bulk selection")
	}
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}
	if nm.library.selectedHashes != nil {
		t.Fatal("selectedHashes should be nil after bulk toggle")
	}
	msg := cmd().(favoriteToggledMsg)
	if msg.count != 2 {
		t.Errorf("expected count=2, got %d", msg.count)
	}
}
