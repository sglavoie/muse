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
	libtui "github.com/sglavoie/muse/internal/tui/library"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
	"github.com/sglavoie/muse/internal/tui/sorting"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
)

// TestCorrectStoredDurationSyncsDerivedViews verifies that correctStoredDuration
// propagates the corrected duration to history, playlist, favorites, and tags
// views so they don't show stale values.
func TestCorrectStoredDurationSyncsDerivedViews(t *testing.T) {
	m := New(context.Background())
	m.library.Index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Duration: 360.0})
	m.library.Mode = libtui.LibraryModeHistory
	m.library.SourceTracks = []library.Track{{Hash: "h1", Duration: 360.0}}
	m.playlists.level = pltui.TrackLevel
	m.playlists.tracks = []db.PlaylistTrackRow{{Hash: "h1", DurationMs: 360000}}
	m.favorites.level = favtui.ListLevel
	m.favorites.category = favtui.CategoryTracks
	m.favorites.tracks = []library.Track{{Hash: "h1", Duration: 360.0}}
	m.tagsView.level = tagtui.TrackLevel
	m.tagsView.tracks = []db.PlaylistTrackRow{{Hash: "h1", DurationMs: 360000}}

	nm, _ := m.correctStoredDuration("h1", 180.0)

	if nm.library.SourceTracks[0].Duration != 180.0 {
		t.Errorf("history duration: got %f, want 180.0", nm.library.SourceTracks[0].Duration)
	}
	if nm.playlists.tracks[0].DurationMs != 180000 {
		t.Errorf("playlist DurationMs: got %d, want 180000", nm.playlists.tracks[0].DurationMs)
	}
	if nm.favorites.tracks[0].Duration != 180.0 {
		t.Errorf("favorites duration: got %f, want 180.0", nm.favorites.tracks[0].Duration)
	}
	if nm.tagsView.tracks[0].DurationMs != 180000 {
		t.Errorf("tags DurationMs: got %d, want 180000", nm.tagsView.tracks[0].DurationMs)
	}
}

// TestCorrectStoredDurationSyncsFavoritesDrilldown verifies that
// correctStoredDuration propagates to favorites drilldown tracks at TrackLevel.
func TestCorrectStoredDurationSyncsFavoritesDrilldown(t *testing.T) {
	m := New(context.Background())
	m.library.Index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Duration: 360.0})
	m.favorites.level = favtui.TrackLevel
	m.favorites.drilldownTracks = []library.Track{{Hash: "h1", Duration: 360.0}}

	nm, _ := m.correctStoredDuration("h1", 180.0)

	if nm.favorites.drilldownTracks[0].Duration != 180.0 {
		t.Errorf("favorites drilldown duration: got %f, want 180.0", nm.favorites.drilldownTracks[0].Duration)
	}
}

// TestCorrectStoredDurationSyncsArtistsTracks verifies that correctStoredDuration
// propagates to artists tracks at TrackLevel.
func TestCorrectStoredDurationSyncsArtistsTracks(t *testing.T) {
	m := New(context.Background())
	m.library.Index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Artist: "A", Album: "X", Duration: 360.0})
	m.browse.artists.level = arttui.TrackLevel
	m.browse.artists.tracks = []library.Track{{Hash: "h1", Artist: "A", Album: "X", Duration: 360.0}}

	nm, _ := m.correctStoredDuration("h1", 180.0)

	if nm.browse.artists.tracks[0].Duration != 180.0 {
		t.Errorf("artists track duration: got %f, want 180.0", nm.browse.artists.tracks[0].Duration)
	}
}

