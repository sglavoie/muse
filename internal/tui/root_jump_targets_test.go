package tui

import (
	"context"
	"fmt"
	"testing"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	gentui "github.com/sglavoie/muse/internal/tui/genres"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
)

// TestJumpToArtist verifies that jumpToArtist switches to the Artists view
// with the cursor pre-positioned on the named artist.
func TestJumpToArtist(t *testing.T) {
	m := newModelWithTracks()
	// DeriveArtists: ["Alice"(0), "Bob"(1), "Charlie"(2)]
	m = m.jumpToArtist("Bob")

	if m.activeView != viewArtists {
		t.Errorf("activeView: want viewArtists(%d), got %d", viewArtists, m.activeView)
	}
	if m.artists.level != arttui.ArtistLevel {
		t.Errorf("level: want ArtistLevel, got %v", m.artists.level)
	}
	if m.artists.cursor != 1 {
		t.Errorf("cursor: want 1 (Bob), got %d", m.artists.cursor)
	}
	if len(m.artists.artists) != 3 {
		t.Errorf("artists list: want 3, got %d", len(m.artists.artists))
	}
	if m.artists.artists[1] != "Bob" {
		t.Errorf("artists[1]: want %q, got %q", "Bob", m.artists.artists[1])
	}
}

// TestJumpToArtist_First verifies cursor positioning at the start of the list.
func TestJumpToArtist_First(t *testing.T) {
	m := newModelWithTracks()
	m = m.jumpToArtist("Alice")

	if m.artists.cursor != 0 {
		t.Errorf("cursor: want 0 (Alice), got %d", m.artists.cursor)
	}
}

// TestJumpToArtist_NotFound verifies that jumpToArtist is a no-op when the
// artist does not exist in the library.
func TestJumpToArtist_NotFound(t *testing.T) {
	m := newModelWithTracks()
	original := m.activeView
	m = m.jumpToArtist("Nobody")

	if m.activeView != original {
		t.Errorf("no-op expected: activeView changed to %d", m.activeView)
	}
}

// TestJumpToArtist_UnknownArtist verifies that an empty artist string is mapped
// to the UnknownArtist sentinel so it can still be located in the derived list.
func TestJumpToArtist_UnknownArtist(t *testing.T) {
	tracks := []library.Track{
		{Artist: "", Album: "Mystery", Title: "Anonymous Track", Hash: "h1", Path: "/a.flac"},
		{Artist: "Bob", Album: "Gamma", Title: "Bob Track", Hash: "h2", Path: "/b.flac"},
	}
	m := New(context.Background(), WithLibraryTracks(tracks))
	// LibraryIndex sorts unknown artists after named artists.
	// DeriveArtists preserves that order: ["Bob", "(unknown artist)"]
	m = m.jumpToArtist("")

	if m.activeView != viewArtists {
		t.Errorf("activeView: want viewArtists, got %d", m.activeView)
	}
	if m.artists.cursor != 1 {
		t.Errorf("cursor: want 1 (%s), got %d", arttui.UnknownArtist, m.artists.cursor)
	}
	if len(m.artists.artists) < 2 || m.artists.artists[1] != arttui.UnknownArtist {
		t.Errorf("artists[1]: want %q, got %q", arttui.UnknownArtist, m.artists.artists[1])
	}
}

// TestJumpToArtist_CentersScroll verifies that the scroll offset is computed
// to center the target row within the visible area.
func TestJumpToArtist_CentersScroll(t *testing.T) {
	tracks := make([]library.Track, 20)
	for i := range tracks {
		tracks[i] = library.Track{
			Artist: fmt.Sprintf("Artist%02d", i),
			Album:  "Album",
			Title:  fmt.Sprintf("Track%d", i),
			Hash:   fmt.Sprintf("h%d", i),
			Path:   fmt.Sprintf("/t%d.flac", i),
		}
	}
	m := New(context.Background(), WithLibraryTracks(tracks))
	m.artists.height = 10

	m = m.jumpToArtist("Artist15")

	if m.artists.cursor != 15 {
		t.Errorf("cursor: want 15, got %d", m.artists.cursor)
	}
	wantScroll := max(0, 15-m.artists.height/2)
	if m.artists.scrollOffset != wantScroll {
		t.Errorf("scrollOffset: want %d, got %d", wantScroll, m.artists.scrollOffset)
	}
}

