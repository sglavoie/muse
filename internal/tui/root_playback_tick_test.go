package tui

import (
	"context"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	gentui "github.com/sglavoie/muse/internal/tui/genres"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
)

// TestMainPlaybackTickCorrectsDuration verifies that when mpv reports a
// duration that significantly differs from the stored value, the library
// index, queue entries, and DB are corrected.
func TestMainPlaybackTickCorrectsDuration(t *testing.T) {
	dw := &stubDurationWriter{}
	sp := &stubElapsedReader{elapsed: 10.0, total: 180.0}
	m := New(context.Background(), WithPlayer(sp), WithDurationWriter(dw))
	m.library.index.Insert(library.Track{
		Hash: "h1", Path: "/a.flac", Artist: "A", Title: "T", Duration: 360.0,
	})
	m.queue.entries = []QueueEntry{
		{Hash: "h1", Path: "/a.flac", Artist: "A", Title: "T", DurationMs: 360000},
	}
	m.playback.state = PlaybackPlaying
	m.playback.currentHash = "h1"

	nm, cmd := m.handleTickMsg()

	// Library index should be corrected to mpv value.
	if t2, ok := nm.library.index.FindByHash("h1"); !ok || t2.Duration != 180.0 {
		t.Fatalf("expected library duration 180.0, got %v", t2.Duration)
	}
	// Queue entry should be corrected.
	if nm.queue.entries[0].DurationMs != 180000 {
		t.Fatalf("expected queue DurationMs 180000, got %d", nm.queue.entries[0].DurationMs)
	}
	// Flag should be set.
	if !nm.playback.durationCorrected {
		t.Fatal("expected durationCorrected to be set")
	}
	// DB write should be scheduled.
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}
	batch, ok := cmd().(tea.BatchMsg)
	if !ok {
		t.Fatal("expected BatchMsg")
	}
	for _, c := range batch {
		if c == nil {
			continue
		}
		msg := make(chan tea.Msg, 1)
		go func(fn tea.Cmd) { msg <- fn() }(c)
		select {
		case <-msg:
		case <-time.After(50 * time.Millisecond):
		}
	}
	if len(dw.calls) != 1 {
		t.Fatalf("expected 1 DB write call, got %d", len(dw.calls))
	}
	if dw.calls[0].hash != "h1" || dw.calls[0].durationMs != 180000 {
		t.Fatalf("unexpected DB write: %+v", dw.calls[0])
	}
}

// TestMainPlaybackTickSkipsSmallDifference verifies that when the stored
// duration is within 2 seconds of mpv's total, no correction occurs.
func TestMainPlaybackTickSkipsSmallDifference(t *testing.T) {
	dw := &stubDurationWriter{}
	sp := &stubElapsedReader{elapsed: 10.0, total: 180.0}
	m := New(context.Background(), WithPlayer(sp), WithDurationWriter(dw))
	m.library.index.Insert(library.Track{
		Hash: "h1", Path: "/a.flac", Artist: "A", Title: "T", Duration: 180.5,
	})
	m.queue.entries = []QueueEntry{
		{Hash: "h1", Path: "/a.flac", Artist: "A", Title: "T", DurationMs: 180500},
	}
	m.playback.state = PlaybackPlaying
	m.playback.currentHash = "h1"

	nm, _ := m.handleTickMsg()

	// Library index should NOT be changed.
	if t2, ok := nm.library.index.FindByHash("h1"); !ok || t2.Duration != 180.5 {
		t.Fatalf("expected library duration to stay 180.5, got %v", t2.Duration)
	}
	if nm.queue.entries[0].DurationMs != 180500 {
		t.Fatalf("expected queue DurationMs to stay 180500, got %d", nm.queue.entries[0].DurationMs)
	}
	if !nm.playback.durationCorrected {
		t.Fatal("expected durationCorrected to be set (to prevent re-checking)")
	}
	if len(dw.calls) != 0 {
		t.Fatalf("expected 0 DB write calls, got %d", len(dw.calls))
	}
}

// TestMainPlaybackTickFillsZeroDuration verifies that when a track has
// Duration=0, the main playback tick fills it from mpv.
func TestMainPlaybackTickFillsZeroDuration(t *testing.T) {
	dw := &stubDurationWriter{}
	sp := &stubElapsedReader{elapsed: 5.0, total: 180.0}
	m := New(context.Background(), WithPlayer(sp), WithDurationWriter(dw))
	m.library.index.Insert(library.Track{
		Hash: "h1", Path: "/a.flac", Artist: "A", Title: "T", Duration: 0,
	})
	m.queue.entries = []QueueEntry{
		{Hash: "h1", Path: "/a.flac", Artist: "A", Title: "T", DurationMs: 0},
	}
	m.playback.state = PlaybackPlaying
	m.playback.currentHash = "h1"

	nm, _ := m.handleTickMsg()

	if t2, ok := nm.library.index.FindByHash("h1"); !ok || t2.Duration != 180.0 {
		t.Fatalf("expected library duration 180.0, got %v", t2.Duration)
	}
	if nm.queue.entries[0].DurationMs != 180000 {
		t.Fatalf("expected queue DurationMs 180000, got %d", nm.queue.entries[0].DurationMs)
	}
}

