package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/library"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
)

func TestToggleFavorite_AlbumsTrackLevelBulk(t *testing.T) {
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

	nm, cmd, handled := m.handleToggleFavorite()
	if !handled {
		t.Fatal("handleToggleFavorite should handle viewAlbums at TrackLevel with bulk selection")
	}
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}
	if nm.browse.albums.selectedHashes != nil {
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

func TestToggleFavorite_AlbumsTrackLevelSingle(t *testing.T) {
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

	_, cmd, handled := m.handleToggleFavorite()
	if !handled {
		t.Fatal("handleToggleFavorite should handle viewAlbums at TrackLevel")
	}
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}
	msg := cmd().(favoriteToggledMsg)
	if msg.typ != "track" {
		t.Errorf("expected typ='track', got %q", msg.typ)
	}
	if msg.value != "b1" {
		t.Errorf("expected value='b1', got %q", msg.value)
	}
}