// TestJumpToAlbum verifies that jumpToAlbum switches to the Albums view with
// the cursor pre-positioned on the named album.
func TestJumpToAlbum(t *testing.T) {
	m := newModelWithTracks()
	// DeriveAllAlbums (sorted): ["Alpha"(0), "Beta"(1), "Delta"(2), "Gamma"(3)]
	m = m.jumpToAlbum("Gamma")

	if m.activeView != viewAlbums {
		t.Errorf("activeView: want viewAlbums(%d), got %d", viewAlbums, m.activeView)
	}
	if m.albums.level != albtui.AlbumLevel {
		t.Errorf("level: want AlbumLevel, got %v", m.albums.level)
	}
	if m.albums.cursor != 3 {
		t.Errorf("cursor: want 3 (Gamma), got %d", m.albums.cursor)
	}
	if len(m.albums.albums) != 4 {
		t.Errorf("albums list: want 4, got %d", len(m.albums.albums))
	}
	if m.albums.albums[3] != "Gamma" {
		t.Errorf("albums[3]: want %q, got %q", "Gamma", m.albums.albums[3])
	}
}

// TestJumpToAlbum_First verifies cursor positioning at the start of the list.
func TestJumpToAlbum_First(t *testing.T) {
	m := newModelWithTracks()
	m = m.jumpToAlbum("Alpha")

	if m.albums.cursor != 0 {
		t.Errorf("cursor: want 0 (Alpha), got %d", m.albums.cursor)
	}
}

// TestJumpToAlbum_NotFound verifies that jumpToAlbum is a no-op when the
// album does not exist in the library.
func TestJumpToAlbum_NotFound(t *testing.T) {
	m := newModelWithTracks()
	original := m.activeView
	m = m.jumpToAlbum("Nonexistent")

	if m.activeView != original {
		t.Errorf("no-op expected: activeView changed to %d", m.activeView)
	}
}

// TestJumpToAlbum_UnknownAlbum verifies that an empty album string is mapped
// to the UnknownAlbum sentinel so it can be located in the derived list.
func TestJumpToAlbum_UnknownAlbum(t *testing.T) {
	tracks := []library.Track{
		{Artist: "Alice", Album: "", Title: "No Album Track", Hash: "h1", Path: "/a.flac"},
		{Artist: "Bob", Album: "Gamma", Title: "Bob Track", Hash: "h2", Path: "/b.flac"},
	}
	m := New(context.Background(), WithLibraryTracks(tracks))
	// DeriveAllAlbums sorts unknown albums after named ones: ["Gamma", "(unknown album)"]
	m = m.jumpToAlbum("")

	if m.activeView != viewAlbums {
		t.Errorf("activeView: want viewAlbums, got %d", m.activeView)
	}
	if m.albums.cursor != 1 {
		t.Errorf("cursor: want 1 (%s), got %d", arttui.UnknownAlbum, m.albums.cursor)
	}
	if len(m.albums.albums) < 2 || m.albums.albums[1] != arttui.UnknownAlbum {
		t.Errorf("albums[1]: want %q, got %q", arttui.UnknownAlbum, m.albums.albums[1])
	}
}

// TestJumpToAlbum_CentersScroll verifies the scroll offset centers the row.
func TestJumpToAlbum_CentersScroll(t *testing.T) {
	tracks := make([]library.Track, 20)
	for i := range tracks {
		tracks[i] = library.Track{
			Artist: "Artist",
			Album:  fmt.Sprintf("Album%02d", i),
			Title:  fmt.Sprintf("Track%d", i),
			Hash:   fmt.Sprintf("h%d", i),
			Path:   fmt.Sprintf("/t%d.flac", i),
		}
	}
	m := New(context.Background(), WithLibraryTracks(tracks))
	m.albums.height = 10

	m = m.jumpToAlbum("Album15")

	if m.albums.cursor != 15 {
		t.Errorf("cursor: want 15, got %d", m.albums.cursor)
	}
	wantScroll := max(0, 15-m.albums.height/2)
	if m.albums.scrollOffset != wantScroll {
		t.Errorf("scrollOffset: want %d, got %d", wantScroll, m.albums.scrollOffset)
	}
}

