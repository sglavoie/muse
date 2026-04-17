package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	gentui "github.com/sglavoie/muse/internal/tui/genres"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
)

// TestFocusedTrackMeta_Library verifies that the library cursor drives the
// returned artist and album values.
func TestFocusedTrackMeta_Library(t *testing.T) {
	m := newModelWithTracks()
	// Default active view is viewLibrary, cursor=0 -> Alice/Alpha.
	artist, album, _ := m.focusedTrackMeta()
	if artist != "Alice" {
		t.Errorf("artist: want %q, got %q", "Alice", artist)
	}
	if album != "Alpha" {
		t.Errorf("album: want %q, got %q", "Alpha", album)
	}
}

// TestFocusedTrackMeta_Library_SecondTrack verifies the cursor can be advanced
// to return a different track's metadata.
func TestFocusedTrackMeta_Library_SecondTrack(t *testing.T) {
	m := newModelWithTracks()
	m.library.cursor = 3 // Bob/Gamma/Track G1
	artist, album, _ := m.focusedTrackMeta()
	if artist != "Bob" {
		t.Errorf("artist: want %q, got %q", "Bob", artist)
	}
	if album != "Gamma" {
		t.Errorf("album: want %q, got %q", "Gamma", album)
	}
}

// TestFocusedTrackMeta_Queue verifies that the queue cursor drives the
// returned artist and album values.
func TestFocusedTrackMeta_Queue(t *testing.T) {
	m := newModelWithTracks()
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{
		{Artist: "Bob", Album: "Gamma", Title: "Track G1"},
		{Artist: "Alice", Album: "Alpha", Title: "Track A1"},
	}
	m.queue.cursor = 0

	artist, album, _ := m.focusedTrackMeta()
	if artist != "Bob" {
		t.Errorf("artist: want %q, got %q", "Bob", artist)
	}
	if album != "Gamma" {
		t.Errorf("album: want %q, got %q", "Gamma", album)
	}
}

// TestFocusedTrackMeta_Queue_SecondEntry advances the cursor to a different entry.
func TestFocusedTrackMeta_Queue_SecondEntry(t *testing.T) {
	m := newModelWithTracks()
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{
		{Artist: "Bob", Album: "Gamma", Title: "Track G1"},
		{Artist: "Alice", Album: "Alpha", Title: "Track A1"},
	}
	m.queue.cursor = 1

	artist, album, _ := m.focusedTrackMeta()
	if artist != "Alice" {
		t.Errorf("artist: want %q, got %q", "Alice", artist)
	}
	if album != "Alpha" {
		t.Errorf("album: want %q, got %q", "Alpha", album)
	}
}

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

// TestFocusedTrackMeta_EmptyView verifies that an empty library returns ("","",false).
func TestFocusedTrackMeta_EmptyView(t *testing.T) {
	m := New(context.Background())
	artist, album, found := m.focusedTrackMeta()
	if artist != "" || album != "" {
		t.Errorf("empty library: want (%q,%q), got (%q,%q)", "", "", artist, album)
	}
	if found {
		t.Error("empty library: expected found=false")
	}
}

// TestFocusedTrackMeta_QueueEmpty verifies that an empty queue returns ("","",false).
func TestFocusedTrackMeta_QueueEmpty(t *testing.T) {
	m := newModelWithTracks()
	m.activeView = viewQueue
	artist, album, found := m.focusedTrackMeta()
	if artist != "" || album != "" {
		t.Errorf("empty queue: want (%q,%q), got (%q,%q)", "", "", artist, album)
	}
	if found {
		t.Error("empty queue: expected found=false")
	}
}

// TestFocusedTrackMeta_Found verifies that the found flag is true when a track
// with empty artist/album metadata is focused.
func TestFocusedTrackMeta_Found(t *testing.T) {
	tracks := []library.Track{{Artist: "", Album: "", Title: "No Meta", Hash: "h1", Path: "/a.flac"}}
	m := New(context.Background(), WithLibraryTracks(tracks))
	artist, album, found := m.focusedTrackMeta()
	if !found {
		t.Error("expected found=true for track with empty artist/album")
	}
	if artist != "" {
		t.Errorf("artist: want empty, got %q", artist)
	}
	if album != "" {
		t.Errorf("album: want empty, got %q", album)
	}
}

