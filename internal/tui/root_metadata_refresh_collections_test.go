package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	libtui "github.com/sglavoie/muse/internal/tui/library"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
)

// TestRefreshDerivedViewsUpdatesPlaylistTracks verifies that
// refreshDerivedViews updates playlist track metadata when the playlists
// view is at TrackLevel.
func TestRefreshDerivedViewsUpdatesPlaylistTracks(t *testing.T) {
	m := New(context.Background())
	m.library.Index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Artist: "OldArt", Title: "OldTitle", Album: "OldAlbum"})
	m.playlists.Level = pltui.TrackLevel
	m.playlists.Tracks = []db.PlaylistTrackRow{
		{ID: 1, Hash: "h1", Path: "/a.flac", Artist: "OldArt", Title: "OldTitle", Album: "OldAlbum"},
	}

	m.library.Index.UpdateMetadata("h1", "NewTitle", "NewArt", "NewAlbum", "")
	m = m.refreshDerivedViews()

	if m.playlists.Tracks[0].Title != "NewTitle" {
		t.Errorf("playlist track title = %q, want %q", m.playlists.Tracks[0].Title, "NewTitle")
	}
	if m.playlists.Tracks[0].Artist != "NewArt" {
		t.Errorf("playlist track artist = %q, want %q", m.playlists.Tracks[0].Artist, "NewArt")
	}
	if m.playlists.Tracks[0].Album != "NewAlbum" {
		t.Errorf("playlist track album = %q, want %q", m.playlists.Tracks[0].Album, "NewAlbum")
	}
}

// TestRefreshDerivedViewsUpdatesFavoritesTracks verifies that
// refreshDerivedViews updates favorites track metadata at ListLevel with
// CategoryTracks.
func TestRefreshDerivedViewsUpdatesFavoritesTracks(t *testing.T) {
	m := New(context.Background())
	m.library.Index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Artist: "OldArt", Title: "OldTitle", Album: "OldAlbum"})
	m.favorites.level = favtui.ListLevel
	m.favorites.category = favtui.CategoryTracks
	m.favorites.tracks = []library.Track{
		{Hash: "h1", Path: "/a.flac", Artist: "OldArt", Title: "OldTitle", Album: "OldAlbum"},
	}

	m.library.Index.UpdateMetadata("h1", "NewTitle", "NewArt", "NewAlbum", "")
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
	m.library.Index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Artist: "OldArt", Title: "OldTitle", Album: "OldAlbum"})
	m.favorites.level = favtui.TrackLevel
	m.favorites.drilldownTracks = []library.Track{
		{Hash: "h1", Path: "/a.flac", Artist: "OldArt", Title: "OldTitle", Album: "OldAlbum"},
	}

	m.library.Index.UpdateMetadata("h1", "NewTitle", "NewArt", "NewAlbum", "")
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

// TestRefreshDerivedViewsUpdatesTagsTracks verifies that
// refreshDerivedViews updates tags view track metadata at TrackLevel.
func TestRefreshDerivedViewsUpdatesTagsTracks(t *testing.T) {
	m := New(context.Background())
	m.library.Index.Insert(library.Track{Hash: "th1", Path: "/a.flac", Artist: "OldArt", Title: "OldTitle", Album: "OldAlbum"})
	m.tagsView.level = tagtui.TrackLevel
	m.tagsView.tracks = []db.PlaylistTrackRow{
		{ID: 1, Hash: "th1", Path: "/a.flac", Artist: "OldArt", Title: "OldTitle", Album: "OldAlbum"},
	}

	m.library.Index.UpdateMetadata("th1", "NewTitle", "NewArt", "NewAlbum", "")
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
	m.library.Index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Artist: "Art", Title: "Title", Album: "Alb"})
	m.playlists.Level = pltui.ListLevel
	m = m.refreshDerivedViews()
}

// TestRefreshDerivedViewsUpdatesDuration verifies that refreshDerivedViews
// propagates duration changes from the library index to history, playlist,
// favorites, and tags tracks.
func TestRefreshDerivedViewsUpdatesDuration(t *testing.T) {
	m := New(context.Background())
	m.library.Index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Duration: 120})

	m.library.Mode = libtui.LibraryModeHistory
	m.library.SourceTracks = []library.Track{
		{Hash: "h1", Path: "/a.flac", Duration: 120},
	}
	m.playlists.Level = pltui.TrackLevel
	m.playlists.Tracks = []db.PlaylistTrackRow{
		{ID: 1, Hash: "h1", Path: "/a.flac", DurationMs: 120_000},
	}
	m.tagsView.level = tagtui.TrackLevel
	m.tagsView.tracks = []db.PlaylistTrackRow{
		{ID: 1, Hash: "h1", Path: "/a.flac", DurationMs: 120_000},
	}
	m.favorites.level = favtui.ListLevel
	m.favorites.category = favtui.CategoryTracks
	m.favorites.tracks = []library.Track{
		{Hash: "h1", Path: "/a.flac", Duration: 120},
	}

	// Update the library index duration (simulating a duration audit or
	// watcher correction).
	m.library.Index.UpdateDuration("h1", 200)
	m = m.refreshDerivedViews()

	if m.library.SourceTracks[0].Duration != 200 {
		t.Errorf("library sourceTracks Duration = %v, want 200", m.library.SourceTracks[0].Duration)
	}
	if m.playlists.Tracks[0].DurationMs != 200_000 {
		t.Errorf("playlists DurationMs = %d, want 200000", m.playlists.Tracks[0].DurationMs)
	}
	if m.tagsView.tracks[0].DurationMs != 200_000 {
		t.Errorf("tags DurationMs = %d, want 200000", m.tagsView.tracks[0].DurationMs)
	}
	if m.favorites.tracks[0].Duration != 200 {
		t.Errorf("favorites Duration = %v, want 200", m.favorites.tracks[0].Duration)
	}
}

// TestRefreshDerivedViewsUpdatesTrackAndDiscNumbers verifies that track and disc
// numbers reach every cache refreshTrackMetadata feeds. The TSV import edits
// both, and they are rendered, so omitting them left History, Favorites and
// Mixes showing the pre-import position.
func TestRefreshDerivedViewsUpdatesTrackAndDiscNumbers(t *testing.T) {
	m := New(context.Background())
	m.library.Index.Insert(library.Track{Hash: "h1", Path: "/a.flac", TrackNumber: 1, DiscNumber: 1})

	m.library.Mode = libtui.LibraryModeHistory
	m.library.SourceTracks = []library.Track{{Hash: "h1", Path: "/a.flac", TrackNumber: 1, DiscNumber: 1}}
	m.favorites.level = favtui.TrackLevel
	m.favorites.drilldownTracks = []library.Track{{Hash: "h1", Path: "/a.flac", TrackNumber: 1, DiscNumber: 1}}
	m.mixes.cachedMixes = map[db.MixID][]library.Track{
		db.MixRediscover: {{Hash: "h1", Path: "/a.flac", TrackNumber: 1, DiscNumber: 1}},
	}

	m.library.Index.UpdateTrackPosition("h1", 9, 2)
	m = m.refreshDerivedViews()

	for name, got := range map[string]library.Track{
		"history":   m.library.SourceTracks[0],
		"favorites": m.favorites.drilldownTracks[0],
		"mixes":     m.mixes.cachedMixes[db.MixRediscover][0],
	} {
		if got.TrackNumber != 9 || got.DiscNumber != 2 {
			t.Errorf("%s position = %d/%d, want 9/2", name, got.TrackNumber, got.DiscNumber)
		}
	}
}
