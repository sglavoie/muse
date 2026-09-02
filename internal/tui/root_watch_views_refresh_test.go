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
	m.library.Index.Insert(library.Track{Artist: "Art", Title: "T1", Path: "/a", Hash: "h1", Album: "Album1"})
	// Drill into "Art" albums.
	m.activeView = viewBrowse
	m.browse.mode = BrowseModeArtists
	m.browse.artists.artists = []string{"Art"}
	m.browse.artists.selectedArtist = "Art"
	m.browse.artists = m.browse.artists.openAlbums([]string{"Album1"})

	if len(m.browse.artists.albums) != 1 {
		t.Fatalf("setup: expected 1 album, got %d", len(m.browse.artists.albums))
	}

	// Watcher adds a second track with a new album under the same artist.
	next, _ := m.Update(watchBatchMsg{
		tracks: []library.Track{
			{Artist: "Art", Title: "T2", Path: "/b", Hash: "h2", Album: "Album2"},
		},
	})
	nm := next.(Model)

	if len(nm.browse.artists.albums) != 2 {
		t.Errorf("expected 2 albums after watcher, got %d: %v", len(nm.browse.artists.albums), nm.browse.artists.albums)
	}
}

// TestWatchBatchRefreshesArtistTrackLevel verifies that when the artists view
// is drilled into TrackLevel, a watchBatchMsg re-derives the track list so
// metadata changes are reflected immediately.
func TestWatchBatchRefreshesArtistTrackLevel(t *testing.T) {
	m := New(context.Background())
	m.library.Index.Insert(library.Track{Artist: "Art", Title: "T1", Path: "/a", Hash: "h1", Album: "Alb"})
	// Drill into "Art" > "Alb" tracks.
	m.activeView = viewBrowse
	m.browse.mode = BrowseModeArtists
	m.browse.artists.artists = []string{"Art"}
	m.browse.artists.selectedArtist = "Art"
	m.browse.artists = m.browse.artists.openAlbums([]string{"Alb"})
	m.browse.artists.selectedAlbum = "Alb"
	m.browse.artists = m.browse.artists.openTracks([]library.Track{
		{Artist: "Art", Title: "T1", Path: "/a", Hash: "h1", Album: "Alb"},
	})

	if len(m.browse.artists.tracks) != 1 {
		t.Fatalf("setup: expected 1 track, got %d", len(m.browse.artists.tracks))
	}

	// Watcher adds a new track under the same artist+album.
	next, _ := m.Update(watchBatchMsg{
		tracks: []library.Track{
			{Artist: "Art", Title: "T2", Path: "/b", Hash: "h2", Album: "Alb"},
		},
	})
	nm := next.(Model)

	if len(nm.browse.artists.tracks) != 2 {
		t.Errorf("expected 2 tracks after watcher, got %d", len(nm.browse.artists.tracks))
	}
}

// TestWatchBatchRefreshesAlbumTrackLevel verifies that when the albums view
// is drilled into TrackLevel, a watchBatchMsg re-derives the track list so
// new tracks and metadata changes are reflected immediately.
func TestWatchBatchRefreshesAlbumTrackLevel(t *testing.T) {
	m := New(context.Background())
	m.library.Index.Insert(library.Track{Artist: "Art", Title: "T1", Path: "/a", Hash: "h1", Album: "Alb"})
	// Drill into "Alb" tracks.
	m.activeView = viewBrowse
	m.browse.mode = BrowseModeAlbums
	m.browse.albums.albums = []string{"Alb"}
	m.browse.albums.selectedAlbum = "Alb"
	m.browse.albums = m.browse.albums.openTracks([]library.Track{
		{Artist: "Art", Title: "T1", Path: "/a", Hash: "h1", Album: "Alb"},
	})

	if len(m.browse.albums.tracks) != 1 {
		t.Fatalf("setup: expected 1 track, got %d", len(m.browse.albums.tracks))
	}

	// Watcher adds a new track under the same album.
	next, _ := m.Update(watchBatchMsg{
		tracks: []library.Track{
			{Artist: "Art2", Title: "T2", Path: "/b", Hash: "h2", Album: "Alb"},
		},
	})
	nm := next.(Model)

	if len(nm.browse.albums.tracks) != 2 {
		t.Errorf("expected 2 tracks after watcher, got %d", len(nm.browse.albums.tracks))
	}
}
