package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/library"
)

func TestToggleFavorite_HistorySingleOpensPicker(t *testing.T) {
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

	nm := openFavoritePickerForTest(t, m)
	if got := nm.overlay.favoriteTargetPicker.tracks[0].Hash; got != "h1" {
		t.Errorf("picker track hash = %q, want h1", got)
	}
}

func TestToggleFavorite_HistoryBulkOpensPicker(t *testing.T) {
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

	nm := openFavoritePickerForTest(t, m)
	if len(nm.library.selectedHashes) != 2 {
		t.Fatal("opening picker should preserve selectedHashes")
	}
	if got := len(nm.overlay.favoriteTargetPicker.tracks); got != 2 {
		t.Errorf("picker tracks = %d, want 2", got)
	}
}
