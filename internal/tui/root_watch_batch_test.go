package tui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/keys"
	"github.com/sglavoie/muse/internal/tui/sorting"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
)

// TestWatchBatchInvalidatesFilterCache verifies that a watchBatchMsg that
// removes a track invalidates the cached fuzzy-match results so that stale
// TrackIndex values cannot cause an out-of-bounds panic on the next render
// or FocusedTrack call.
func TestWatchBatchInvalidatesFilterCache(t *testing.T) {
	watchCh := make(chan library.WatchEvent, 1)
	m := New(context.Background(), WithWatchChannel(watchCh))
	m.library.fuzzy = library.FakeFuzzySearcher{}

	// Seed three tracks.
	m.library.index.Insert(library.Track{Artist: "A", Title: "Alpha", Path: "/a", Hash: "h1"})
	m.library.index.Insert(library.Track{Artist: "B", Title: "Beta", Path: "/b", Hash: "h2"})
	m.library.index.Insert(library.Track{Artist: "C", Title: "Charlie", Path: "/c", Hash: "h3"})

	// Activate and commit a filter that matches all three.
	m.library.filter, _ = m.library.filter.Activate()
	m.library.filter, _ = m.library.filter.Update(tea.KeyPressMsg{Text: "a"})
	m.library.cursor = 0
	m.library.scrollOffset = 0
	m.library = m.library.rebuildFilterCache()
	m.library.filter = m.library.filter.Commit()

	// Move cursor to the last match.
	for range 2 {
		m.library = m.library.HandleAction(keys.ActionCursorDown)
	}
	if m.library.cursor != 2 {
		t.Fatalf("setup: expected cursor=2, got %d", m.library.cursor)
	}

	// Deliver a watchBatchMsg that deletes two tracks, leaving only one match.
	next, _ := m.Update(watchBatchMsg{
		deleted: []string{"/b", "/c"},
	})
	nm := next.(Model)

	// The cursor must be clamped so it does not point past the end of the
	// new (shorter) match set.
	if nm.library.cursor >= len(nm.library.filteredMatches) && len(nm.library.filteredMatches) > 0 {
		t.Fatalf("cursor %d out of range for %d filtered matches",
			nm.library.cursor, len(nm.library.filteredMatches))
	}

	// FocusedTrack must not panic.
	track, ok := nm.library.view().FocusedTrack()
	if !ok && nm.library.index.Len() > 0 && len(nm.library.filteredMatches) > 0 {
		t.Fatal("FocusedTrack returned false despite non-empty filtered matches")
	}
	if ok && track.Path == "/b" {
		t.Fatal("FocusedTrack returned a deleted track")
	}
	if ok && track.Path == "/c" {
		t.Fatal("FocusedTrack returned a deleted track")
	}
}

// TestWatchBatchClampsUnfilteredCursor verifies that when no filter is
// active and the watcher deletes tracks, the library cursor is clamped
// so it does not point past the end of the track list.
func TestWatchBatchClampsUnfilteredCursor(t *testing.T) {
	watchCh := make(chan library.WatchEvent, 1)
	m := New(context.Background(), WithWatchChannel(watchCh))

	// Seed three tracks.
	m.library.index.Insert(library.Track{Artist: "A", Title: "Alpha", Path: "/a", Hash: "h1"})
	m.library.index.Insert(library.Track{Artist: "B", Title: "Beta", Path: "/b", Hash: "h2"})
	m.library.index.Insert(library.Track{Artist: "C", Title: "Charlie", Path: "/c", Hash: "h3"})

	// Place cursor at the last track.
	m.library.cursor = 2

	// Delete two tracks, leaving only one.
	next, _ := m.Update(watchBatchMsg{deleted: []string{"/b", "/c"}})
	nm := next.(Model)

	if nm.library.cursor >= nm.library.index.Len() {
		t.Fatalf("cursor %d out of range for %d tracks",
			nm.library.cursor, nm.library.index.Len())
	}
}

