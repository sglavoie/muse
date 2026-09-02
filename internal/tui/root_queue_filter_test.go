package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/tui/keys"
	"github.com/sglavoie/muse/internal/tui/queue"
)

// TestQueueFilterActivatesOnSlash verifies that pressing '/' in the queue view
// activates the queue filter bar.
func TestQueueFilterActivatesOnSlash(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewQueue
	m.queue.Entries = []queue.Entry{
		{Path: "/a.flac", Title: "Alpha", Artist: "ArtA"},
		{Path: "/b.flac", Title: "Beta", Artist: "ArtB"},
	}

	next, _ := m.Update(FilterBarMsg{})
	nm := next.(Model)

	if !nm.queue.Filter.Active {
		t.Fatal("queue filter should be active after FilterBarMsg")
	}
}

// TestQueueFilterNarrowsDisplayedEntries verifies that typing a filter query
// reduces the rendered entries to those matching the query.
func TestQueueFilterNarrowsDisplayedEntries(t *testing.T) {
	q := queue.View{
		Entries: []queue.Entry{
			{Path: "/a.flac", Title: "Alpha", Artist: "ArtA", Album: "AlbA"},
			{Path: "/b.flac", Title: "Beta", Artist: "ArtB", Album: "AlbB"},
			{Path: "/c.flac", Title: "Gamma", Artist: "ArtC", Album: "AlbC"},
		},
	}
	q.Filter, _ = q.Filter.Activate()
	q.Filter.Text = "beta"
	q.Cursor = 0
	q.ScrollOffset = 0

	filtered := q.FilteredEntries()
	if len(filtered) != 1 {
		t.Fatalf("expected 1 filtered entry, got %d", len(filtered))
	}
	if filtered[0].Title != "Beta" {
		t.Fatalf("expected Beta, got %s", filtered[0].Title)
	}
}

// TestQueueFilterMatchesArtistAndAlbum verifies that the filter matches against
// artist and album fields, not just title.
func TestQueueFilterMatchesArtistAndAlbum(t *testing.T) {
	q := queue.View{
		Entries: []queue.Entry{
			{Path: "/a.flac", Title: "Song1", Artist: "Radiohead", Album: "OK Computer"},
			{Path: "/b.flac", Title: "Song2", Artist: "Coldplay", Album: "Parachutes"},
		},
	}
	q.Filter, _ = q.Filter.Activate()
	q.Filter.Text = "radio"
	q.Cursor = 0
	q.ScrollOffset = 0

	filtered := q.FilteredEntries()
	if len(filtered) != 1 {
		t.Fatalf("expected 1 filtered entry matching artist, got %d", len(filtered))
	}
	if filtered[0].Artist != "Radiohead" {
		t.Fatalf("expected Radiohead, got %s", filtered[0].Artist)
	}
}

// TestQueueFilterCursorStaysInBounds verifies that the cursor is clamped when
// filter narrows the visible entries.
func TestQueueFilterCursorStaysInBounds(t *testing.T) {
	q := queue.View{
		Entries: []queue.Entry{
			{Path: "/a.flac", Title: "Alpha", Artist: "A"},
			{Path: "/b.flac", Title: "Beta", Artist: "B"},
			{Path: "/c.flac", Title: "Gamma", Artist: "C"},
		},
		Cursor: 2, // pointing at Gamma
	}
	q.Filter, _ = q.Filter.Activate()
	// Type "alpha" — only 1 match, cursor must be clamped to 0.
	q.Filter.Text = "alpha"

	// moveQueueCursor(0) clamps cursor to valid range without moving.
	q = q.MoveQueueCursor(0)

	if q.Cursor != 0 {
		t.Fatalf("cursor should be 0 after filter narrows to 1 entry, got %d", q.Cursor)
	}
}

// TestQueueFilterClearedOnClearQueue verifies that clearing the queue also
// clears any active filter.
func TestQueueFilterClearedOnClearQueue(t *testing.T) {
	d := &stubQueueDequeuer{}
	m := New(context.Background(), WithQueueDequeuer(d))
	m.activeView = viewQueue
	m.queue.Entries = []queue.Entry{
		{ID: 10, Path: "/a.flac", Title: "Song", Artist: "Art"},
	}
	m.queue.Filter.Active = true
	m.queue.Filter.Text = "song"

	next, _ := m.Update(queue.ClearQueueMsg{})
	nm := next.(Model)

	if nm.queue.Filter.Active {
		t.Error("filter should be deactivated after queue clear")
	}
	if nm.queue.Filter.Text != "" {
		t.Errorf("filter text should be empty after queue clear, got %q", nm.queue.Filter.Text)
	}
}

