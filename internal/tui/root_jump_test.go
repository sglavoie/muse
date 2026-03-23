package tui

import (
	"context"
	"fmt"
	"testing"

	"github.com/sglavoie/muse/internal/library"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
)

// --- focusedTrackMeta ---

// TestFocusedTrackMeta_Library verifies that the library cursor drives the
// returned artist and album values.
func TestFocusedTrackMeta_Library(t *testing.T) {
	m := newModelWithTracks()
	// Default active view is viewLibrary, cursor=0 → Alice/Alpha.
	artist, album := m.focusedTrackMeta()
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
	artist, album := m.focusedTrackMeta()
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

	artist, album := m.focusedTrackMeta()
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

	artist, album := m.focusedTrackMeta()
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

		artist, album := m.focusedTrackMeta()
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

		artist, album := m.focusedTrackMeta()
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

		artist, album := m.focusedTrackMeta()
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

		artist, album := m.focusedTrackMeta()
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

		artist, album := m.focusedTrackMeta()
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

		artist, album := m.focusedTrackMeta()
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

		artist, album := m.focusedTrackMeta()
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

		artist, album := m.focusedTrackMeta()
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

		artist, album := m.focusedTrackMeta()
		if artist != "Charlie" {
			t.Errorf("artist: want %q, got %q", "Charlie", artist)
		}
		if album != "Delta" {
			t.Errorf("album: want %q, got %q", "Delta", album)
		}
	})
}

// TestFocusedTrackMeta_EmptyView verifies that an empty library returns ("","").
func TestFocusedTrackMeta_EmptyView(t *testing.T) {
	m := New(context.Background())
	artist, album := m.focusedTrackMeta()
	if artist != "" || album != "" {
		t.Errorf("empty library: want (%q,%q), got (%q,%q)", "", "", artist, album)
	}
}

// TestFocusedTrackMeta_QueueEmpty verifies that an empty queue returns ("","").
func TestFocusedTrackMeta_QueueEmpty(t *testing.T) {
	m := newModelWithTracks()
	m.activeView = viewQueue
	artist, album := m.focusedTrackMeta()
	if artist != "" || album != "" {
		t.Errorf("empty queue: want (%q,%q), got (%q,%q)", "", "", artist, album)
	}
}

// --- jumpToArtist ---

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
	// Build a library with 20 distinct artists.
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

	// DeriveArtists produces ["Artist00".."Artist19"]; "Artist15" is at index 15.
	m = m.jumpToArtist("Artist15")

	if m.artists.cursor != 15 {
		t.Errorf("cursor: want 15, got %d", m.artists.cursor)
	}
	wantScroll := max(0, 15-m.artists.height/2)
	if m.artists.scrollOffset != wantScroll {
		t.Errorf("scrollOffset: want %d, got %d", wantScroll, m.artists.scrollOffset)
	}
}

// --- jumpToAlbum ---

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

	// "Album15" is at index 15.
	m = m.jumpToAlbum("Album15")

	if m.albums.cursor != 15 {
		t.Errorf("cursor: want 15, got %d", m.albums.cursor)
	}
	wantScroll := max(0, 15-m.albums.height/2)
	if m.albums.scrollOffset != wantScroll {
		t.Errorf("scrollOffset: want %d, got %d", wantScroll, m.albums.scrollOffset)
	}
}
