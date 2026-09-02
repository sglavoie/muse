package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	libtui "github.com/sglavoie/muse/internal/tui/library"
	"github.com/sglavoie/muse/internal/tui/sorting"
)

// TestFocusedTrackForPreview_Queue verifies that focusedTrackForPreview
// returns the queue entry under the cursor.
func TestFocusedTrackForPreview_Queue(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{
		{Hash: "h1", Path: "/a.flac", Artist: "A", Title: "T1", DurationMs: 5000},
		{Hash: "h2", Path: "/b.flac", Artist: "B", Title: "T2", DurationMs: 6000},
	}
	m.queue.cursor = 1

	track, ok := m.focusedTrackForPreview()
	if !ok {
		t.Fatal("expected track from queue")
	}
	if track.Hash != "h2" || track.Artist != "B" {
		t.Fatalf("unexpected track: %+v", track)
	}
	if track.Duration != 6.0 {
		t.Fatalf("expected duration 6.0, got %v", track.Duration)
	}
}

// TestFocusedTrackForPreview_QueueMissing verifies that a missing queue
// entry returns false.
func TestFocusedTrackForPreview_QueueMissing(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{
		{Hash: "h1", Path: "/a.flac", Missing: true},
	}
	m.queue.cursor = 0

	_, ok := m.focusedTrackForPreview()
	if ok {
		t.Fatal("expected false for missing queue entry")
	}
}

// TestFocusedTrackForPreview_EmptyQueue verifies that an empty queue
// returns false.
func TestFocusedTrackForPreview_EmptyQueue(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewQueue
	m.queue.entries = nil
	m.queue.cursor = 0

	_, ok := m.focusedTrackForPreview()
	if ok {
		t.Fatal("expected false for empty queue")
	}
}

// TestFocusedTrackForPreview_QueueWithFilter verifies that
// focusedTrackForPreview uses the filtered entry list when a queue filter
// is active, rather than indexing into the unfiltered entries slice.
func TestFocusedTrackForPreview_QueueWithFilter(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{
		{Hash: "h1", Path: "/a.flac", Artist: "Alice", Title: "Alpha", DurationMs: 3000},
		{Hash: "h2", Path: "/b.flac", Artist: "Bob", Title: "Beta", DurationMs: 4000},
		{Hash: "h3", Path: "/c.flac", Artist: "Carol", Title: "Charlie", DurationMs: 5000},
	}
	// Activate a filter that matches only "Bob".
	m.queue.filter.Text = "bob"
	m.queue.cursor = 0 // first (and only) filtered entry

	track, ok := m.focusedTrackForPreview()
	if !ok {
		t.Fatal("expected track from filtered queue")
	}
	if track.Hash != "h2" {
		t.Fatalf("expected hash h2 (Bob), got %q", track.Hash)
	}
}

// TestFocusedTrackForPreview_QueueWithSort verifies that
// focusedTrackForPreview respects the queue display order when sorted.
func TestFocusedTrackForPreview_QueueWithSort(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{
		{Hash: "h1", Path: "/a.flac", Artist: "Zeta", Title: "Zulu", DurationMs: 3000},
		{Hash: "h2", Path: "/b.flac", Artist: "Alpha", Title: "Able", DurationMs: 4000},
	}
	// Sort by artist ascending: Alpha first, then Zeta.
	m.queue.sortState = sorting.SortState{Field: sorting.SortArtist, Dir: sorting.SortAsc}
	m.queue = m.queue.recomputeDisplayOrder()
	m.queue.cursor = 0 // first display entry after sort = Alpha

	track, ok := m.focusedTrackForPreview()
	if !ok {
		t.Fatal("expected track from sorted queue")
	}
	// The first display entry after sorting by artist asc should be Alpha.
	if track.Hash != "h2" {
		t.Fatalf("expected hash h2 (Alpha), got %q", track.Hash)
	}
}

// TestFocusedTrackForPreview_QueueIncludesAlbum verifies that
// focusedTrackForPreview includes the Album field from queue entries
// so preview and track-info overlays display the album name.
func TestFocusedTrackForPreview_QueueIncludesAlbum(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{
		{Hash: "h1", Path: "/a.flac", Artist: "A", Title: "T", Album: "MyAlbum", DurationMs: 5000},
	}
	m.queue.cursor = 0

	track, ok := m.focusedTrackForPreview()
	if !ok {
		t.Fatal("expected track from queue")
	}
	if track.Album != "MyAlbum" {
		t.Fatalf("expected Album %q, got %q", "MyAlbum", track.Album)
	}
}

