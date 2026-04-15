package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
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

// TestRefreshDerivedViewsRebuildsLibrarySortCache verifies that
// refreshDerivedViews recomputes the library sort cache when sorting is active.
func TestRefreshDerivedViewsRebuildsLibrarySortCache(t *testing.T) {
	m := newModelWithTracks()
	m.library.sortState = SortState{Field: SortTitle, Dir: SortAsc}
	m.library = m.library.recomputeSortedTracks()
	if m.library.sortedTracks == nil {
		t.Fatal("expected sortedTracks to be non-nil after recomputeSortedTracks")
	}

	m.library.index.UpdateMetadata("h1", "ZZZ Last", "Alice", "Alpha", "")
	m = m.refreshDerivedViews()

	if m.library.sortedTracks == nil {
		t.Fatal("expected sortedTracks to remain non-nil after refreshDerivedViews")
	}
	last := m.library.sortedTracks[len(m.library.sortedTracks)-1]
	if last.Hash != "h1" || last.Title != "ZZZ Last" {
		t.Errorf("expected renamed track at end of sorted list, got hash=%q title=%q", last.Hash, last.Title)
	}
}

// TestRefreshDerivedViewsRebuildsLibraryFilterCache verifies that
// refreshDerivedViews recomputes the library filter cache when a filter is
// active.
func TestRefreshDerivedViewsRebuildsLibraryFilterCache(t *testing.T) {
	m := newModelWithTracks()
	m.library.filter.Text = "Unique"
	m.library = m.library.rebuildFilterCache()
	if len(m.library.filteredMatches) != 0 {
		t.Fatalf("expected 0 filter matches before rename, got %d", len(m.library.filteredMatches))
	}

	m.library.index.UpdateMetadata("h1", "Unique Title", "Alice", "Alpha", "")
	m = m.refreshDerivedViews()

	if len(m.library.filteredMatches) == 0 {
		t.Error("expected filter matches after renaming track to 'Unique Title', got 0")
	}
}

// TestRefreshDerivedViewsUpdatesPlaylistTracks verifies that
// refreshDerivedViews updates playlist track metadata when the playlists
// view is at TrackLevel.
func TestRefreshDerivedViewsUpdatesPlaylistTracks(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Artist: "OldArt", Title: "OldTitle", Album: "OldAlbum"})
	m.playlists.level = pltui.TrackLevel
	m.playlists.tracks = []db.PlaylistTrackRow{
		{ID: 1, Hash: "h1", Path: "/a.flac", Artist: "OldArt", Title: "OldTitle", Album: "OldAlbum"},
	}

	m.library.index.UpdateMetadata("h1", "NewTitle", "NewArt", "NewAlbum", "")
	m = m.refreshDerivedViews()

	if m.playlists.tracks[0].Title != "NewTitle" {
		t.Errorf("playlist track title = %q, want %q", m.playlists.tracks[0].Title, "NewTitle")
	}
	if m.playlists.tracks[0].Artist != "NewArt" {
		t.Errorf("playlist track artist = %q, want %q", m.playlists.tracks[0].Artist, "NewArt")
	}
	if m.playlists.tracks[0].Album != "NewAlbum" {
		t.Errorf("playlist track album = %q, want %q", m.playlists.tracks[0].Album, "NewAlbum")
	}
}

// TestRefreshDerivedViewsUpdatesFavoritesTracks verifies that
// refreshDerivedViews updates favorites track metadata at ListLevel with
// CategoryTracks.
func TestRefreshDerivedViewsUpdatesFavoritesTracks(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Artist: "OldArt", Title: "OldTitle", Album: "OldAlbum"})
	m.favorites.level = favtui.ListLevel
	m.favorites.category = favtui.CategoryTracks
	m.favorites.tracks = []library.Track{
		{Hash: "h1", Path: "/a.flac", Artist: "OldArt", Title: "OldTitle", Album: "OldAlbum"},
	}

	m.library.index.UpdateMetadata("h1", "NewTitle", "NewArt", "NewAlbum", "")
	m = m.refreshDerivedViews()

	if m.favorites.tracks[0].Title != "NewTitle" {
		t.Errorf("favorites track title = %q, want %q", m.favorites.tracks[0].Title, "NewTitle")
	}
	if m.favorites.tracks[0].Artist != "NewArt" {
		t.Errorf("favorites track artist = %q, want %q", m.favorites.tracks[0].Artist, "NewArt")
	}
	if m.favorites.tracks[0].Album != "NewAlbum" {
		t.Errorf("favorites track album = %q, want %q", m.favorites.tracks[0].Album, "NewAlbum")
	}
}

