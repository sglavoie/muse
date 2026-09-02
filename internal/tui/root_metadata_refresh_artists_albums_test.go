package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/library"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	"github.com/sglavoie/muse/internal/tui/sorting"
)

// TestRefreshDerivedViewsUpdatesArtistsTrackLevel verifies that
// refreshDerivedViews rebuilds the track list when the artists view is at
// TrackLevel.
func TestRefreshDerivedViewsUpdatesArtistsTrackLevel(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "4")
	m = pressEnter(m)
	m = pressEnter(m)
	if m.browse.Artists.Level != arttui.TrackLevel {
		t.Fatalf("expected TrackLevel, got %v", m.browse.Artists.Level)
	}
	if len(m.browse.Artists.Tracks) != 2 {
		t.Fatalf("expected 2 tracks at TrackLevel, got %d", len(m.browse.Artists.Tracks))
	}

	m.library.Index.UpdateMetadata("h1", "Renamed Track", "Alice", "Alpha", "")
	m = m.refreshDerivedViews()

	found := false
	for _, tr := range m.browse.Artists.Tracks {
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
	m = switchToAlbumsMode(m)
	m = pressEnter(m)
	if m.browse.Albums.Level != albtui.TrackLevel {
		t.Fatalf("expected TrackLevel, got %v", m.browse.Albums.Level)
	}
	if len(m.browse.Albums.Tracks) < 1 {
		t.Fatalf("expected at least 1 track at TrackLevel, got %d", len(m.browse.Albums.Tracks))
	}

	m.library.Index.UpdateMetadata("h1", "Renamed Album Track", "Alice", "Alpha", "")
	m = m.refreshDerivedViews()

	found := false
	for _, tr := range m.browse.Albums.Tracks {
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
	m.library.Index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Artist: "Zebra", Title: "T1"})
	m.library.Index.Insert(library.Track{Hash: "h2", Path: "/b.flac", Artist: "Alpha", Title: "T2"})
	m.library.Index.Insert(library.Track{Hash: "h3", Path: "/c.flac", Artist: "Middle", Title: "T3"})
	m.browse.Artists.Artists = arttui.DeriveArtists(m.library.Index.Tracks())
	m.browse.Artists.Level = arttui.ArtistLevel
	m.browse.Artists.SortState = sorting.SortState{Field: sorting.SortName, Dir: sorting.SortDesc}
	m.browse.Artists = m.browse.Artists.RecomputeArtistsSort()
	if len(m.browse.Artists.Artists) < 3 || m.browse.Artists.Artists[0] != "Zebra" {
		t.Fatalf("pre-check: expected descending [Zebra, Middle, Alpha], got %v", m.browse.Artists.Artists)
	}

	m = m.refreshDerivedViews()

	if len(m.browse.Artists.Artists) != 3 {
		t.Fatalf("after refresh: expected 3 artists, got %d", len(m.browse.Artists.Artists))
	}
	if m.browse.Artists.Artists[0] != "Zebra" || m.browse.Artists.Artists[1] != "Middle" || m.browse.Artists.Artists[2] != "Alpha" {
		t.Errorf("after refresh: expected descending [Zebra, Middle, Alpha], got %v", m.browse.Artists.Artists)
	}
}

// TestRefreshDerivedViewsResortsAlbumsAfterEdit verifies that an active
// sort on the Albums view is re-applied after refreshDerivedViews.
func TestRefreshDerivedViewsResortsAlbumsAfterEdit(t *testing.T) {
	m := New(context.Background())
	m.library.Index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Artist: "Art", Album: "Zen", Title: "T1"})
	m.library.Index.Insert(library.Track{Hash: "h2", Path: "/b.flac", Artist: "Art", Album: "Apex", Title: "T2"})
	m.library.Index.Insert(library.Track{Hash: "h3", Path: "/c.flac", Artist: "Art", Album: "Mid", Title: "T3"})
	m.browse.Albums.Albums = albtui.DeriveAllAlbums(m.library.Index.Tracks())
	m.browse.Albums.Level = albtui.AlbumLevel
	m.browse.Albums.SortState = sorting.SortState{Field: sorting.SortName, Dir: sorting.SortDesc}
	m.browse.Albums = m.browse.Albums.RecomputeAlbumsSort()
	if len(m.browse.Albums.Albums) < 3 || m.browse.Albums.Albums[0] != "Zen" {
		t.Fatalf("pre-check: expected descending [Zen, Mid, Apex], got %v", m.browse.Albums.Albums)
	}

	m = m.refreshDerivedViews()

	if len(m.browse.Albums.Albums) != 3 {
		t.Fatalf("after refresh: expected 3 albums, got %d", len(m.browse.Albums.Albums))
	}
	if m.browse.Albums.Albums[0] != "Zen" || m.browse.Albums.Albums[1] != "Mid" || m.browse.Albums.Albums[2] != "Apex" {
		t.Errorf("after refresh: expected descending [Zen, Mid, Apex], got %v", m.browse.Albums.Albums)
	}
}
