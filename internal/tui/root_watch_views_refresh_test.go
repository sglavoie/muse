package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/browse"
)

// TestWatchBatchRefreshesArtistAlbumLevel verifies that when the artists view
// is drilled into AlbumLevel, a watchBatchMsg re-derives the album list so
// newly added albums appear and removed albums disappear.
func TestWatchBatchRefreshesArtistAlbumLevel(t *testing.T) {
	m := New(context.Background())
	m.library.Index.Insert(library.Track{Artist: "Art", Title: "T1", Path: "/a", Hash: "h1", Album: "Album1"})
	// Drill into "Art" albums.
	m.ActiveView = viewBrowse
	m.browse.Mode = browse.BrowseModeArtists
	m.browse.Artists.Artists = []string{"Art"}
	m.browse.Artists.SelectedArtist = "Art"
	m.browse.Artists = m.browse.Artists.OpenAlbums([]string{"Album1"})

	if len(m.browse.Artists.Albums) != 1 {
		t.Fatalf("setup: expected 1 album, got %d", len(m.browse.Artists.Albums))
	}

	// Watcher adds a second track with a new album under the same artist.
	next, _ := m.Update(watchBatchMsg{
		tracks: []library.Track{
			{Artist: "Art", Title: "T2", Path: "/b", Hash: "h2", Album: "Album2"},
		},
	})
	nm := next.(Model)

	if len(nm.browse.Artists.Albums) != 2 {
		t.Errorf("expected 2 albums after watcher, got %d: %v", len(nm.browse.Artists.Albums), nm.browse.Artists.Albums)
	}
}

// TestWatchBatchRefreshesArtistTrackLevel verifies that when the artists view
// is drilled into TrackLevel, a watchBatchMsg re-derives the track list so
// metadata changes are reflected immediately.
func TestWatchBatchRefreshesArtistTrackLevel(t *testing.T) {
	m := New(context.Background())
	m.library.Index.Insert(library.Track{Artist: "Art", Title: "T1", Path: "/a", Hash: "h1", Album: "Alb"})
	// Drill into "Art" > "Alb" tracks.
	m.ActiveView = viewBrowse
	m.browse.Mode = browse.BrowseModeArtists
	m.browse.Artists.Artists = []string{"Art"}
	m.browse.Artists.SelectedArtist = "Art"
	m.browse.Artists = m.browse.Artists.OpenAlbums([]string{"Alb"})
	m.browse.Artists.SelectedAlbum = "Alb"
	m.browse.Artists = m.browse.Artists.OpenTracks([]library.Track{
		{Artist: "Art", Title: "T1", Path: "/a", Hash: "h1", Album: "Alb"},
	})

	if len(m.browse.Artists.Tracks) != 1 {
		t.Fatalf("setup: expected 1 track, got %d", len(m.browse.Artists.Tracks))
	}

	// Watcher adds a new track under the same artist+album.
	next, _ := m.Update(watchBatchMsg{
		tracks: []library.Track{
			{Artist: "Art", Title: "T2", Path: "/b", Hash: "h2", Album: "Alb"},
		},
	})
	nm := next.(Model)

	if len(nm.browse.Artists.Tracks) != 2 {
		t.Errorf("expected 2 tracks after watcher, got %d", len(nm.browse.Artists.Tracks))
	}
}

// TestWatchBatchRefreshesAlbumTrackLevel verifies that when the albums view
// is drilled into TrackLevel, a watchBatchMsg re-derives the track list so
// new tracks and metadata changes are reflected immediately.
func TestWatchBatchRefreshesAlbumTrackLevel(t *testing.T) {
	m := New(context.Background())
	m.library.Index.Insert(library.Track{Artist: "Art", Title: "T1", Path: "/a", Hash: "h1", Album: "Alb"})
	// Drill into "Alb" tracks.
	m.ActiveView = viewBrowse
	m.browse.Mode = browse.BrowseModeAlbums
	m.browse.Albums.Albums = []string{"Alb"}
	m.browse.Albums.SelectedAlbum = "Alb"
	m.browse.Albums = m.browse.Albums.OpenTracks([]library.Track{
		{Artist: "Art", Title: "T1", Path: "/a", Hash: "h1", Album: "Alb"},
	})

	if len(m.browse.Albums.Tracks) != 1 {
		t.Fatalf("setup: expected 1 track, got %d", len(m.browse.Albums.Tracks))
	}

	// Watcher adds a new track under the same album.
	next, _ := m.Update(watchBatchMsg{
		tracks: []library.Track{
			{Artist: "Art2", Title: "T2", Path: "/b", Hash: "h2", Album: "Alb"},
		},
	})
	nm := next.(Model)

	if len(nm.browse.Albums.Tracks) != 2 {
		t.Errorf("expected 2 tracks after watcher, got %d", len(nm.browse.Albums.Tracks))
	}
}
