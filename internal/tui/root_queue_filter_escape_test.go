package tui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/tui/queue"
)

// TestQueueFilterEscapeClears verifies that pressing Escape clears the filter
// and resets the cursor.
func TestQueueFilterEscapeClears(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewQueue
	m.queue.Entries = []queue.Entry{
		{Path: "/a.flac", Title: "Alpha", Artist: "ArtA"},
		{Path: "/b.flac", Title: "Beta", Artist: "ArtB"},
	}

	// Activate and type a filter.
	next, _ := m.Update(FilterBarMsg{})
	m = next.(Model)
	next, _ = m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	m = next.(Model)
	if m.queue.Filter.Text != "a" {
		t.Fatalf("filter text should be 'a', got %q", m.queue.Filter.Text)
	}

	// Press Escape to clear.
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = next.(Model)
	if m.queue.Filter.Active {
		t.Error("filter should be inactive after Escape")
	}
	if m.queue.Filter.Text != "" {
		t.Errorf("filter text should be empty after Escape, got %q", m.queue.Filter.Text)
	}
}

// TestQueueFilterEnterCommits verifies that pressing Enter commits the filter
// (exits input mode but keeps the text).
func TestQueueFilterEnterCommits(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewQueue
	m.queue.Entries = []queue.Entry{
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
	if m.queue.Filter.Active {
		t.Error("filter input mode should be inactive after Enter")
	}
	if m.queue.Filter.Text != "x" {
		t.Errorf("filter text should be preserved after Enter, got %q", m.queue.Filter.Text)
	}
}

// TestQueueEscClearsCommittedFilter verifies that pressing Esc after committing
// a filter (Enter) clears the filter text and resets the cursor.
func TestQueueEscClearsCommittedFilter(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewQueue
	m.queue.Entries = []queue.Entry{
		{Path: "/a.flac", Title: "Alpha", Artist: "ArtA"},
		{Path: "/b.flac", Title: "Beta", Artist: "ArtB"},
	}

	// Activate, type, and commit the filter.
	next, _ := m.Update(FilterBarMsg{})
	m = next.(Model)
	next, _ = m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	m = next.(Model)
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	if m.queue.Filter.Text != "a" {
		t.Fatalf("committed filter text should be 'a', got %q", m.queue.Filter.Text)
	}

	// Press Esc to clear the committed filter.
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = next.(Model)
	if m.queue.Filter.Active {
		t.Error("filter should be inactive after Esc")
	}
	if m.queue.Filter.Text != "" {
		t.Errorf("filter text should be empty after Esc, got %q", m.queue.Filter.Text)
	}
	if m.queue.Cursor != 0 {
		t.Errorf("cursor should be 0 after Esc, got %d", m.queue.Cursor)
	}
}

// TestQueueEscNoOpWithoutFilter verifies that pressing Esc in the queue view
// without a committed filter does not crash or change state unexpectedly.
func TestQueueEscNoOpWithoutFilter(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewQueue
	m.queue.Entries = []queue.Entry{
		{Path: "/a.flac", Title: "Alpha", Artist: "ArtA"},
	}
	m.queue.Cursor = 0

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	nm := next.(Model)
	if nm.queue.Cursor != 0 {
		t.Errorf("cursor should remain 0, got %d", nm.queue.Cursor)
	}
	if nm.queue.Filter.Text != "" {
		t.Errorf("filter text should be empty, got %q", nm.queue.Filter.Text)
	}
}
