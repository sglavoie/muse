package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	gentui "github.com/sglavoie/muse/internal/tui/genres"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
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
	m.activeView = viewGenres
	m.genres.level = gentui.ArtistLevel
	m.genres.artists = []string{"Bach", "Coltrane", "Davis"}
	m.genres.cursor = 1

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
// Genres TrackLevel without bulk selection calls toggleFavoriteSingle with
// type "track" and the focused track hash.
func TestToggleFavorite_GenresTrackLevelSingle(t *testing.T) {
	w := &stubFavoriteWriter{}
	r := &stubFavoriteReader{favorites: map[string][]string{
		"track": {},
	}}
	m := New(context.Background(), WithFavoriteReader(r), WithFavoriteWriter(w))
	m.activeView = viewGenres
	m.genres.level = gentui.TrackLevel
	m.genres.tracks = []library.Track{
		{Hash: "h1", Title: "Song A"},
		{Hash: "h2", Title: "Song B"},
	}
	m.genres.cursor = 0

	_, cmd, handled := m.handleToggleFavorite()
	if !handled {
		t.Fatal("handleToggleFavorite should handle viewGenres at TrackLevel")
	}
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}

	msg := cmd().(favoriteToggledMsg)
	if msg.typ != "track" {
		t.Errorf("expected typ='track', got %q", msg.typ)
	}
	if msg.value != "h1" {
		t.Errorf("expected value='h1', got %q", msg.value)
	}
	if !msg.added {
		t.Error("expected added=true")
	}
}

// TestToggleFavorite_GenresTrackLevelBulk verifies that pressing 'F' at
// Genres TrackLevel with bulk selection calls toggleFavoriteBulk with type
// "track" and all selected hashes, and clears selectedHashes.
func TestToggleFavorite_GenresTrackLevelBulk(t *testing.T) {
	w := &stubFavoriteWriter{}
	r := &stubFavoriteReader{favorites: map[string][]string{
		"track": {},
	}}
	m := New(context.Background(), WithFavoriteReader(r), WithFavoriteWriter(w))
	m.activeView = viewGenres
	m.genres.level = gentui.TrackLevel
	m.genres.tracks = []library.Track{
		{Hash: "h1", Title: "Song A"},
		{Hash: "h2", Title: "Song B"},
		{Hash: "h3", Title: "Song C"},
	}
	m.genres.selectedHashes = map[string]struct{}{
		"h1": {},
		"h3": {},
	}

	nm, cmd, handled := m.handleToggleFavorite()
	if !handled {
		t.Fatal("handleToggleFavorite should handle viewGenres at TrackLevel with bulk selection")
	}
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}
	if nm.genres.selectedHashes != nil {
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

// TestToggleFavorite_GenresGenreLevelNoop verifies that pressing 'F' at
// Genres GenreLevel does nothing (no "genre" favorite type exists).
func TestToggleFavorite_GenresGenreLevelNoop(t *testing.T) {
	w := &stubFavoriteWriter{}
	r := &stubFavoriteReader{favorites: map[string][]string{}}
	m := New(context.Background(), WithFavoriteReader(r), WithFavoriteWriter(w))
	m.activeView = viewGenres
	m.genres.level = gentui.GenreLevel
	m.genres.genres = []string{"Jazz", "Rock"}
	m.genres.cursor = 0

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

func TestToggleFavorite_HistorySingle(t *testing.T) {
	w := &stubFavoriteWriter{}
	r := &stubFavoriteReader{favorites: map[string][]string{"track": {}}}
	m := New(context.Background(), WithFavoriteReader(r), WithFavoriteWriter(w))
	m.activeView = viewHistory
	m.history.sourceTracks = []library.Track{
		{Hash: "h1", Title: "Song A"},
		{Hash: "h2", Title: "Song B"},
	}
	m.history = m.history.recomputeSortedTracks()
	m.history.cursor = 0

	_, cmd, handled := m.handleToggleFavorite()
	if !handled {
		t.Fatal("handleToggleFavorite should handle viewHistory")
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
	m.activeView = viewHistory
	m.history.sourceTracks = []library.Track{
		{Hash: "h1", Title: "Song A"},
		{Hash: "h2", Title: "Song B"},
	}
	m.history = m.history.recomputeSortedTracks()
	m.history.selectedHashes = map[string]struct{}{"h1": {}, "h2": {}}

	nm, cmd, handled := m.handleToggleFavorite()
	if !handled {
		t.Fatal("handleToggleFavorite should handle viewHistory with bulk selection")
	}
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}
	if nm.history.selectedHashes != nil {
		t.Fatal("selectedHashes should be nil after bulk toggle")
	}
	msg := cmd().(favoriteToggledMsg)
	if msg.count != 2 {
		t.Errorf("expected count=2, got %d", msg.count)
	}
}

func TestToggleFavorite_TagsTrackLevelSingle(t *testing.T) {
	w := &stubFavoriteWriter{}
	r := &stubFavoriteReader{favorites: map[string][]string{"track": {}}}
	m := New(context.Background(), WithFavoriteReader(r), WithFavoriteWriter(w))
	m.activeView = viewTags
	m.tagsView.level = tagtui.TrackLevel
	m.tagsView.tracks = []db.PlaylistTrackRow{
		{Hash: "ht1", Title: "Tag Song A"},
		{Hash: "ht2", Title: "Tag Song B"},
	}
	m.tagsView.cursor = 1

	_, cmd, handled := m.handleToggleFavorite()
	if !handled {
		t.Fatal("handleToggleFavorite should handle viewTags at TrackLevel")
	}
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}
	msg := cmd().(favoriteToggledMsg)
	if msg.typ != "track" {
		t.Errorf("expected typ='track', got %q", msg.typ)
	}
	if msg.value != "ht2" {
		t.Errorf("expected value='ht2', got %q", msg.value)
	}
}

func TestToggleFavorite_TagsTrackLevelBulk(t *testing.T) {
	w := &stubFavoriteWriter{}
	r := &stubFavoriteReader{favorites: map[string][]string{"track": {}}}
	m := New(context.Background(), WithFavoriteReader(r), WithFavoriteWriter(w))
	m.activeView = viewTags
	m.tagsView.level = tagtui.TrackLevel
	m.tagsView.tracks = []db.PlaylistTrackRow{
		{Hash: "ht1", Title: "Tag Song A"},
		{Hash: "ht2", Title: "Tag Song B"},
	}
	m.tagsView.selectedHashes = map[string]struct{}{"ht1": {}, "ht2": {}}

	nm, cmd, handled := m.handleToggleFavorite()
	if !handled {
		t.Fatal("handleToggleFavorite should handle viewTags at TrackLevel with bulk selection")
	}
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}
	if nm.tagsView.selectedHashes != nil {
		t.Fatal("selectedHashes should be nil after bulk toggle")
	}
	msg := cmd().(favoriteToggledMsg)
	if msg.count != 2 {
		t.Errorf("expected count=2, got %d", msg.count)
	}
}