// TestWatchBatchInvalidatesSortedTracks verifies that when a sort is
// active and the watcher deletes tracks, the sorted cache is rebuilt
// so it does not contain stale entries.
func TestWatchBatchInvalidatesSortedTracks(t *testing.T) {
	watchCh := make(chan library.WatchEvent, 1)
	m := New(context.Background(), WithWatchChannel(watchCh))

	// Seed three tracks.
	m.library.index.Insert(library.Track{Artist: "C", Title: "Charlie", Path: "/c", Hash: "h3"})
	m.library.index.Insert(library.Track{Artist: "A", Title: "Alpha", Path: "/a", Hash: "h1"})
	m.library.index.Insert(library.Track{Artist: "B", Title: "Beta", Path: "/b", Hash: "h2"})

	// Activate sort by artist ascending.
	m.library.sortState = sorting.SortState{Field: sorting.SortArtist, Dir: sorting.SortAsc}
	m.library = m.library.recomputeSortedTracks()
	if len(m.library.sortedTracks) != 3 {
		t.Fatalf("setup: expected 3 sorted tracks, got %d", len(m.library.sortedTracks))
	}

	// Delete one track.
	next, _ := m.Update(watchBatchMsg{deleted: []string{"/b"}})
	nm := next.(Model)

	// The sorted cache must reflect the deletion.
	if len(nm.library.sortedTracks) != 2 {
		t.Fatalf("expected 2 sorted tracks after deletion, got %d", len(nm.library.sortedTracks))
	}
	for _, sortedTrack := range nm.library.sortedTracks {
		if sortedTrack.Hash == "h2" {
			t.Fatal("deleted track h2 still present in sortedTracks")
		}
	}
}

// TestWatchBatchMarksQueueEntriesMissing verifies that when the watcher
// reports deleted files, the corresponding queue entries are immediately
// marked as Missing so they appear grayed out in the UI.
func TestWatchBatchMarksQueueEntriesMissing(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{
		Artist: "A", Title: "T1", Path: "/a.flac", Hash: "h1",
	})
	m.library.index.Insert(library.Track{
		Artist: "B", Title: "T2", Path: "/b.flac", Hash: "h2",
	})
	m.queue.entries = []QueueEntry{
		{ID: 1, Hash: "h1", Path: "/a.flac", Artist: "A", Title: "T1"},
		{ID: 2, Hash: "h2", Path: "/b.flac", Artist: "B", Title: "T2"},
	}

	// Delete /a.flac via watcher.
	next, _ := m.Update(watchBatchMsg{deleted: []string{"/a.flac"}})
	nm := next.(Model)

	if !nm.queue.entries[0].Missing {
		t.Error("queue entry for /a.flac should be marked Missing after watcher deletion")
	}
	if nm.queue.entries[1].Missing {
		t.Error("queue entry for /b.flac should NOT be marked Missing")
	}
}

// TestWatchBatchUpdatesQueueMetadata verifies that when the watcher
// reports a modified track, the queue entries for that track are updated
// with the new metadata.
func TestWatchBatchUpdatesQueueMetadata(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{
		Artist: "OldArtist", Title: "OldTitle", Album: "OldAlbum",
		Path: "/a.flac", Hash: "h1",
	})
	m.queue.entries = []QueueEntry{
		{ID: 1, Hash: "h1", Path: "/a.flac",
			Artist: "OldArtist", Title: "OldTitle", Album: "OldAlbum",
			Display: "OldArtist – OldTitle"},
	}

	// Watcher reports an update with new metadata.
	next, _ := m.Update(watchBatchMsg{
		tracks: []library.Track{
			{Artist: "NewArtist", Title: "NewTitle", Album: "NewAlbum",
				Path: "/a.flac", Hash: "h1"},
		},
	})
	nm := next.(Model)

	e := nm.queue.entries[0]
	if e.Artist != "NewArtist" {
		t.Errorf("expected Artist=%q, got %q", "NewArtist", e.Artist)
	}
	if e.Title != "NewTitle" {
		t.Errorf("expected Title=%q, got %q", "NewTitle", e.Title)
	}
	if e.Album != "NewAlbum" {
		t.Errorf("expected Album=%q, got %q", "NewAlbum", e.Album)
	}
	if e.Display != "NewArtist – NewTitle" {
		t.Errorf("expected Display=%q, got %q", "NewArtist – NewTitle", e.Display)
	}
}

