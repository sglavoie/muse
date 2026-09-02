package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/library"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
)

func TestToggleFavorite_ArtistsTrackLevelBulkOpensPicker(t *testing.T) {
	w := &stubFavoriteWriter{}
	r := &stubFavoriteReader{favorites: map[string][]string{"track": {}}}
	m := New(context.Background(), WithFavoriteReader(r), WithFavoriteWriter(w))
	m.activeView = viewBrowse
	m.browse.mode = BrowseModeArtists
	m.browse.artists.level = arttui.TrackLevel
	m.browse.artists.tracks = []library.Track{
		{Hash: "a1", Title: "Song A"},
		{Hash: "a2", Title: "Song B"},
		{Hash: "a3", Title: "Song C"},
	}
	m.browse.artists.selectedHashes = map[string]struct{}{
		"a1": {},
		"a3": {},
	}

	nm := openFavoritePickerForTest(t, m)
	if len(nm.browse.artists.selectedHashes) != 2 {
		t.Fatal("opening picker should preserve selectedHashes")
	}
	if got := len(nm.overlay.favoriteTargetPicker.Tracks); got != 2 {
		t.Fatalf("picker tracks = %d, want 2", got)
	}
	if len(w.added) != 0 {
		t.Fatal("opening picker should not write favorites")
	}
}

func TestToggleFavorite_ArtistsTrackLevelSingleOpensPicker(t *testing.T) {
	w := &stubFavoriteWriter{}
	r := &stubFavoriteReader{favorites: map[string][]string{"track": {}}}
	m := New(context.Background(), WithFavoriteReader(r), WithFavoriteWriter(w))
	m.activeView = viewBrowse
	m.browse.mode = BrowseModeArtists
	m.browse.artists.level = arttui.TrackLevel
	m.browse.artists.tracks = []library.Track{
		{Hash: "a1", Title: "Song A"},
		{Hash: "a2", Title: "Song B"},
	}
	m.browse.artists.cursor = 1

	nm := openFavoritePickerForTest(t, m)
	if got := nm.overlay.favoriteTargetPicker.Tracks[0].Hash; got != "a2" {
		t.Errorf("picker track hash = %q, want a2", got)
	}
}