// TestJumpToArtist_EmptyArtistViaAction verifies that pressing the jump key on
// a track with an empty artist field jumps to the "(unknown artist)" row in the
// Artists view rather than being a no-op.
func TestJumpToArtist_EmptyArtistViaAction(t *testing.T) {
	tracks := []library.Track{
		{Artist: "", Album: "Mystery", Title: "Anonymous", Hash: "h1", Path: "/a.flac"},
		{Artist: "Bob", Album: "Gamma", Title: "Bob Track", Hash: "h2", Path: "/b.flac"},
	}
	m := New(context.Background(), WithLibraryTracks(tracks))
	// Library index sorts unknown artists after named ones, so cursor 1 is
	// the track with Artist=="".
	m.library.cursor = 1

	artist, _, ok := m.focusedTrackMeta()
	if !ok {
		t.Fatal("expected focused track to be found")
	}
	if artist != "" {
		t.Fatalf("expected empty artist, got %q", artist)
	}
	m = m.jumpToArtist(artist)

	if m.activeView != viewArtists {
		t.Errorf("activeView: want viewArtists, got %d", m.activeView)
	}
	if m.artists.artists[m.artists.cursor] != arttui.UnknownArtist {
		t.Errorf("cursor on %q, want %q", m.artists.artists[m.artists.cursor], arttui.UnknownArtist)
	}
}

// TestJumpToAlbum_EmptyAlbumViaAction verifies that pressing the jump key on
// a track with an empty album field jumps to the "(unknown album)" row in the
// Albums view rather than being a no-op.
func TestJumpToAlbum_EmptyAlbumViaAction(t *testing.T) {
	tracks := []library.Track{
		{Artist: "Alice", Album: "", Title: "No Album", Hash: "h1", Path: "/a.flac"},
		{Artist: "Bob", Album: "Gamma", Title: "Bob Track", Hash: "h2", Path: "/b.flac"},
	}
	m := New(context.Background(), WithLibraryTracks(tracks))
	// Library index sorts unknown albums after named ones within the same
	// artist. "Alice" (with empty album) sorts before "Bob" alphabetically,
	// so Alice's track is at index 0.
	m.library.cursor = 0

	_, album, ok := m.focusedTrackMeta()
	if !ok {
		t.Fatal("expected focused track to be found")
	}
	if album != "" {
		t.Fatalf("expected empty album, got %q", album)
	}
	m = m.jumpToAlbum(album)

	if m.activeView != viewAlbums {
		t.Errorf("activeView: want viewAlbums, got %d", m.activeView)
	}
	if m.albums.albums[m.albums.cursor] != arttui.UnknownAlbum {
		t.Errorf("cursor on %q, want %q", m.albums.albums[m.albums.cursor], arttui.UnknownAlbum)
	}
}

func TestJumpToGenre(t *testing.T) {
	tracks := []library.Track{
		{Artist: "Alice", Album: "Alpha", Genre: "Rock", Title: "T1", Hash: "h1", Path: "/a.flac"},
		{Artist: "Bob", Album: "Beta", Genre: "Jazz", Title: "T2", Hash: "h2", Path: "/b.flac"},
	}
	m := New(context.Background(), WithLibraryTracks(tracks))
	m = m.jumpToGenre("Jazz")

	if m.activeView != viewGenres {
		t.Errorf("activeView: want viewGenres(%d), got %d", viewGenres, m.activeView)
	}
	if m.genres.level != gentui.GenreLevel {
		t.Errorf("level: want GenreLevel, got %v", m.genres.level)
	}
	// Genres are sorted alphabetically: Jazz, Rock
	if m.genres.cursor != 0 {
		t.Errorf("cursor: want 0 (Jazz), got %d", m.genres.cursor)
	}
}

func TestJumpToGenre_NotFound(t *testing.T) {
	m := newModelWithTracks()
	original := m.activeView
	m = m.jumpToGenre("Nonexistent")
	if m.activeView != original {
		t.Errorf("no-op expected: activeView changed to %d", m.activeView)
	}
}

