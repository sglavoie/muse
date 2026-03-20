package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// TestQueueFilterActivatesOnSlash verifies that pressing '/' in the queue view
// activates the queue filter bar.
func TestQueueFilterActivatesOnSlash(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{
		{Path: "/a.flac", Title: "Alpha", Artist: "ArtA"},
		{Path: "/b.flac", Title: "Beta", Artist: "ArtB"},
	}

	next, _ := m.Update(FilterBarMsg{})
	nm := next.(Model)

	if !nm.queue.filter.Active {
		t.Fatal("queue filter should be active after FilterBarMsg")
	}
}

// TestQueueFilterNarrowsDisplayedEntries verifies that typing a filter query
// reduces the rendered entries to those matching the query.
func TestQueueFilterNarrowsDisplayedEntries(t *testing.T) {
	q := QueueView{
		entries: []QueueEntry{
			{Path: "/a.flac", Title: "Alpha", Artist: "ArtA", Album: "AlbA"},
			{Path: "/b.flac", Title: "Beta", Artist: "ArtB", Album: "AlbB"},
			{Path: "/c.flac", Title: "Gamma", Artist: "ArtC", Album: "AlbC"},
		},
	}
	q.filter = q.filter.Activate()
	q.filter.Text = "beta"
	q.cursor = 0
	q.scrollOffset = 0

	filtered := q.filteredEntries()
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
	q := QueueView{
		entries: []QueueEntry{
			{Path: "/a.flac", Title: "Song1", Artist: "Radiohead", Album: "OK Computer"},
			{Path: "/b.flac", Title: "Song2", Artist: "Coldplay", Album: "Parachutes"},
		},
	}
	q.filter = q.filter.Activate()
	q.filter.Text = "radio"
	q.cursor = 0
	q.scrollOffset = 0

	filtered := q.filteredEntries()
	if len(filtered) != 1 {
		t.Fatalf("expected 1 filtered entry matching artist, got %d", len(filtered))
	}
	if filtered[0].Artist != "Radiohead" {
		t.Fatalf("expected Radiohead, got %s", filtered[0].Artist)
	}
}

// TestQueueFilterEscapeClears verifies that pressing Escape clears the filter
// and resets the cursor.
func TestQueueFilterEscapeClears(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{
		{Path: "/a.flac", Title: "Alpha", Artist: "ArtA"},
		{Path: "/b.flac", Title: "Beta", Artist: "ArtB"},
	}

	// Activate and type a filter.
	next, _ := m.Update(FilterBarMsg{})
	m = next.(Model)
	next, _ = m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	m = next.(Model)
	if m.queue.filter.Text != "a" {
		t.Fatalf("filter text should be 'a', got %q", m.queue.filter.Text)
	}

	// Press Escape to clear.
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = next.(Model)
	if m.queue.filter.Active {
		t.Error("filter should be inactive after Escape")
	}
	if m.queue.filter.Text != "" {
		t.Errorf("filter text should be empty after Escape, got %q", m.queue.filter.Text)
	}
}

// TestQueueFilterEnterCommits verifies that pressing Enter commits the filter
// (exits input mode but keeps the text).
func TestQueueFilterEnterCommits(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{
		{Path: "/a.flac", Title: "Alpha", Artist: "ArtA"},
	}

	// Activate and type.
	next, _ := m.Update(FilterBarMsg{})
	m = next.(Model)
	next, _ = m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	m = next.(Model)

	// Press Enter to commit.
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	if m.queue.filter.Active {
		t.Error("filter input mode should be inactive after Enter")
	}
	if m.queue.filter.Text != "x" {
		t.Errorf("filter text should be preserved after Enter, got %q", m.queue.filter.Text)
	}
}

// TestQueueFilterCursorStaysInBounds verifies that the cursor is clamped when
// filter narrows the visible entries.
func TestQueueFilterCursorStaysInBounds(t *testing.T) {
	q := QueueView{
		entries: []QueueEntry{
			{Path: "/a.flac", Title: "Alpha", Artist: "A"},
			{Path: "/b.flac", Title: "Beta", Artist: "B"},
			{Path: "/c.flac", Title: "Gamma", Artist: "C"},
		},
		cursor: 2, // pointing at Gamma
	}
	q.filter = q.filter.Activate()
	// Type "alpha" — only 1 match, cursor must reset to 0.
	q.filter.Text = "alpha"
	q.cursor = 0
	q.scrollOffset = 0

	if q.cursor != 0 {
		t.Fatalf("cursor should be 0 after filter narrows to 1 entry, got %d", q.cursor)
	}
}

