package tui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
)

// TestLibraryEscClearsCommittedFilter verifies that pressing Esc while the
// library has a committed (non-active) filter clears the filter text and
// restores the full library listing.
func TestLibraryEscClearsCommittedFilter(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Artist: "Alice", Title: "Alpha", Hash: "h1"})
	m.library.index.Insert(library.Track{Artist: "Bob", Title: "Beta", Hash: "h2"})

	m = pressSlash(m)
	m = typeText(m, "ali")
	m = pressEnter(m)

	if m.library.filter.Text != "ali" {
		t.Fatalf("expected committed filterText \"ali\", got %q", m.library.filter.Text)
	}
	if m.library.filter.Active {
		t.Fatal("expected filterActive=false after commit")
	}

	m = pressEsc(m)
	if m.library.filter.Text != "" {
		t.Errorf("expected filterText cleared after Esc, got %q", m.library.filter.Text)
	}
	if m.library.filter.Active {
		t.Error("expected filterActive=false after Esc")
	}
}

// TestLibraryEnterClearsCommittedFilter verifies that pressing Enter on a
// focused track while a committed filter is active enqueues the track and
// clears the filter.
func TestLibraryEnterClearsCommittedFilter(t *testing.T) {
	qa := &stubQueueAppender{}
	m := New(context.Background(), WithQueueWriter(qa))
	m.library.index.Insert(library.Track{Artist: "Alice", Title: "Alpha", Hash: "h1"})
	m.library.index.Insert(library.Track{Artist: "Bob", Title: "Beta", Hash: "h2"})

	m = pressSlash(m)
	m = typeText(m, "ali")
	m = pressEnter(m)

	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)

	if m.library.filter.Text != "" {
		t.Errorf("expected filterText cleared after enqueue Enter, got %q", m.library.filter.Text)
	}
	if m.library.filter.Active {
		t.Error("expected filterActive=false after enqueue Enter")
	}
	if cmd == nil {
		t.Fatal("expected non-nil enqueue cmd")
	}
	cmd()
	if len(qa.tracks) != 1 {
		t.Errorf("expected 1 enqueued track, got %d", len(qa.tracks))
	}
}

// TestHistoryEscClearsCommittedFilter verifies that pressing Esc while the
// History view has a committed filter clears the filter text.
func TestHistoryEscClearsCommittedFilter(t *testing.T) {
	m := New(context.Background())
	m.library.mode = LibraryModeHistory
	m.activeView = viewLibrary
	m.library.sourceTracks = []library.Track{
		{Artist: "Alice", Title: "Alpha", Hash: "h1"},
		{Artist: "Bob", Title: "Beta", Hash: "h2"},
	}

	m = pressSlash(m)
	m = typeText(m, "ali")
	m = pressEnter(m)

	if m.library.filter.Text != "ali" {
		t.Fatalf("expected committed filterText \"ali\", got %q", m.library.filter.Text)
	}
	if m.library.filter.Active {
		t.Fatal("expected filterActive=false after commit")
	}

	m = pressEsc(m)
	if m.library.filter.Text != "" {
		t.Errorf("expected filterText cleared after Esc, got %q", m.library.filter.Text)
	}
	if m.library.filter.Active {
		t.Error("expected filterActive=false after Esc")
	}
}

// TestFilterBarMsgIgnoredOutsideLibraryView verifies that pressing '/' while on
// the playlists view does not activate the library filter bar. Before the fix,
// FilterBarMsg always toggled the library filter regardless of the active view,
// silently leaving it in an inconsistent state.
func TestFilterBarMsgIgnoredOutsideLibraryView(t *testing.T) {
	m := New(context.Background())

	next, _ := m.Update(tea.KeyPressMsg{Code: '3', Text: "3"})
	nm := next.(Model)
	if nm.activeView != viewQueue {
		t.Fatalf("expected viewQueue, got %d", nm.activeView)
	}

	next, _ = nm.Update(FilterBarMsg{})
	nm = next.(Model)
	if nm.library.filter.Active {
		t.Error("library filterActive should remain false when FilterBarMsg is sent on the queue view")
	}
	if !nm.queue.filter.Active {
		t.Error("queue filter should be active after FilterBarMsg on queue view")
	}

	next, _ = m.Update(tea.KeyPressMsg{Code: '2', Text: "2"})
	nm = next.(Model)
	next, _ = nm.Update(FilterBarMsg{})
	nm = next.(Model)
	if nm.library.filter.Active {
		t.Error("library filterActive should remain false when FilterBarMsg is sent on playlists view")
	}
	if nm.statusBar.TopNotification().Msg == "" {
		t.Error("FilterBarMsg on playlists view should set a transient notification")
	}
}