// TestMainPlaybackTickCorrectOnceOnly verifies that after the flag is set,
// subsequent ticks do not trigger additional corrections.
func TestMainPlaybackTickCorrectOnceOnly(t *testing.T) {
	dw := &stubDurationWriter{}
	sp := &stubElapsedReader{elapsed: 10.0, total: 180.0}
	m := New(context.Background(), WithPlayer(sp), WithDurationWriter(dw))
	m.library.index.Insert(library.Track{
		Hash: "h1", Path: "/a.flac", Artist: "A", Title: "T", Duration: 360.0,
	})
	m.queue.entries = []QueueEntry{
		{Hash: "h1", Path: "/a.flac", Artist: "A", Title: "T", DurationMs: 360000},
	}
	m.playback.state = PlaybackPlaying
	m.playback.currentHash = "h1"

	// First tick corrects.
	nm, _ := m.handleTickMsg()
	if !nm.playback.durationCorrected {
		t.Fatal("expected durationCorrected after first tick")
	}

	// Change the stub to report a different total.
	sp.total = 999.0
	nm2, _ := nm.handleTickMsg()

	// Duration should remain at the first correction (180), not 999.
	if t2, ok := nm2.library.index.FindByHash("h1"); !ok || t2.Duration != 180.0 {
		t.Fatalf("expected duration to remain 180.0, got %v", t2.Duration)
	}
}

// TestTrackChangedResetsCorrectFlag verifies that the durationCorrected flag
// is reset when a new track starts.
func TestTrackChangedResetsCorrectFlag(t *testing.T) {
	m := New(context.Background())
	m.playback.durationCorrected = true
	m.queue.entries = []QueueEntry{{ID: 1, Hash: "h2", Path: "/b.flac"}}

	nm, _ := m.handleTrackChangedMsg(TrackChangedMsg{
		Hash: "h2", Index: 0, Artist: "B", Title: "T2",
	})

	if nm.playback.durationCorrected {
		t.Fatal("expected durationCorrected to be reset after track change")
	}
}

// TestTickMsgUpdatesPeakElapsed verifies that the tick handler updates the
// peak elapsed fields when the elapsed value increases.
func TestTickMsgUpdatesPeakElapsed(t *testing.T) {
	sp := &stubElapsedReader{elapsed: 45.0, total: 200.0}
	m := New(context.Background(), WithPlayer(sp))
	m.playback.state = PlaybackPlaying
	m.playback.currentHash = "h1"

	nm, _ := m.handleTickMsg()

	if nm.playback.peakElapsedSecs != 45.0 {
		t.Fatalf("expected peakElapsedSecs=45.0, got %v", nm.playback.peakElapsedSecs)
	}
	if nm.playback.peakTotalSecs != 200.0 {
		t.Fatalf("expected peakTotalSecs=200.0, got %v", nm.playback.peakTotalSecs)
	}
}

// TestTickMsgDoesNotDecreasePeak verifies that the peak does not decrease when
// the tick reports a smaller elapsed value (e.g. after a gapless transition
// where mpv is already playing the next track).
func TestTickMsgDoesNotDecreasePeak(t *testing.T) {
	sp := &stubElapsedReader{elapsed: 0.5, total: 300.0}
	m := New(context.Background(), WithPlayer(sp))
	m.playback.state = PlaybackPlaying
	m.playback.currentHash = "h1"
	m.playback.peakElapsedSecs = 180.0
	m.playback.peakTotalSecs = 200.0

	nm, _ := m.handleTickMsg()

	if nm.playback.peakElapsedSecs != 180.0 {
		t.Fatalf("expected peakElapsedSecs to stay 180.0, got %v", nm.playback.peakElapsedSecs)
	}
	if nm.playback.peakTotalSecs != 200.0 {
		t.Fatalf("expected peakTotalSecs to stay 200.0, got %v", nm.playback.peakTotalSecs)
	}
}

