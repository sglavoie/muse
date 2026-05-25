package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/library"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
)

func TestToggleFavorite_ArtistsTrackLevelBulk(t *testing.T) {
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

	nm, cmd, handled := m.handleToggleFavorite()
	if !handled {
		t.Fatal("handleToggleFavorite should handle viewArtists at TrackLevel with bulk selection")
	}
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}
	if nm.browse.artists.selectedHashes != nil {
		t.Fatal("selectedHashes should be nil after bulk toggle")
	}

	msg := cmd().(favoriteToggledMsg)
	if msg.typ != "track" {
		t.Errorf("expected typ='track', got %q", msg.typ)
	}
	if msg.count != 2 {
		t.Errorf("expected count=2, got %d", msg.count)
	}
	if len(w.added) != 2 {
		t.Fatalf("expected 2 AddFavorite calls, got %d", len(w.added))
	}
}

func TestToggleFavorite_ArtistsTrackLevelSingle(t *testing.T) {
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

	_, cmd, handled := m.handleToggleFavorite()
	if !handled {
		t.Fatal("handleToggleFavorite should handle viewArtists at TrackLevel")
	}
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}
	msg := cmd().(favoriteToggledMsg)
	if msg.typ != "track" {
		t.Errorf("expected typ='track', got %q", msg.typ)
	}
	if msg.value != "a2" {
		t.Errorf("expected value='a2', got %q", msg.value)
	}
}
