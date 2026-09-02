package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/browse"
	gentui "github.com/sglavoie/muse/internal/tui/genres"
)

// TestToggleFavorite_GenresArtistLevel verifies that pressing 'F' at Genres
// ArtistLevel calls toggleFavoriteSingle with type "artist" and the focused
// artist name.
func TestToggleFavorite_GenresArtistLevel(t *testing.T) {
	w := &stubFavoriteWriter{}
	r := &stubFavoriteReader{favorites: map[string][]string{
		"artist": {},
	}}
	m := New(context.Background(), WithFavoriteReader(r), WithFavoriteWriter(w))
	m.ActiveView = viewBrowse
	m.browse.Mode = browse.BrowseModeGenres
	m.browse.Genres.Level = gentui.ArtistLevel
	m.browse.Genres.Artists = []string{"Bach", "Coltrane", "Davis"}
	m.browse.Genres.Cursor = 1

	_, cmd, handled := m.handleToggleFavorite()
	if !handled {
		t.Fatal("handleToggleFavorite should handle viewGenres at ArtistLevel")
	}
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}

	msg := cmd().(favoriteToggledMsg)
	if msg.typ != "artist" {
		t.Errorf("expected typ='artist', got %q", msg.typ)
	}
	if msg.value != "Coltrane" {
		t.Errorf("expected value='Coltrane', got %q", msg.value)
	}
	if !msg.added {
		t.Error("expected added=true (Coltrane was not a favorite)")
	}
	if len(w.added) != 1 || w.added[0].typ != "artist" || w.added[0].value != "Coltrane" {
		t.Fatalf("expected AddFavorite(artist, Coltrane), got %v", w.added)
	}
}

// TestToggleFavorite_GenresTrackLevelSingle verifies that pressing 'F' at
// Genres TrackLevel opens the picker for the focused track.
func TestToggleFavorite_GenresTrackLevelSingleOpensPicker(t *testing.T) {
	w := &stubFavoriteWriter{}
	r := &stubFavoriteReader{favorites: map[string][]string{
		"track": {},
	}}
	m := New(context.Background(), WithFavoriteReader(r), WithFavoriteWriter(w))
	m.ActiveView = viewBrowse
	m.browse.Mode = browse.BrowseModeGenres
	m.browse.Genres.Level = gentui.TrackLevel
	m.browse.Genres.Tracks = []library.Track{
		{Hash: "h1", Title: "Song A"},
		{Hash: "h2", Title: "Song B"},
	}
	m.browse.Genres.Cursor = 0

	nm := openFavoritePickerForTest(t, m)
	if got := nm.overlay.favoriteTargetPicker.Tracks[0].Hash; got != "h1" {
		t.Errorf("picker track hash = %q, want h1", got)
	}
}

// TestToggleFavorite_GenresTrackLevelBulk verifies that pressing 'F' at
// Genres TrackLevel opens the picker for the selected tracks without clearing
// the selection.
func TestToggleFavorite_GenresTrackLevelBulkOpensPicker(t *testing.T) {
	w := &stubFavoriteWriter{}
	r := &stubFavoriteReader{favorites: map[string][]string{
		"track": {},
	}}
	m := New(context.Background(), WithFavoriteReader(r), WithFavoriteWriter(w))
	m.ActiveView = viewBrowse
	m.browse.Mode = browse.BrowseModeGenres
	m.browse.Genres.Level = gentui.TrackLevel
	m.browse.Genres.Tracks = []library.Track{
		{Hash: "h1", Title: "Song A"},
		{Hash: "h2", Title: "Song B"},
		{Hash: "h3", Title: "Song C"},
	}
	m.browse.Genres.SelectedHashes = map[string]struct{}{
		"h1": {},
		"h3": {},
	}

	nm := openFavoritePickerForTest(t, m)
	if len(nm.browse.Genres.SelectedHashes) != 2 {
		t.Fatal("opening picker should preserve selectedHashes")
	}
	if got := len(nm.overlay.favoriteTargetPicker.Tracks); got != 2 {
		t.Fatalf("picker tracks = %d, want 2", got)
	}
	if len(w.added) != 0 {
		t.Fatal("opening picker should not write favorites")
	}
}

// TestToggleFavorite_GenresGenreLevelNoop verifies that pressing 'F' at
// Genres GenreLevel does nothing (no "genre" favorite type exists).
func TestToggleFavorite_GenresGenreLevelNoop(t *testing.T) {
	w := &stubFavoriteWriter{}
	r := &stubFavoriteReader{favorites: map[string][]string{}}
	m := New(context.Background(), WithFavoriteReader(r), WithFavoriteWriter(w))
	m.ActiveView = viewBrowse
	m.browse.Mode = browse.BrowseModeGenres
	m.browse.Genres.Level = gentui.GenreLevel
	m.browse.Genres.Genres = []string{"Jazz", "Rock"}
	m.browse.Genres.Cursor = 0

	_, cmd, handled := m.handleToggleFavorite()
	if !handled {
		t.Fatal("handleToggleFavorite should still return handled=true (falls through to default)")
	}
	if cmd != nil {
		t.Fatal("expected nil cmd at GenreLevel (no genre favorite type)")
	}
	if len(w.added) != 0 {
		t.Fatal("no AddFavorite should be called at GenreLevel")
	}
}