// TestCorrectStoredDurationSyncsAlbumsTracks verifies that correctStoredDuration
// propagates to albums tracks at TrackLevel.
func TestCorrectStoredDurationSyncsAlbumsTracks(t *testing.T) {
	m := New(context.Background())
	m.library.Index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Artist: "A", Album: "X", Duration: 360.0})
	m.browse.albums.level = albtui.TrackLevel
	m.browse.albums.tracks = []library.Track{{Hash: "h1", Artist: "A", Album: "X", Duration: 360.0}}

	nm, _ := m.correctStoredDuration("h1", 180.0)

	if nm.browse.albums.tracks[0].Duration != 180.0 {
		t.Errorf("albums track duration: got %f, want 180.0", nm.browse.albums.tracks[0].Duration)
	}
}

// TestCorrectStoredDurationSyncsGenresTracks verifies that correctStoredDuration
// propagates to genres tracks at TrackLevel.
func TestCorrectStoredDurationSyncsGenresTracks(t *testing.T) {
	m := New(context.Background())
	m.library.Index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Artist: "A", Genre: "Rock", Duration: 360.0})
	m.browse.genres.level = gentui.TrackLevel
	m.browse.genres.tracks = []library.Track{{Hash: "h1", Artist: "A", Genre: "Rock", Duration: 360.0}}

	nm, _ := m.correctStoredDuration("h1", 180.0)

	if nm.browse.genres.tracks[0].Duration != 180.0 {
		t.Errorf("genres track duration: got %f, want 180.0", nm.browse.genres.tracks[0].Duration)
	}
}

// TestCorrectStoredDurationRebuildsLibrarySortCache verifies that
// correctStoredDuration recomputes the library sortedTracks cache when a
// duration sort is active so the cached copy does not show stale values.
func TestCorrectStoredDurationRebuildsLibrarySortCache(t *testing.T) {
	m := New(context.Background())
	m.library.Index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Title: "A", Duration: 360.0})
	m.library.Index.Insert(library.Track{Hash: "h2", Path: "/b.flac", Title: "B", Duration: 100.0})
	m.library.SortState = sorting.SortState{Field: sorting.SortDuration, Dir: sorting.SortAsc}
	m.library = m.library.RecomputeSortedTracks()

	// Before correction: h2 (100s) < h1 (360s).
	if len(m.library.SortedTracks) != 2 || m.library.SortedTracks[0].Hash != "h2" {
		t.Fatal("pre-condition: expected h2 first in duration-asc sort")
	}

	nm, _ := m.correctStoredDuration("h1", 50.0)

	// After correction h1 is 50s, so h1 (50s) < h2 (100s).
	if nm.library.SortedTracks == nil {
		t.Fatal("expected sortedTracks to be non-nil after correction")
	}
	if len(nm.library.SortedTracks) != 2 || nm.library.SortedTracks[0].Hash != "h1" {
		t.Errorf("expected h1 first after duration correction, got %v", nm.library.SortedTracks[0].Hash)
	}
}

// TestCorrectStoredDurationRebuildsHistorySortCache verifies that
// correctStoredDuration recomputes the history sortedTracks cache when a sort
// is active.
func TestCorrectStoredDurationRebuildsHistorySortCache(t *testing.T) {
	m := New(context.Background())
	m.library.Index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Duration: 360.0})
	m.library.Mode = libtui.LibraryModeHistory
	m.library.SourceTracks = []library.Track{
		{Hash: "h1", Path: "/a.flac", Duration: 360.0},
		{Hash: "h2", Path: "/b.flac", Duration: 100.0},
	}
	m.library.SortState = sorting.SortState{Field: sorting.SortDuration, Dir: sorting.SortAsc}
	m.library = m.library.RecomputeSortedTracks()

	// Before: h2 (100s) < h1 (360s).
	if m.library.SortedTracks[0].Hash != "h2" {
		t.Fatal("pre-condition: expected h2 first")
	}

	nm, _ := m.correctStoredDuration("h1", 50.0)

	// After: h1 (50s) < h2 (100s).
	if nm.library.SortedTracks == nil {
		t.Fatal("expected history sortedTracks non-nil")
	}
	if nm.library.SortedTracks[0].Hash != "h1" {
		t.Errorf("expected h1 first after correction, got %v", nm.library.SortedTracks[0].Hash)
	}
}