func TestJumpToGenre_UnknownGenre(t *testing.T) {
	tracks := []library.Track{
		{Artist: "Alice", Album: "Alpha", Genre: "", Title: "T1", Hash: "h1", Path: "/a.flac"},
		{Artist: "Bob", Album: "Beta", Genre: "Rock", Title: "T2", Hash: "h2", Path: "/b.flac"},
	}
	m := New(context.Background(), WithLibraryTracks(tracks))
	m = m.jumpToGenre("")

	if m.activeView != viewGenres {
		t.Errorf("activeView: want viewGenres, got %d", m.activeView)
	}
	// "(unknown genre)" sorts after "Rock"
	if m.genres.cursor != 1 {
		t.Errorf("cursor: want 1 (unknown genre), got %d", m.genres.cursor)
	}
}

func TestJumpToPlaylist(t *testing.T) {
	m := newModelWithTracks()
	m.playlists.playlists = []db.PlaylistRow{
		{ID: 1, Name: "Alpha"},
		{ID: 2, Name: "Beta"},
		{ID: 3, Name: "Gamma"},
	}
	m = m.jumpToPlaylist("Beta")

	if m.activeView != viewPlaylists {
		t.Errorf("activeView: want viewPlaylists(%d), got %d", viewPlaylists, m.activeView)
	}
	if m.playlists.level != pltui.ListLevel {
		t.Errorf("level: want ListLevel, got %v", m.playlists.level)
	}
	if m.playlists.cursor != 1 {
		t.Errorf("cursor: want 1 (Beta), got %d", m.playlists.cursor)
	}
}

func TestJumpToPlaylist_NotFound(t *testing.T) {
	m := newModelWithTracks()
	m.playlists.playlists = []db.PlaylistRow{{ID: 1, Name: "Alpha"}}
	original := m.activeView
	m = m.jumpToPlaylist("Nonexistent")
	if m.activeView != original {
		t.Errorf("no-op expected: activeView changed to %d", m.activeView)
	}
}

// TestJumpToArtist_ExitsVisualMode verifies that visual mode is cleared when
// jumping to an artist so that stale visual state from the source view does
// not bleed into the target view.
func TestJumpToArtist_ExitsVisualMode(t *testing.T) {
	m := newModelWithTracks()
	m.activeView = viewLibrary
	m = enterVisualMode(m)
	if !m.visual.active {
		t.Fatal("precondition: visual mode should be active")
	}

	m = m.jumpToArtist("Bob")

	if m.visual.active {
		t.Error("visual mode should be cleared after jumpToArtist")
	}
}

// TestJumpToAlbum_ExitsVisualMode verifies that visual mode is cleared when
// jumping to an album.
func TestJumpToAlbum_ExitsVisualMode(t *testing.T) {
	m := newModelWithTracks()
	m.activeView = viewLibrary
	m = enterVisualMode(m)
	if !m.visual.active {
		t.Fatal("precondition: visual mode should be active")
	}

	m = m.jumpToAlbum("Alpha")

	if m.visual.active {
		t.Error("visual mode should be cleared after jumpToAlbum")
	}
}

// TestJumpToGenre_ExitsVisualMode verifies that visual mode is cleared when
// jumping to a genre.
func TestJumpToGenre_ExitsVisualMode(t *testing.T) {
	tracks := []library.Track{
		{Artist: "Alice", Album: "Alpha", Genre: "Rock", Title: "T1", Hash: "h1", Path: "/a.flac"},
		{Artist: "Bob", Album: "Beta", Genre: "Jazz", Title: "T2", Hash: "h2", Path: "/b.flac"},
	}
	m := New(context.Background(), WithLibraryTracks(tracks))
	m.activeView = viewLibrary
	m = enterVisualMode(m)
	if !m.visual.active {
		t.Fatal("precondition: visual mode should be active")
	}

	m = m.jumpToGenre("Jazz")

	if m.visual.active {
		t.Error("visual mode should be cleared after jumpToGenre")
	}
}

// TestJumpToPlaylist_ExitsVisualMode verifies that visual mode is cleared when
// jumping to a playlist.
func TestJumpToPlaylist_ExitsVisualMode(t *testing.T) {
	m := newModelWithTracks()
	m.playlists.playlists = []db.PlaylistRow{
		{ID: 1, Name: "Alpha"},
		{ID: 2, Name: "Beta"},
	}
	m.activeView = viewLibrary
	m = enterVisualMode(m)
	if !m.visual.active {
		t.Fatal("precondition: visual mode should be active")
	}

	m = m.jumpToPlaylist("Beta")

	if m.visual.active {
		t.Error("visual mode should be cleared after jumpToPlaylist")
	}
}