// TestCorrectStoredDurationSyncsDerivedViews verifies that correctStoredDuration
// propagates the corrected duration to history, playlist, favorites, and tags
// views so they don't show stale values.
func TestCorrectStoredDurationSyncsDerivedViews(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Duration: 360.0})
	m.library.mode = LibraryModeHistory
	m.library.sourceTracks = []library.Track{{Hash: "h1", Duration: 360.0}}
	m.playlists.level = pltui.TrackLevel
	m.playlists.tracks = []db.PlaylistTrackRow{{Hash: "h1", DurationMs: 360000}}
	m.favorites.level = favtui.ListLevel
	m.favorites.category = favtui.CategoryTracks
	m.favorites.tracks = []library.Track{{Hash: "h1", Duration: 360.0}}
	m.tagsView.level = tagtui.TrackLevel
	m.tagsView.tracks = []db.PlaylistTrackRow{{Hash: "h1", DurationMs: 360000}}

	nm, _ := m.correctStoredDuration("h1", 180.0)

	if nm.library.sourceTracks[0].Duration != 180.0 {
		t.Errorf("history duration: got %f, want 180.0", nm.library.sourceTracks[0].Duration)
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
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Duration: 360.0})
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
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Artist: "A", Album: "X", Duration: 360.0})
	m.artists.level = arttui.TrackLevel
	m.artists.tracks = []library.Track{{Hash: "h1", Artist: "A", Album: "X", Duration: 360.0}}

	nm, _ := m.correctStoredDuration("h1", 180.0)

	if nm.artists.tracks[0].Duration != 180.0 {
		t.Errorf("artists track duration: got %f, want 180.0", nm.artists.tracks[0].Duration)
	}
}

// TestCorrectStoredDurationSyncsAlbumsTracks verifies that correctStoredDuration
// propagates to albums tracks at TrackLevel.
func TestCorrectStoredDurationSyncsAlbumsTracks(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Artist: "A", Album: "X", Duration: 360.0})
	m.albums.level = albtui.TrackLevel
	m.albums.tracks = []library.Track{{Hash: "h1", Artist: "A", Album: "X", Duration: 360.0}}

	nm, _ := m.correctStoredDuration("h1", 180.0)

	if nm.albums.tracks[0].Duration != 180.0 {
		t.Errorf("albums track duration: got %f, want 180.0", nm.albums.tracks[0].Duration)
	}
}

// TestCorrectStoredDurationSyncsGenresTracks verifies that correctStoredDuration
// propagates to genres tracks at TrackLevel.
func TestCorrectStoredDurationSyncsGenresTracks(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Artist: "A", Genre: "Rock", Duration: 360.0})
	m.genres.level = gentui.TrackLevel
	m.genres.tracks = []library.Track{{Hash: "h1", Artist: "A", Genre: "Rock", Duration: 360.0}}

	nm, _ := m.correctStoredDuration("h1", 180.0)

	if nm.genres.tracks[0].Duration != 180.0 {
		t.Errorf("genres track duration: got %f, want 180.0", nm.genres.tracks[0].Duration)
	}
}

// TestCorrectStoredDurationRebuildsLibrarySortCache verifies that
// correctStoredDuration recomputes the library sortedTracks cache when a
// duration sort is active so the cached copy does not show stale values.
func TestCorrectStoredDurationRebuildsLibrarySortCache(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Title: "A", Duration: 360.0})
	m.library.index.Insert(library.Track{Hash: "h2", Path: "/b.flac", Title: "B", Duration: 100.0})
	m.library.sortState = SortState{Field: SortDuration, Dir: SortAsc}
	m.library = m.library.recomputeSortedTracks()

	// Before correction: h2 (100s) < h1 (360s).
	if len(m.library.sortedTracks) != 2 || m.library.sortedTracks[0].Hash != "h2" {
		t.Fatal("pre-condition: expected h2 first in duration-asc sort")
	}

	nm, _ := m.correctStoredDuration("h1", 50.0)

	// After correction h1 is 50s, so h1 (50s) < h2 (100s).
	if nm.library.sortedTracks == nil {
		t.Fatal("expected sortedTracks to be non-nil after correction")
	}
	if len(nm.library.sortedTracks) != 2 || nm.library.sortedTracks[0].Hash != "h1" {
		t.Errorf("expected h1 first after duration correction, got %v", nm.library.sortedTracks[0].Hash)
	}
}