// TestCorrectStoredDurationRebuildsArtistsSortCache verifies that
// correctStoredDuration recomputes the artists track sort cache when a
// duration sort is active.
func TestCorrectStoredDurationRebuildsArtistsSortCache(t *testing.T) {
	m := New(context.Background())
	m.library.Index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Artist: "A", Duration: 360.0})
	m.library.Index.Insert(library.Track{Hash: "h2", Path: "/b.flac", Artist: "A", Duration: 100.0})
	m.browse.artists.level = arttui.TrackLevel
	m.browse.artists.tracks = []library.Track{
		{Hash: "h1", Artist: "A", Duration: 360.0},
		{Hash: "h2", Artist: "A", Duration: 100.0},
	}
	m.browse.artists.sortState = sorting.SortState{Field: sorting.SortDuration, Dir: sorting.SortAsc}
	m.browse.artists = m.browse.artists.recomputeArtistsSort()

	// Before: h2 (100s) < h1 (360s).
	if m.browse.artists.tracks[0].Hash != "h2" {
		t.Fatal("pre-condition: expected h2 first in duration-asc sort")
	}

	nm, _ := m.correctStoredDuration("h1", 50.0)

	// After: h1 (50s) < h2 (100s).
	if nm.browse.artists.tracks[0].Hash != "h1" {
		t.Errorf("expected h1 first after correction, got %v", nm.browse.artists.tracks[0].Hash)
	}
}

// TestCorrectStoredDurationRebuildsAlbumsSortCache verifies that
// correctStoredDuration recomputes the albums track sort cache when a
// duration sort is active.
func TestCorrectStoredDurationRebuildsAlbumsSortCache(t *testing.T) {
	m := New(context.Background())
	m.library.Index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Artist: "A", Album: "X", Duration: 360.0})
	m.library.Index.Insert(library.Track{Hash: "h2", Path: "/b.flac", Artist: "A", Album: "X", Duration: 100.0})
	m.browse.albums.level = albtui.TrackLevel
	m.browse.albums.tracks = []library.Track{
		{Hash: "h1", Artist: "A", Album: "X", Duration: 360.0},
		{Hash: "h2", Artist: "A", Album: "X", Duration: 100.0},
	}
	m.browse.albums.sortState = sorting.SortState{Field: sorting.SortDuration, Dir: sorting.SortAsc}
	m.browse.albums = m.browse.albums.recomputeAlbumsSort()

	if m.browse.albums.tracks[0].Hash != "h2" {
		t.Fatal("pre-condition: expected h2 first in duration-asc sort")
	}

	nm, _ := m.correctStoredDuration("h1", 50.0)

	if nm.browse.albums.tracks[0].Hash != "h1" {
		t.Errorf("expected h1 first after correction, got %v", nm.browse.albums.tracks[0].Hash)
	}
}

// TestCorrectStoredDurationRebuildsGenresSortCache verifies that
// correctStoredDuration recomputes the genres track sort cache when a
// duration sort is active.
func TestCorrectStoredDurationRebuildsGenresSortCache(t *testing.T) {
	m := New(context.Background())
	m.library.Index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Artist: "A", Genre: "Rock", Duration: 360.0})
	m.library.Index.Insert(library.Track{Hash: "h2", Path: "/b.flac", Artist: "A", Genre: "Rock", Duration: 100.0})
	m.browse.genres.level = gentui.TrackLevel
	m.browse.genres.tracks = []library.Track{
		{Hash: "h1", Artist: "A", Genre: "Rock", Duration: 360.0},
		{Hash: "h2", Artist: "A", Genre: "Rock", Duration: 100.0},
	}
	m.browse.genres.sortState = sorting.SortState{Field: sorting.SortDuration, Dir: sorting.SortAsc}
	m.browse.genres = m.browse.genres.recomputeGenresSort()

	if m.browse.genres.tracks[0].Hash != "h2" {
		t.Fatal("pre-condition: expected h2 first in duration-asc sort")
	}

	nm, _ := m.correctStoredDuration("h1", 50.0)

	if nm.browse.genres.tracks[0].Hash != "h1" {
		t.Errorf("expected h1 first after correction, got %v", nm.browse.genres.tracks[0].Hash)
	}
}