func TestToggleFavorite_TagsTagLevelNoop(t *testing.T) {
	w := &stubFavoriteWriter{}
	r := &stubFavoriteReader{favorites: map[string][]string{}}
	m := New(context.Background(), WithFavoriteReader(r), WithFavoriteWriter(w))
	m.activeView = viewTags
	m.tagsView.level = tagtui.TagLevel

	_, cmd, handled := m.handleToggleFavorite()
	if !handled {
		t.Fatal("handleToggleFavorite should return handled=true")
	}
	if cmd != nil {
		t.Fatal("expected nil cmd at TagLevel")
	}
}

func TestToggleFavorite_ArtistsTrackLevelBulk(t *testing.T) {
	w := &stubFavoriteWriter{}
	r := &stubFavoriteReader{favorites: map[string][]string{"track": {}}}
	m := New(context.Background(), WithFavoriteReader(r), WithFavoriteWriter(w))
	m.activeView = viewArtists
	m.artists.level = arttui.TrackLevel
	m.artists.tracks = []library.Track{
		{Hash: "a1", Title: "Song A"},
		{Hash: "a2", Title: "Song B"},
		{Hash: "a3", Title: "Song C"},
	}
	m.artists.selectedHashes = map[string]struct{}{
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
	if nm.artists.selectedHashes != nil {
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
	m.activeView = viewArtists
	m.artists.level = arttui.TrackLevel
	m.artists.tracks = []library.Track{
		{Hash: "a1", Title: "Song A"},
		{Hash: "a2", Title: "Song B"},
	}
	m.artists.cursor = 1

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

func TestToggleFavorite_AlbumsTrackLevelBulk(t *testing.T) {
	w := &stubFavoriteWriter{}
	r := &stubFavoriteReader{favorites: map[string][]string{"track": {}}}
	m := New(context.Background(), WithFavoriteReader(r), WithFavoriteWriter(w))
	m.activeView = viewAlbums
	m.albums.level = albtui.TrackLevel
	m.albums.tracks = []library.Track{
		{Hash: "b1", Title: "Track 1"},
		{Hash: "b2", Title: "Track 2"},
		{Hash: "b3", Title: "Track 3"},
	}
	m.albums.selectedHashes = map[string]struct{}{
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
	if nm.albums.selectedHashes != nil {
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
	m.activeView = viewAlbums
	m.albums.level = albtui.TrackLevel
	m.albums.tracks = []library.Track{
		{Hash: "b1", Title: "Track 1"},
		{Hash: "b2", Title: "Track 2"},
	}
	m.albums.cursor = 0

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