// TestCorrectStoredDurationRebuildsHistorySortCache verifies that
// correctStoredDuration recomputes the history sortedTracks cache when a sort
// is active.
func TestCorrectStoredDurationRebuildsHistorySortCache(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Duration: 360.0})
	m.library.mode = LibraryModeHistory
	m.library.sourceTracks = []library.Track{
		{Hash: "h1", Path: "/a.flac", Duration: 360.0},
		{Hash: "h2", Path: "/b.flac", Duration: 100.0},
	}
	m.library.sortState = SortState{Field: SortDuration, Dir: SortAsc}
	m.library = m.library.recomputeSortedTracks()

	// Before: h2 (100s) < h1 (360s).
	if m.library.sortedTracks[0].Hash != "h2" {
		t.Fatal("pre-condition: expected h2 first")
	}

	nm, _ := m.correctStoredDuration("h1", 50.0)

	// After: h1 (50s) < h2 (100s).
	if nm.library.sortedTracks == nil {
		t.Fatal("expected history sortedTracks non-nil")
	}
	if nm.library.sortedTracks[0].Hash != "h1" {
		t.Errorf("expected h1 first after correction, got %v", nm.library.sortedTracks[0].Hash)
	}
}

// TestCorrectStoredDurationRebuildsArtistsSortCache verifies that
// correctStoredDuration recomputes the artists track sort cache when a
// duration sort is active.
func TestCorrectStoredDurationRebuildsArtistsSortCache(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Artist: "A", Duration: 360.0})
	m.library.index.Insert(library.Track{Hash: "h2", Path: "/b.flac", Artist: "A", Duration: 100.0})
	m.artists.level = arttui.TrackLevel
	m.artists.tracks = []library.Track{
		{Hash: "h1", Artist: "A", Duration: 360.0},
		{Hash: "h2", Artist: "A", Duration: 100.0},
	}
	m.artists.sortState = SortState{Field: SortDuration, Dir: SortAsc}
	m.artists = m.artists.recomputeArtistsSort()

	// Before: h2 (100s) < h1 (360s).
	if m.artists.tracks[0].Hash != "h2" {
		t.Fatal("pre-condition: expected h2 first in duration-asc sort")
	}

	nm, _ := m.correctStoredDuration("h1", 50.0)

	// After: h1 (50s) < h2 (100s).
	if nm.artists.tracks[0].Hash != "h1" {
		t.Errorf("expected h1 first after correction, got %v", nm.artists.tracks[0].Hash)
	}
}

// TestCorrectStoredDurationRebuildsAlbumsSortCache verifies that
// correctStoredDuration recomputes the albums track sort cache when a
// duration sort is active.
func TestCorrectStoredDurationRebuildsAlbumsSortCache(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Artist: "A", Album: "X", Duration: 360.0})
	m.library.index.Insert(library.Track{Hash: "h2", Path: "/b.flac", Artist: "A", Album: "X", Duration: 100.0})
	m.albums.level = albtui.TrackLevel
	m.albums.tracks = []library.Track{
		{Hash: "h1", Artist: "A", Album: "X", Duration: 360.0},
		{Hash: "h2", Artist: "A", Album: "X", Duration: 100.0},
	}
	m.albums.sortState = SortState{Field: SortDuration, Dir: SortAsc}
	m.albums = m.albums.recomputeAlbumsSort()

	if m.albums.tracks[0].Hash != "h2" {
		t.Fatal("pre-condition: expected h2 first in duration-asc sort")
	}

	nm, _ := m.correctStoredDuration("h1", 50.0)

	if nm.albums.tracks[0].Hash != "h1" {
		t.Errorf("expected h1 first after correction, got %v", nm.albums.tracks[0].Hash)
	}
}

// TestCorrectStoredDurationRebuildsGenresSortCache verifies that
// correctStoredDuration recomputes the genres track sort cache when a
// duration sort is active.
func TestCorrectStoredDurationRebuildsGenresSortCache(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Artist: "A", Genre: "Rock", Duration: 360.0})
	m.library.index.Insert(library.Track{Hash: "h2", Path: "/b.flac", Artist: "A", Genre: "Rock", Duration: 100.0})
	m.genres.level = gentui.TrackLevel
	m.genres.tracks = []library.Track{
		{Hash: "h1", Artist: "A", Genre: "Rock", Duration: 360.0},
		{Hash: "h2", Artist: "A", Genre: "Rock", Duration: 100.0},
	}
	m.genres.sortState = SortState{Field: SortDuration, Dir: SortAsc}
	m.genres = m.genres.recomputeGenresSort()

	if m.genres.tracks[0].Hash != "h2" {
		t.Fatal("pre-condition: expected h2 first in duration-asc sort")
	}

	nm, _ := m.correctStoredDuration("h1", 50.0)

	if nm.genres.tracks[0].Hash != "h1" {
		t.Errorf("expected h1 first after correction, got %v", nm.genres.tracks[0].Hash)
	}
}