// TestCorrectStoredDurationSyncsCachedMixTracks verifies that correctStoredDuration
// propagates to the mixes cache so that opening a mix later does not surface a
// stale pre-correction duration value.
func TestCorrectStoredDurationSyncsCachedMixTracks(t *testing.T) {
	m := New(context.Background())
	m.library.Index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Duration: 360.0})
	m.mixes.cachedMixes = map[db.MixID][]library.Track{
		db.MixRediscover: {{Hash: "h1", Duration: 360.0}},
	}

	nm, _ := m.correctStoredDuration("h1", 180.0)

	cached, ok := nm.mixes.cachedMixes[db.MixRediscover]
	if !ok || len(cached) == 0 {
		t.Fatal("mixes cache missing or empty after correction")
	}
	if cached[0].Duration != 180.0 {
		t.Errorf("cached mix track duration after correction: got %f, want 180.0", cached[0].Duration)
	}
}

// TestCorrectStoredDurationSyncsActiveMixTrackView verifies that when the user
// is viewing tracks inside a mix at the time of correction, the trackView
// reflects the corrected duration immediately.
func TestCorrectStoredDurationSyncsActiveMixTrackView(t *testing.T) {
	m := New(context.Background())
	m.library.Index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Duration: 360.0})
	m.mixes.cachedMixes = map[db.MixID][]library.Track{
		db.MixRediscover: {{Hash: "h1", Duration: 360.0}},
	}
	m.mixes.openMix(db.MixRediscover)

	nm, _ := m.correctStoredDuration("h1", 180.0)

	if len(nm.mixes.trackView.SourceTracks) == 0 {
		t.Fatal("mixes trackView sourceTracks empty after correction")
	}
	if nm.mixes.trackView.SourceTracks[0].Duration != 180.0 {
		t.Errorf("mixes trackView duration after correction: got %f, want 180.0",
			nm.mixes.trackView.SourceTracks[0].Duration)
	}
}

// TestCorrectStoredDurationRebuildsQueueSortCache verifies that when the queue
// has an active duration sort, correctStoredDuration recomputes the
// displayOrder cache so a per-track mpv-reported duration correction flips the
// visible order rather than leaving the user with stale pre-correction
// ordering.
func TestCorrectStoredDurationRebuildsQueueSortCache(t *testing.T) {
	m := New(context.Background())
	m.library.Index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Duration: 360.0})
	m.library.Index.Insert(library.Track{Hash: "h2", Path: "/b.flac", Duration: 100.0})
	m.queue.entries = []QueueEntry{
		{ID: 1, Hash: "h1", Path: "/a.flac", DurationMs: 360000},
		{ID: 2, Hash: "h2", Path: "/b.flac", DurationMs: 100000},
	}
	m.queue.sortState = sorting.SortState{Field: sorting.SortDuration, Dir: sorting.SortAsc}
	m.queue = m.queue.recomputeDisplayOrder()
	if m.queue.entries[m.queue.displayOrder[0]].Hash != "h2" {
		t.Fatalf("pre-correction queue order: want h2 first, got %s", m.queue.entries[m.queue.displayOrder[0]].Hash)
	}

	// Correct h1 down to 50s — h1 (50) should now be ordered before h2 (100).
	nm, _ := m.correctStoredDuration("h1", 50.0)

	if len(nm.queue.displayOrder) != 2 {
		t.Fatalf("post-correction displayOrder len: got %d, want 2", len(nm.queue.displayOrder))
	}
	if nm.queue.entries[nm.queue.displayOrder[0]].Hash != "h1" {
		t.Errorf("post-correction queue order: want h1 first, got %s",
			nm.queue.entries[nm.queue.displayOrder[0]].Hash)
	}
}
