package tui

import (
	"testing"

	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
)

// TestFocusedTrackMeta_ArtistsAtEachLevel covers all three Artists drill-down
// levels: ArtistLevel, AlbumLevel, and TrackLevel.
func TestFocusedTrackMeta_ArtistsAtEachLevel(t *testing.T) {
	tracks := sampleLibraryTracks()

	t.Run("ArtistLevel", func(t *testing.T) {
		m := newModelWithTracks()
		m.activeView = viewArtists
		m.artists = ArtistsView{
			level:   arttui.ArtistLevel,
			artists: []string{"Alice", "Bob", "Charlie"},
			cursor:  1, // Bob
		}

		artist, album, _ := m.focusedTrackMeta()
		if artist != "Bob" {
			t.Errorf("artist: want %q, got %q", "Bob", artist)
		}
		if album != "" {
			t.Errorf("album: want empty, got %q", album)
		}
	})

	t.Run("AlbumLevel", func(t *testing.T) {
		m := newModelWithTracks()
		m.activeView = viewArtists
		m.artists = ArtistsView{
			level:          arttui.AlbumLevel,
			selectedArtist: "Alice",
			albums:         []string{"Alpha", "Beta"},
			cursor:         1, // Beta
		}

		artist, album, _ := m.focusedTrackMeta()
		if artist != "Alice" {
			t.Errorf("artist: want %q, got %q", "Alice", artist)
		}
		if album != "Beta" {
			t.Errorf("album: want %q, got %q", "Beta", album)
		}
	})

	t.Run("TrackLevel", func(t *testing.T) {
		m := newModelWithTracks()
		m.activeView = viewArtists
		m.artists = ArtistsView{
			level:  arttui.TrackLevel,
			tracks: tracks[3:5], // Bob/Gamma tracks
			cursor: 0,
		}

		artist, album, _ := m.focusedTrackMeta()
		if artist != "Bob" {
			t.Errorf("artist: want %q, got %q", "Bob", artist)
		}
		if album != "Gamma" {
			t.Errorf("album: want %q, got %q", "Gamma", album)
		}
	})
}

// TestFocusedTrackMeta_AlbumsAtEachLevel covers both Albums drill-down levels.
func TestFocusedTrackMeta_AlbumsAtEachLevel(t *testing.T) {
	tracks := sampleLibraryTracks()

	t.Run("AlbumLevel", func(t *testing.T) {
		m := newModelWithTracks()
		m.activeView = viewAlbums
		m.albums = AlbumsView{
			level:  albtui.AlbumLevel,
			albums: []string{"Alpha", "Gamma"},
			cursor: 1, // Gamma
		}

		artist, album, _ := m.focusedTrackMeta()
		if artist != "" {
			t.Errorf("artist: want empty, got %q", artist)
		}
		if album != "Gamma" {
			t.Errorf("album: want %q, got %q", "Gamma", album)
		}
	})

	t.Run("TrackLevel", func(t *testing.T) {
		m := newModelWithTracks()
		m.activeView = viewAlbums
		m.albums = AlbumsView{
			level:  albtui.TrackLevel,
			tracks: tracks[0:2], // Alice/Alpha tracks
			cursor: 1,
		}

		artist, album, _ := m.focusedTrackMeta()
		if artist != "Alice" {
			t.Errorf("artist: want %q, got %q", "Alice", artist)
		}
		if album != "Alpha" {
			t.Errorf("album: want %q, got %q", "Alpha", album)
		}
	})
}

// TestFocusedTrackMeta_FavoritesAtEachLevel covers all Favorites paths.
func TestFocusedTrackMeta_FavoritesAtEachLevel(t *testing.T) {
	tracks := sampleLibraryTracks()

	t.Run("ListLevel_Artists", func(t *testing.T) {
		m := newModelWithTracks()
		m.activeView = viewFavorites
		m.favorites = FavoritesView{
			level:    favtui.ListLevel,
			category: favtui.CategoryArtists,
			artists:  []string{"Alice", "Bob"},
			cursor:   1,
		}

		artist, album, _ := m.focusedTrackMeta()
		if artist != "Bob" {
			t.Errorf("artist: want %q, got %q", "Bob", artist)
		}
		if album != "" {
			t.Errorf("album: want empty, got %q", album)
		}
	})

	t.Run("ListLevel_Albums", func(t *testing.T) {
		m := newModelWithTracks()
		m.activeView = viewFavorites
		m.favorites = FavoritesView{
			level:    favtui.ListLevel,
			category: favtui.CategoryAlbums,
			albums:   []string{"Alpha", "Gamma"},
			cursor:   1,
		}

		artist, album, _ := m.focusedTrackMeta()
		if artist != "" {
			t.Errorf("artist: want empty, got %q", artist)
		}
		if album != "Gamma" {
			t.Errorf("album: want %q, got %q", "Gamma", album)
		}
	})

	t.Run("ListLevel_Tracks", func(t *testing.T) {
		m := newModelWithTracks()
		m.activeView = viewFavorites
		m.favorites = FavoritesView{
			level:    favtui.ListLevel,
			category: favtui.CategoryTracks,
			tracks:   tracks[3:5], // Bob/Gamma
			cursor:   0,
		}

		artist, album, _ := m.focusedTrackMeta()
		if artist != "Bob" {
			t.Errorf("artist: want %q, got %q", "Bob", artist)
		}
		if album != "Gamma" {
			t.Errorf("album: want %q, got %q", "Gamma", album)
		}
	})

	t.Run("TrackLevel", func(t *testing.T) {
		m := newModelWithTracks()
		m.activeView = viewFavorites
		m.favorites = FavoritesView{
			level:           favtui.TrackLevel,
			drilldownTracks: tracks[5:6], // Charlie/Delta
			cursor:          0,
		}

		artist, album, _ := m.focusedTrackMeta()
		if artist != "Charlie" {
			t.Errorf("artist: want %q, got %q", "Charlie", artist)
		}
		if album != "Delta" {
			t.Errorf("album: want %q, got %q", "Delta", album)
		}
	})
}

// TestFocusedTrackMeta_ArtistsAlbumLevelEmptyAlbums verifies that when the
// album list is empty at AlbumLevel, the artist is still returned but the
// album is empty, and found is true (artist context exists).
func TestFocusedTrackMeta_ArtistsAlbumLevelEmptyAlbums(t *testing.T) {
	m := newModelWithTracks()
	m.activeView = viewArtists
	m.artists = ArtistsView{
		level:          arttui.AlbumLevel,
		selectedArtist: "Alice",
		albums:         nil,
		cursor:         0,
	}

	artist, album, found := m.focusedTrackMeta()
	if !found {
		t.Error("expected found=true when selectedArtist is set")
	}
	if artist != "Alice" {
		t.Errorf("artist: want %q, got %q", "Alice", artist)
	}
	if album != "" {
		t.Errorf("album: want empty, got %q", album)
	}
}

// TestFocusedTrackMeta_ArtistsAlbumLevelNoArtist verifies that when both the
// album list and selectedArtist are empty at AlbumLevel, found is false.
func TestFocusedTrackMeta_ArtistsAlbumLevelNoArtist(t *testing.T) {
	m := newModelWithTracks()
	m.activeView = viewArtists
	m.artists = ArtistsView{
		level:          arttui.AlbumLevel,
		selectedArtist: "",
		albums:         nil,
		cursor:         0,
	}

	_, _, found := m.focusedTrackMeta()
	if found {
		t.Error("expected found=false when both selectedArtist and albums are empty")
	}
}