// TestWatchBatchUpdatesStatusBarForCurrentTrack verifies that when the
// watcher reports metadata changes for the currently-playing track, the
// status bar is updated.
func TestWatchBatchUpdatesStatusBarForCurrentTrack(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{
		Artist: "OldArtist", Title: "OldTitle",
		Path: "/a.flac", Hash: "h1",
	})
	m.queue.entries = []QueueEntry{
		{ID: 1, Hash: "h1", Path: "/a.flac",
			Artist: "OldArtist", Title: "OldTitle"},
	}
	m.playback.currentHash = "h1"
	m.statusBar.artist = "OldArtist"
	m.statusBar.title = "OldTitle"

	next, _ := m.Update(watchBatchMsg{
		tracks: []library.Track{
			{Artist: "NewArtist", Title: "NewTitle",
				Path: "/a.flac", Hash: "h1"},
		},
	})
	nm := next.(Model)

	if nm.statusBar.artist != "NewArtist" {
		t.Errorf("expected status bar artist=%q, got %q", "NewArtist", nm.statusBar.artist)
	}
	if nm.statusBar.title != "NewTitle" {
		t.Errorf("expected status bar title=%q, got %q", "NewTitle", nm.statusBar.title)
	}
}

// TestWatchBatchClearsMissingOnReappearance verifies that when a
// previously-missing track reappears via the watcher (same hash, new
// path), its queue entries have the Missing flag cleared.
func TestWatchBatchClearsMissingOnReappearance(t *testing.T) {
	m := New(context.Background())
	m.queue.entries = []QueueEntry{
		{ID: 1, Hash: "h1", Path: "/a.flac",
			Artist: "A", Title: "T1", Missing: true},
	}

	// Watcher reports the track as re-created.
	next, _ := m.Update(watchBatchMsg{
		tracks: []library.Track{
			{Artist: "A", Title: "T1", Path: "/a.flac", Hash: "h1"},
		},
	})
	nm := next.(Model)

	if nm.queue.entries[0].Missing {
		t.Error("queue entry should no longer be Missing after track reappears")
	}
}

// TestWatchBatchSyncsPlayerQueue verifies that the player's in-memory
// queue is rebuilt when the watcher updates queue entry metadata.
func TestWatchBatchSyncsPlayerQueue(t *testing.T) {
	sp := &stubPlayerQueueSyncer{}
	m := New(context.Background(), WithPlayer(sp))
	m.library.index.Insert(library.Track{
		Artist: "Old", Title: "Old", Path: "/a.flac", Hash: "h1",
	})
	m.queue.entries = []QueueEntry{
		{ID: 1, Hash: "h1", Path: "/a.flac", Artist: "Old", Title: "Old"},
	}

	next, _ := m.Update(watchBatchMsg{
		tracks: []library.Track{
			{Artist: "New", Title: "New", Path: "/a.flac", Hash: "h1"},
		},
	})
	_ = next.(Model)

	if len(sp.setQueueCalls) == 0 {
		t.Fatal("expected SetQueue to be called after watcher metadata update")
	}
}

