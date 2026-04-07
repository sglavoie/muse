package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/library"
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
	m.queue.sortState = SortState{Field: SortArtist, Dir: SortAsc}
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

// TestFocusedTrackForPreview_RecentlyAdded verifies that
// focusedTrackForPreview returns the focused track from the Recently Added view.
func TestFocusedTrackForPreview_RecentlyAdded(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewRecentlyAdded
	m.recentlyAdded.index.Insert(library.Track{
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