// TestFocusedTrackForPreview_Tags verifies that focusedTrackForPreview
// returns the focused track from the Tags view at TrackLevel.
func TestFocusedTrackForPreview_Tags(t *testing.T) {
	m := newTagsTrackLevelModel()
	m.tagsView.cursor = 1 // "Tag Track 2" by artist "B"

	track, ok := m.focusedTrackForPreview()
	if !ok {
		t.Fatal("expected track from tags view")
	}
	if track.Hash != "th2" {
		t.Errorf("hash: want th2, got %q", track.Hash)
	}
	if track.Artist != "B" {
		t.Errorf("artist: want B, got %q", track.Artist)
	}
	if track.Duration != 200.0 {
		t.Errorf("duration: want 200.0, got %v", track.Duration)
	}
}

// TestFocusedTrackForPreview_TagsEnrichesFromIndex verifies that
// focusedTrackForPreview enriches PlayCount and Genre from the library index
// when the Tags view is active. Without the enrichment a preview or track-info
// overlay would show 0 plays and an empty genre.
func TestFocusedTrackForPreview_TagsEnrichesFromIndex(t *testing.T) {
	m := newTagsTrackLevelModel()
	m.tagsView.cursor = 0 // "Tag Track 1", hash "th1"

	// Seed the library index with PlayCount and Genre for the same hash.
	m.library.Index.Insert(library.Track{
		Hash:      "th1",
		Path:      "/tag1.flac",
		Artist:    "A",
		Title:     "Tag Track 1",
		PlayCount: 42,
		Genre:     "Jazz",
	})

	track, ok := m.focusedTrackForPreview()
	if !ok {
		t.Fatal("expected track from tags view")
	}
	if track.PlayCount != 42 {
		t.Errorf("PlayCount: want 42, got %d", track.PlayCount)
	}
	if track.Genre != "Jazz" {
		t.Errorf("Genre: want Jazz, got %q", track.Genre)
	}
}

// TestFocusedTrackForPreview_TagsTagLevel verifies that focusedTrackForPreview
// returns false when the Tags view is at TagLevel (no track focused).
func TestFocusedTrackForPreview_TagsTagLevel(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewTags
	// level defaults to TagLevel
	m.tagsView.tags = []db.TagRow{{ID: 1, Name: "T1"}}
	m.tagsView.tagCounts = map[int64]int{1: 3}

	_, ok := m.focusedTrackForPreview()
	if ok {
		t.Fatal("expected false at TagLevel")
	}
}

// TestFocusedTrackForPreview_RecentlyAdded verifies that
// focusedTrackForPreview returns the focused track from the Recently Added view.
func TestFocusedTrackForPreview_RecentlyAdded(t *testing.T) {
	m := New(context.Background())
	m.library.Mode = libtui.LibraryModeRecent
	m.activeView = viewLibrary
	m.library.Index.Insert(library.Track{
		Hash: "h1", Path: "/a.flac", Artist: "A", Title: "T",
	})

	track, ok := m.focusedTrackForPreview()
	if !ok {
		t.Fatal("expected track from recently added view")
	}
	if track.Hash != "h1" || track.Artist != "A" {
		t.Fatalf("unexpected track: %+v", track)
	}
}

// TestFocusedTrackForPalette_Tags verifies that focusedTrackForPalette
// returns the focused track's path, artist, and title from the Tags view.
func TestFocusedTrackForPalette_Tags(t *testing.T) {
	m := newTagsTrackLevelModel()
	m.tagsView.cursor = 0 // "Tag Track 1" by artist "A", path not set in helper

	// Set a path on the track so we can verify it
	m.tagsView.tracks[0] = db.PlaylistTrackRow{
		ID: 1, Hash: "th1", Title: "Tag Track 1", Artist: "A", Album: "X",
		DurationMs: 180000, Path: "/tag1.flac",
	}

	path, artist, title, found := m.focusedTrackForPalette()
	if !found {
		t.Fatal("expected found=true for Tags track level")
	}
	if path != "/tag1.flac" {
		t.Errorf("path: want /tag1.flac, got %q", path)
	}
	if artist != "A" {
		t.Errorf("artist: want A, got %q", artist)
	}
	if title != "Tag Track 1" {
		t.Errorf("title: want Tag Track 1, got %q", title)
	}
}

// TestFocusedTrackForPalette_TagsTagLevel verifies that
// focusedTrackForPalette returns false at TagLevel.
func TestFocusedTrackForPalette_TagsTagLevel(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewTags
	m.tagsView.tags = []db.TagRow{{ID: 1, Name: "T1"}}
	m.tagsView.tagCounts = map[int64]int{1: 3}

	_, _, _, found := m.focusedTrackForPalette()
	if found {
		t.Fatal("expected found=false at TagLevel")
	}
}