// TestQueueFilterRendersFilterBar verifies that the filter bar appears in the
// rendered queue output when the filter is active.
func TestQueueFilterRendersFilterBar(t *testing.T) {
	q := QueueView{
		entries: []QueueEntry{
			{Path: "/a.flac", Title: "Song", Artist: "Art", Album: "Alb"},
		},
	}
	q.filter = q.filter.Activate()
	q.filter.Text = "song"
	q.cursor = 0
	q.scrollOffset = 0

	s := styles.NewStyles(true)
	out := q.render(s, "")
	if !strings.Contains(out, "Filter: > song") {
		t.Errorf("rendered output should contain filter bar, got:\n%s", out)
	}
}

// TestQueueFilterNoMatchesShowsMessage verifies that when filter matches
// nothing, "(no matches)" is displayed.
func TestQueueFilterNoMatchesShowsMessage(t *testing.T) {
	q := QueueView{
		entries: []QueueEntry{
			{Path: "/a.flac", Title: "Song", Artist: "Art"},
		},
	}
	q.filter = q.filter.Activate()
	q.filter.Text = "zzzzz"
	q.cursor = 0
	q.scrollOffset = 0

	s := styles.NewStyles(true)
	out := q.render(s, "")
	if !strings.Contains(out, "(no matches)") {
		t.Errorf("expected '(no matches)' when filter has no results, got:\n%s", out)
	}
}

// TestQueueFilterClearedOnClearQueue verifies that clearing the queue also
// clears any active filter.
func TestQueueFilterClearedOnClearQueue(t *testing.T) {
	d := &stubQueueDequeuer{}
	m := New(context.Background())
	m.SetQueueDequeuer(d)
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{
		{ID: 10, Path: "/a.flac", Title: "Song", Artist: "Art"},
	}
	m.queue.filter.Active = true
	m.queue.filter.Text = "song"

	next, _ := m.Update(ClearQueueMsg{})
	nm := next.(Model)

	if nm.queue.filter.Active {
		t.Error("filter should be deactivated after queue clear")
	}
	if nm.queue.filter.Text != "" {
		t.Errorf("filter text should be empty after queue clear, got %q", nm.queue.filter.Text)
	}
}

// TestQueueFilterSelectMapsToCorrectEntry verifies that selecting an entry
// while filter is active dispatches PlayQueueIndexMsg with the correct
// underlying entry index.
func TestQueueFilterSelectMapsToCorrectEntry(t *testing.T) {
	q := QueueView{
		entries: []QueueEntry{
			{ID: 1, Path: "/a.flac", Hash: "ha", Title: "Alpha", Artist: "A"},
			{ID: 2, Path: "/b.flac", Hash: "hb", Title: "Beta", Artist: "B"},
			{ID: 3, Path: "/c.flac", Hash: "hc", Title: "Gamma", Artist: "C"},
		},
	}
	q.filter = q.filter.Activate()
	q.filter.Text = "beta"
	q.cursor = 0
	q.scrollOffset = 0

	// Cursor 0 in filtered view should be "Beta" (real index 1).
	q.cursor = 0
	_, cmd := q.HandleAction(actionSelect)
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}
	msg := cmd()
	pqi, ok := msg.(PlayQueueIndexMsg)
	if !ok {
		t.Fatalf("expected PlayQueueIndexMsg, got %T", msg)
	}
	if pqi.Index != 1 {
		t.Fatalf("expected real index 1, got %d", pqi.Index)
	}
}

// TestQueueFilterMoveDisabled verifies that move-up/move-down are disabled
// when a filter is active.
func TestQueueFilterMoveDisabled(t *testing.T) {
	q := QueueView{
		entries: []QueueEntry{
			{Path: "/a.flac", Title: "Alpha", Artist: "A"},
			{Path: "/b.flac", Title: "Beta", Artist: "B"},
		},
		cursor: 0,
	}
	q.filter = q.filter.Activate()
	q.filter.Text = "a" // still matches both
	q.cursor = 0
	q.scrollOffset = 0

	_, cmd := q.HandleAction(actionMoveTrackDown)
	if cmd != nil {
		t.Error("move-down should be disabled when filter is active")
	}

	q.cursor = 1
	_, cmd = q.HandleAction(actionMoveTrackUp)
	if cmd != nil {
		t.Error("move-up should be disabled when filter is active")
	}
}
