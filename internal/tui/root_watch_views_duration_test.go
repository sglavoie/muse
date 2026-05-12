package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
)

// TestWatchBatchUpdatesQueueDuration verifies that a watcher metadata update
// propagates duration changes to in-memory queue entries.
func TestWatchBatchUpdatesQueueDuration(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Duration: 120})
	m.queue.entries = []QueueEntry{
		{ID: 1, Hash: "h1", Path: "/a.flac", DurationMs: 120_000},
	}

	next, _ := m.Update(watchBatchMsg{
		tracks: []library.Track{
			{Hash: "h1", Path: "/a.flac", Duration: 180},
		},
	})
	nm := next.(Model)

	if nm.queue.entries[0].DurationMs != 180_000 {
		t.Errorf("queue entry DurationMs = %d, want 180000", nm.queue.entries[0].DurationMs)
	}
}

// TestWatchBatchUpdatesHistoryDuration verifies that a watcher metadata update
// propagates duration changes to in-memory history tracks.
func TestWatchBatchUpdatesHistoryDuration(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Duration: 120})
	m.library.mode = LibraryModeHistory
	m.library.sourceTracks = []library.Track{
		{Hash: "h1", Path: "/a.flac", Duration: 120},
	}

	next, _ := m.Update(watchBatchMsg{
		tracks: []library.Track{
			{Hash: "h1", Path: "/a.flac", Duration: 200},
		},
	})
	nm := next.(Model)

	if nm.library.sourceTracks[0].Duration != 200 {
		t.Errorf("history track Duration = %v, want 200", nm.library.sourceTracks[0].Duration)
	}
}

// TestWatchBatchUpdatesPlaylistDuration verifies that a watcher metadata update
// propagates duration changes to in-memory playlist tracks.
func TestWatchBatchUpdatesPlaylistDuration(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Duration: 120})
	m.playlists.level = pltui.TrackLevel
	m.playlists.tracks = []db.PlaylistTrackRow{
		{ID: 1, Hash: "h1", Path: "/a.flac", DurationMs: 120_000},
	}

	next, _ := m.Update(watchBatchMsg{
		tracks: []library.Track{
			{Hash: "h1", Path: "/a.flac", Duration: 200},
		},
	})
	nm := next.(Model)

	if nm.playlists.tracks[0].DurationMs != 200_000 {
		t.Errorf("playlist track DurationMs = %d, want 200000", nm.playlists.tracks[0].DurationMs)
	}
}

// TestWatchBatchUpdatesFavoritesDuration verifies that a watcher metadata update
// propagates duration changes to in-memory favorites tracks.
func TestWatchBatchUpdatesFavoritesDuration(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Duration: 120})
	m.favorites.level = favtui.ListLevel
	m.favorites.category = favtui.CategoryTracks
	m.favorites.tracks = []library.Track{
		{Hash: "h1", Path: "/a.flac", Duration: 120},
	}

	next, _ := m.Update(watchBatchMsg{
		tracks: []library.Track{
			{Hash: "h1", Path: "/a.flac", Duration: 200},
		},
	})
	nm := next.(Model)

	if nm.favorites.tracks[0].Duration != 200 {
		t.Errorf("favorites track Duration = %v, want 200", nm.favorites.tracks[0].Duration)
	}
}

// TestWatchBatchUpdatesFavoritesDrilldownDuration verifies that a watcher
// metadata update propagates duration changes to favorites drilldown tracks.
func TestWatchBatchUpdatesFavoritesDrilldownDuration(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Duration: 120})
	m.favorites.level = favtui.TrackLevel
	m.favorites.drilldownTracks = []library.Track{
		{Hash: "h1", Path: "/a.flac", Duration: 120},
	}

	next, _ := m.Update(watchBatchMsg{
		tracks: []library.Track{
			{Hash: "h1", Path: "/a.flac", Duration: 200},
		},
	})
	nm := next.(Model)

	if nm.favorites.drilldownTracks[0].Duration != 200 {
		t.Errorf("favorites drilldown track Duration = %v, want 200", nm.favorites.drilldownTracks[0].Duration)
	}
}

// TestWatchBatchUpdatesTagsDuration verifies that a watcher metadata update
// propagates duration changes to in-memory tags tracks.
func TestWatchBatchUpdatesTagsDuration(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Duration: 120})
	m.tagsView.level = tagtui.TrackLevel
	m.tagsView.tracks = []db.PlaylistTrackRow{
		{ID: 1, Hash: "h1", Path: "/a.flac", DurationMs: 120_000},
	}

	next, _ := m.Update(watchBatchMsg{
		tracks: []library.Track{
			{Hash: "h1", Path: "/a.flac", Duration: 200},
		},
	})
	nm := next.(Model)

	if nm.tagsView.tracks[0].DurationMs != 200_000 {
		t.Errorf("tags track DurationMs = %d, want 200000", nm.tagsView.tracks[0].DurationMs)
	}
}
