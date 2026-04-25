package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/library"
)

// TestWatchBatchRefreshesArtistAlbumLevel verifies that when the artists view
// is drilled into AlbumLevel, a watchBatchMsg re-derives the album list so
// newly added albums appear and removed albums disappear.
func TestWatchBatchRefreshesArtistAlbumLevel(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Artist: "Art", Title: "T1", Path: "/a", Hash: "h1", Album: "Album1"})
	// Drill into "Art" albums.
	m.activeView = viewArtists
	m.artists.artists = []string{"Art"}
	m.artists.selectedArtist = "Art"
	m.artists = m.artists.openAlbums([]string{"Album1"})

	if len(m.artists.albums) != 1 {
		t.Fatalf("setup: expected 1 album, got %d", len(m.artists.albums))
	}

	// Watcher adds a second track with a new album under the same artist.
	next, _ := m.Update(watchBatchMsg{
		tracks: []library.Track{
			{Artist: "Art", Title: "T2", Path: "/b", Hash: "h2", Album: "Album2"},
		},
	})
	nm := next.(Model)

	if len(nm.artists.albums) != 2 {
		t.Errorf("expected 2 albums after watcher, got %d: %v", len(nm.artists.albums), nm.artists.albums)
	}
}

// TestWatchBatchRefreshesArtistTrackLevel verifies that when the artists view
// is drilled into TrackLevel, a watchBatchMsg re-derives the track list so
// metadata changes are reflected immediately.
func TestWatchBatchRefreshesArtistTrackLevel(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Artist: "Art", Title: "T1", Path: "/a", Hash: "h1", Album: "Alb"})
	// Drill into "Art" > "Alb" tracks.
	m.activeView = viewArtists
	m.artists.artists = []string{"Art"}
	m.artists.selectedArtist = "Art"
	m.artists = m.artists.openAlbums([]string{"Alb"})
	m.artists.selectedAlbum = "Alb"
	m.artists = m.artists.openTracks([]library.Track{
		{Artist: "Art", Title: "T1", Path: "/a", Hash: "h1", Album: "Alb"},
	})

	if len(m.artists.tracks) != 1 {
		t.Fatalf("setup: expected 1 track, got %d", len(m.artists.tracks))
	}

	// Watcher adds a new track under the same artist+album.
	next, _ := m.Update(watchBatchMsg{
		tracks: []library.Track{
			{Artist: "Art", Title: "T2", Path: "/b", Hash: "h2", Album: "Alb"},
		},
	})
	nm := next.(Model)

	if len(nm.artists.tracks) != 2 {
		t.Errorf("expected 2 tracks after watcher, got %d", len(nm.artists.tracks))
	}
}

// TestWatchBatchRefreshesAlbumTrackLevel verifies that when the albums view
// is drilled into TrackLevel, a watchBatchMsg re-derives the track list so
// new tracks and metadata changes are reflected immediately.
func TestWatchBatchRefreshesAlbumTrackLevel(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Artist: "Art", Title: "T1", Path: "/a", Hash: "h1", Album: "Alb"})
	// Drill into "Alb" tracks.
	m.activeView = viewAlbums
	m.albums.albums = []string{"Alb"}
	m.albums.selectedAlbum = "Alb"
	m.albums = m.albums.openTracks([]library.Track{
		{Artist: "Art", Title: "T1", Path: "/a", Hash: "h1", Album: "Alb"},
	})

	if len(m.albums.tracks) != 1 {
		t.Fatalf("setup: expected 1 track, got %d", len(m.albums.tracks))
	}

	// Watcher adds a new track under the same album.
	next, _ := m.Update(watchBatchMsg{
		tracks: []library.Track{
			{Artist: "Art2", Title: "T2", Path: "/b", Hash: "h2", Album: "Alb"},
		},
	})
	nm := next.(Model)

	if len(nm.albums.tracks) != 2 {
		t.Errorf("expected 2 tracks after watcher, got %d", len(nm.albums.tracks))
	}
}
