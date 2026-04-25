package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/library"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
)

// TestRefreshDerivedViewsUpdatesArtistsTrackLevel verifies that
// refreshDerivedViews rebuilds the track list when the artists view is at
// TrackLevel.
func TestRefreshDerivedViewsUpdatesArtistsTrackLevel(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "4")
	m = pressEnter(m)
	m = pressEnter(m)
	if m.artists.level != arttui.TrackLevel {
		t.Fatalf("expected TrackLevel, got %v", m.artists.level)
	}
	if len(m.artists.tracks) != 2 {
		t.Fatalf("expected 2 tracks at TrackLevel, got %d", len(m.artists.tracks))
	}

	m.library.index.UpdateMetadata("h1", "Renamed Track", "Alice", "Alpha", "")
	m = m.refreshDerivedViews()

	found := false
	for _, tr := range m.artists.tracks {
		if tr.Hash == "h1" {
			if tr.Title != "Renamed Track" {
				t.Errorf("expected Title=%q, got %q", "Renamed Track", tr.Title)
			}
			found = true
		}
	}
	if !found {
		t.Error("track h1 not found in refreshed artists.tracks")
	}
}

// TestRefreshDerivedViewsUpdatesAlbumsTrackLevel verifies that
// refreshDerivedViews rebuilds the track list when the albums view is at
// TrackLevel.
func TestRefreshDerivedViewsUpdatesAlbumsTrackLevel(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "5")
	m = pressEnter(m)
	if m.albums.level != albtui.TrackLevel {
		t.Fatalf("expected TrackLevel, got %v", m.albums.level)
	}
	if len(m.albums.tracks) < 1 {
		t.Fatalf("expected at least 1 track at TrackLevel, got %d", len(m.albums.tracks))
	}

	m.library.index.UpdateMetadata("h1", "Renamed Album Track", "Alice", "Alpha", "")
	m = m.refreshDerivedViews()

	found := false
	for _, tr := range m.albums.tracks {
		if tr.Hash == "h1" {
			if tr.Title != "Renamed Album Track" {
				t.Errorf("expected Title=%q, got %q", "Renamed Album Track", tr.Title)
			}
			found = true
		}
	}
	if !found {
		t.Error("track h1 not found in refreshed albums.tracks")
	}
}

// TestRefreshDerivedViewsResortsArtistsAfterEdit verifies that an active
// sort on the Artists view is re-applied after refreshDerivedViews.
func TestRefreshDerivedViewsResortsArtistsAfterEdit(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Artist: "Zebra", Title: "T1"})
	m.library.index.Insert(library.Track{Hash: "h2", Path: "/b.flac", Artist: "Alpha", Title: "T2"})
	m.library.index.Insert(library.Track{Hash: "h3", Path: "/c.flac", Artist: "Middle", Title: "T3"})
	m.artists.artists = arttui.DeriveArtists(m.library.index.Tracks())
	m.artists.level = arttui.ArtistLevel
	m.artists.sortState = SortState{Field: SortName, Dir: SortDesc}
	m.artists = m.artists.recomputeArtistsSort()
	if len(m.artists.artists) < 3 || m.artists.artists[0] != "Zebra" {
		t.Fatalf("pre-check: expected descending [Zebra, Middle, Alpha], got %v", m.artists.artists)
	}

	m = m.refreshDerivedViews()

	if len(m.artists.artists) != 3 {
		t.Fatalf("after refresh: expected 3 artists, got %d", len(m.artists.artists))
	}
	if m.artists.artists[0] != "Zebra" || m.artists.artists[1] != "Middle" || m.artists.artists[2] != "Alpha" {
		t.Errorf("after refresh: expected descending [Zebra, Middle, Alpha], got %v", m.artists.artists)
	}
}

// TestRefreshDerivedViewsResortsAlbumsAfterEdit verifies that an active
// sort on the Albums view is re-applied after refreshDerivedViews.
func TestRefreshDerivedViewsResortsAlbumsAfterEdit(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Artist: "Art", Album: "Zen", Title: "T1"})
	m.library.index.Insert(library.Track{Hash: "h2", Path: "/b.flac", Artist: "Art", Album: "Apex", Title: "T2"})
	m.library.index.Insert(library.Track{Hash: "h3", Path: "/c.flac", Artist: "Art", Album: "Mid", Title: "T3"})
	m.albums.albums = albtui.DeriveAllAlbums(m.library.index.Tracks())
	m.albums.level = albtui.AlbumLevel
	m.albums.sortState = SortState{Field: SortName, Dir: SortDesc}
	m.albums = m.albums.recomputeAlbumsSort()
	if len(m.albums.albums) < 3 || m.albums.albums[0] != "Zen" {
		t.Fatalf("pre-check: expected descending [Zen, Mid, Apex], got %v", m.albums.albums)
	}

	m = m.refreshDerivedViews()

	if len(m.albums.albums) != 3 {
		t.Fatalf("after refresh: expected 3 albums, got %d", len(m.albums.albums))
	}
	if m.albums.albums[0] != "Zen" || m.albums.albums[1] != "Mid" || m.albums.albums[2] != "Apex" {
		t.Errorf("after refresh: expected descending [Zen, Mid, Apex], got %v", m.albums.albums)
	}
}