func TestWatchBatchPreservesCreatedAtForUpdatedPath(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "oldhash", Path: "/a.mp3", Title: "Old", CreatedAt: 123})
	m = m.syncRecentlyAddedFromLibrary()

	next, _ := m.Update(watchBatchMsg{tracks: []library.Track{{Hash: "newhash", Path: "/a.mp3", Title: "New"}}})
	nm := next.(Model)

	track, ok := nm.library.index.FindByHash("newhash")
	if !ok {
		t.Fatal("newhash not found in index")
	}
	if track.CreatedAt != 123 {
		t.Fatalf("created_at: got %d, want 123", track.CreatedAt)
	}
}

// TestWatchBatchPreservesMetadataWithUserEditedOverlay verifies that when a
// watcher event arrives for a track that has a user-edited overlay AND had
// its CreatedAt preserved by preserveTrackMetadata, both the overlay edits
// and the preserved CreatedAt/Duration survive into the library index.
func TestWatchBatchPreservesMetadataWithUserEditedOverlay(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{
		Hash: "h1", Path: "/a.mp3",
		Title: "Original", Artist: "OrigArtist", Album: "OrigAlbum",
		CreatedAt: 42, Duration: 3.5,
	})
	m = m.syncRecentlyAddedFromLibrary()

	m.userEditedOverlay = map[string]db.UserEditedRow{
		"h1": {Hash: "h1", Title: "Edited Title", Artist: "Edited Artist", Album: "Edited Album", Genre: "Edited Genre"},
	}

	// Watcher reports an update with new scanned values (no CreatedAt, no Duration).
	next, _ := m.Update(watchBatchMsg{
		tracks: []library.Track{
			{Hash: "h1", Path: "/a.mp3", Title: "Scanned", Artist: "ScannedArtist", Album: "ScannedAlbum"},
		},
	})
	nm := next.(Model)

	track, ok := nm.library.index.FindByHash("h1")
	if !ok {
		t.Fatal("h1 not found in index")
	}
	if track.Title != "Edited Title" {
		t.Errorf("Title = %q; want %q", track.Title, "Edited Title")
	}
	if track.Artist != "Edited Artist" {
		t.Errorf("Artist = %q; want %q", track.Artist, "Edited Artist")
	}
	if track.Album != "Edited Album" {
		t.Errorf("Album = %q; want %q", track.Album, "Edited Album")
	}
	if track.Genre != "Edited Genre" {
		t.Errorf("Genre = %q; want %q", track.Genre, "Edited Genre")
	}
	if track.CreatedAt != 42 {
		t.Errorf("CreatedAt = %d; want 42 (preserved)", track.CreatedAt)
	}
	if track.Duration != 3.5 {
		t.Errorf("Duration = %f; want 3.5 (preserved)", track.Duration)
	}
}

// TestWatchBatchUpdatesTagsTrackMetadata verifies that when the watcher
// reports metadata changes for a track currently shown in the Tags
// track-level view, the tagsView.tracks slice is updated in place.
func TestWatchBatchUpdatesTagsTrackMetadata(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{
		Artist: "OldArtist", Title: "OldTitle", Album: "OldAlbum",
		Path: "/a.flac", Hash: "h1",
	})
	m.tagsView.level = tagtui.TrackLevel
	m.tagsView.tracks = []db.PlaylistTrackRow{
		{Hash: "h1", Path: "/a.flac", Artist: "OldArtist", Title: "OldTitle", Album: "OldAlbum"},
	}

	next, _ := m.Update(watchBatchMsg{
		tracks: []library.Track{
			{Artist: "NewArtist", Title: "NewTitle", Album: "NewAlbum",
				Path: "/a.flac", Hash: "h1"},
		},
	})
	nm := next.(Model)

	tr := nm.tagsView.tracks[0]
	if tr.Artist != "NewArtist" {
		t.Errorf("expected tags track Artist=%q, got %q", "NewArtist", tr.Artist)
	}
	if tr.Title != "NewTitle" {
		t.Errorf("expected tags track Title=%q, got %q", "NewTitle", tr.Title)
	}
	if tr.Album != "NewAlbum" {
		t.Errorf("expected tags track Album=%q, got %q", "NewAlbum", tr.Album)
	}
}