// TestRefreshDerivedViewsUpdatesFavoritesDrilldownTracks verifies that
// refreshDerivedViews updates favorites drilldown track metadata at TrackLevel.
func TestRefreshDerivedViewsUpdatesFavoritesDrilldownTracks(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Artist: "OldArt", Title: "OldTitle", Album: "OldAlbum"})
	m.favorites.level = favtui.TrackLevel
	m.favorites.drilldownTracks = []library.Track{
		{Hash: "h1", Path: "/a.flac", Artist: "OldArt", Title: "OldTitle", Album: "OldAlbum"},
	}

	m.library.index.UpdateMetadata("h1", "NewTitle", "NewArt", "NewAlbum", "")
	m = m.refreshDerivedViews()

	if m.favorites.drilldownTracks[0].Title != "NewTitle" {
		t.Errorf("drilldown track title = %q, want %q", m.favorites.drilldownTracks[0].Title, "NewTitle")
	}
	if m.favorites.drilldownTracks[0].Artist != "NewArt" {
		t.Errorf("drilldown track artist = %q, want %q", m.favorites.drilldownTracks[0].Artist, "NewArt")
	}
	if m.favorites.drilldownTracks[0].Album != "NewAlbum" {
		t.Errorf("drilldown track album = %q, want %q", m.favorites.drilldownTracks[0].Album, "NewAlbum")
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

// TestRefreshDerivedViewsUpdatesTagsTracks verifies that
// refreshDerivedViews updates tags view track metadata at TrackLevel.
func TestRefreshDerivedViewsUpdatesTagsTracks(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "th1", Path: "/a.flac", Artist: "OldArt", Title: "OldTitle", Album: "OldAlbum"})
	m.tagsView.level = tagtui.TrackLevel
	m.tagsView.tracks = []db.PlaylistTrackRow{
		{ID: 1, Hash: "th1", Path: "/a.flac", Artist: "OldArt", Title: "OldTitle", Album: "OldAlbum"},
	}

	m.library.index.UpdateMetadata("th1", "NewTitle", "NewArt", "NewAlbum", "")
	m = m.refreshDerivedViews()

	if m.tagsView.tracks[0].Title != "NewTitle" {
		t.Errorf("tags track title = %q, want %q", m.tagsView.tracks[0].Title, "NewTitle")
	}
	if m.tagsView.tracks[0].Artist != "NewArt" {
		t.Errorf("tags track artist = %q, want %q", m.tagsView.tracks[0].Artist, "NewArt")
	}
	if m.tagsView.tracks[0].Album != "NewAlbum" {
		t.Errorf("tags track album = %q, want %q", m.tagsView.tracks[0].Album, "NewAlbum")
	}
}

// TestRefreshDerivedViewsSkipsTagsTracksAtTagLevel verifies that
// refreshDerivedViews skips tags track metadata refresh at TagLevel.
func TestRefreshDerivedViewsSkipsTagsTracksAtTagLevel(t *testing.T) {
	m := New(context.Background())
	m.tagsView.level = tagtui.TagLevel
	m.tagsView.tags = []db.TagRow{{ID: 1, Name: "T1"}}
	// Should not panic with empty tracks at TagLevel.
	m = m.refreshDerivedViews()
}

// TestRefreshDerivedViewsSkipsPlaylistTracksAtListLevel verifies that
// refreshDerivedViews does not iterate playlist tracks when the view is at
// ListLevel.
func TestRefreshDerivedViewsSkipsPlaylistTracksAtListLevel(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Artist: "Art", Title: "Title", Album: "Alb"})
	m.playlists.level = pltui.ListLevel
	m = m.refreshDerivedViews()
}