// TestQueueCursorClampedAgainstFilteredEntriesAfterDequeue verifies that
// after dequeuing a track while a filter is active, the cursor is clamped
// to the filtered entry count rather than the total entry count.
func TestQueueCursorClampedAgainstFilteredEntriesAfterDequeue(t *testing.T) {
	d := &stubQueueDequeuer{}
	m := New(context.Background(), WithQueueDequeuer(d))
	m.activeView = viewQueue

	// 5 entries: 2 match filter "alpha", 3 don't.
	m.queue.Entries = []queue.Entry{
		{ID: 1, Path: "/a.flac", Hash: "ha", Title: "Alpha One", Artist: "A"},
		{ID: 2, Path: "/b.flac", Hash: "hb", Title: "Beta", Artist: "B"},
		{ID: 3, Path: "/c.flac", Hash: "hc", Title: "Alpha Two", Artist: "C"},
		{ID: 4, Path: "/d.flac", Hash: "hd", Title: "Gamma", Artist: "D"},
		{ID: 5, Path: "/e.flac", Hash: "he", Title: "Delta", Artist: "E"},
	}

	// Activate filter for "alpha" -> 2 visible entries.
	m.queue.Filter, _ = m.queue.Filter.Activate()
	m.queue.Filter.Text = "alpha"
	m.queue.Cursor = 1 // pointing at "Alpha Two" (index 1 in filtered)

	// Dequeue "Alpha Two" (real index 2 in m.queue.entries).
	next, _ := m.Update(queue.DequeueTrackMsg{Index: 2})
	nm := next.(Model)

	// After dequeue, "Alpha Two" is gone. Filtered list has 1 entry ("Alpha One").
	filtered := nm.queue.FilteredEntries()
	if len(filtered) != 1 {
		t.Fatalf("expected 1 filtered entry after dequeue, got %d", len(filtered))
	}

	// Cursor must be clamped to 0 (the only filtered entry).
	if nm.queue.Cursor != 0 {
		t.Errorf("cursor should be clamped to 0 after filtered dequeue, got %d", nm.queue.Cursor)
	}
}

// TestQueueCursorClampedAgainstFilteredEntriesAfterBulkDequeue verifies that
// after bulk-dequeuing tracks while a filter is active, the cursor is clamped
// to the filtered entry count.
func TestQueueCursorClampedAgainstFilteredEntriesAfterBulkDequeue(t *testing.T) {
	d := &stubQueueDequeuer{}
	m := New(context.Background(), WithQueueDequeuer(d))
	m.activeView = viewQueue

	m.queue.Entries = []queue.Entry{
		{ID: 1, Path: "/a.flac", Hash: "ha", Title: "Alpha One", Artist: "A", Selected: true},
		{ID: 2, Path: "/b.flac", Hash: "hb", Title: "Beta", Artist: "B"},
		{ID: 3, Path: "/c.flac", Hash: "hc", Title: "Alpha Two", Artist: "C", Selected: true},
		{ID: 4, Path: "/d.flac", Hash: "hd", Title: "Gamma", Artist: "D"},
	}

	// Activate filter for "alpha" -> 2 visible entries, both selected.
	m.queue.Filter, _ = m.queue.Filter.Activate()
	m.queue.Filter.Text = "alpha"
	m.queue.Cursor = 1 // pointing at "Alpha Two" in filtered view

	// Bulk dequeue all selected (actionDequeue with selection).
	next, _ := m.handleKeyAction(keys.ActionDequeue)
	nm := next.(Model)

	// Both "Alpha" entries are removed. Filter "alpha" matches nothing.
	filtered := nm.queue.FilteredEntries()
	if len(filtered) != 0 {
		t.Fatalf("expected 0 filtered entries after bulk dequeue, got %d", len(filtered))
	}

	// Cursor must be 0 when filtered list is empty.
	if nm.queue.Cursor != 0 {
		t.Errorf("cursor should be 0 when filtered list is empty, got %d", nm.queue.Cursor)
	}
}