// TestFocusedTrackMeta_GenresArtistLevel verifies that focusedTrackMeta returns
// the focused artist name when the Genres view is at ArtistLevel.
func TestFocusedTrackMeta_GenresArtistLevel(t *testing.T) {
	m := newModelWithTracks()
	m.activeView = viewGenres
	m.genres = GenresView{
		level:   gentui.ArtistLevel,
		artists: []string{"Alice", "Bob"},
		cursor:  1,
	}
	artist, album, found := m.focusedTrackMeta()
	if !found {
		t.Fatal("expected found=true at Genres ArtistLevel")
	}
	if artist != "Bob" {
		t.Errorf("artist: want %q, got %q", "Bob", artist)
	}
	if album != "" {
		t.Errorf("album: want empty, got %q", album)
	}
}

// TestFocusedTrackMeta_GenresTrackLevel verifies that focusedTrackMeta returns
// artist and album from the focused track when the Genres view is at TrackLevel.
func TestFocusedTrackMeta_GenresTrackLevel(t *testing.T) {
	m := newModelWithTracks()
	m.activeView = viewGenres
	m.genres = GenresView{
		level: gentui.TrackLevel,
		tracks: []library.Track{
			{Artist: "Carol", Album: "Delta", Title: "Song", Hash: "h1"},
		},
		cursor: 0,
	}
	artist, album, found := m.focusedTrackMeta()
	if !found {
		t.Fatal("expected found=true at Genres TrackLevel")
	}
	if artist != "Carol" {
		t.Errorf("artist: want %q, got %q", "Carol", artist)
	}
	if album != "Delta" {
		t.Errorf("album: want %q, got %q", "Delta", album)
	}
}

// TestFocusedTrackMeta_GenresGenreLevel verifies that focusedTrackMeta returns
// not-found at GenreLevel since genres are not artists or albums.
func TestFocusedTrackMeta_GenresGenreLevel(t *testing.T) {
	m := newModelWithTracks()
	m.activeView = viewGenres
	m.genres = GenresView{
		level:  gentui.GenreLevel,
		genres: []string{"Rock", "Jazz"},
		cursor: 0,
	}
	_, _, found := m.focusedTrackMeta()
	if found {
		t.Error("expected found=false at Genres GenreLevel")
	}
}

// TestFocusedTrackMeta_TagsTrackLevel verifies that focusedTrackMeta returns
// the artist and album of the focused track when the Tags view is at TrackLevel.
func TestFocusedTrackMeta_TagsTrackLevel(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewTags
	m.tagsView = TagsView{
		level: tagtui.TrackLevel,
		tracks: []db.PlaylistTrackRow{
			{Hash: "h1", Artist: "TagArtist", Album: "TagAlbum", Title: "TagSong"},
		},
		cursor: 0,
	}

	artist, album, found := m.focusedTrackMeta()
	if !found {
		t.Fatal("expected found=true at Tags TrackLevel")
	}
	if artist != "TagArtist" {
		t.Errorf("artist: want %q, got %q", "TagArtist", artist)
	}
	if album != "TagAlbum" {
		t.Errorf("album: want %q, got %q", "TagAlbum", album)
	}
}

// TestFocusedTrackMeta_TagsTagLevel verifies that focusedTrackMeta returns
// not-found at TagLevel since tags are not tracks.
func TestFocusedTrackMeta_TagsTagLevel(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewTags
	m.tagsView = TagsView{
		level: tagtui.TagLevel,
		tags:  []db.TagRow{{ID: 1, Name: "MyTag"}},
	}

	_, _, found := m.focusedTrackMeta()
	if found {
		t.Error("expected found=false at Tags TagLevel")
	}
}
