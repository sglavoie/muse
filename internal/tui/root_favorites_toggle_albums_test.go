package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/library"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
)

func TestToggleFavorite_AlbumsTrackLevelBulkOpensPicker(t *testing.T) {
	w := &stubFavoriteWriter{}
	r := &stubFavoriteReader{favorites: map[string][]string{"track": {}}}
	m := New(context.Background(), WithFavoriteReader(r), WithFavoriteWriter(w))
	m.activeView = viewBrowse
	m.browse.mode = BrowseModeAlbums
	m.browse.albums.level = albtui.TrackLevel
	m.browse.albums.tracks = []library.Track{
		{Hash: "b1", Title: "Track 1"},
		{Hash: "b2", Title: "Track 2"},
		{Hash: "b3", Title: "Track 3"},
	}
	m.browse.albums.selectedHashes = map[string]struct{}{
		"b1": {},
		"b2": {},
	}

	nm := openFavoritePickerForTest(t, m)
	if len(nm.browse.albums.selectedHashes) != 2 {
		t.Fatal("opening picker should preserve selectedHashes")
	}
	if got := len(nm.overlay.favoriteTargetPicker.tracks); got != 2 {
		t.Fatalf("picker tracks = %d, want 2", got)
	}
	if len(w.added) != 0 {
		t.Fatal("opening picker should not write favorites")
	}
}

func TestToggleFavorite_AlbumsTrackLevelSingleOpensPicker(t *testing.T) {
	w := &stubFavoriteWriter{}
	r := &stubFavoriteReader{favorites: map[string][]string{"track": {}}}
	m := New(context.Background(), WithFavoriteReader(r), WithFavoriteWriter(w))
	m.activeView = viewBrowse
	m.browse.mode = BrowseModeAlbums
	m.browse.albums.level = albtui.TrackLevel
	m.browse.albums.tracks = []library.Track{
		{Hash: "b1", Title: "Track 1"},
		{Hash: "b2", Title: "Track 2"},
	}
	m.browse.albums.cursor = 0

	nm := openFavoritePickerForTest(t, m)
	if got := nm.overlay.favoriteTargetPicker.tracks[0].Hash; got != "b1" {
		t.Errorf("picker track hash = %q, want b1", got)
	}
}
