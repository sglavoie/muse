package tui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/browse"
	"github.com/sglavoie/muse/internal/tui/queue"
)

func TestClearFilterPreservesFocusedTrack(t *testing.T) {
	m := New(context.Background())
	m = m.handleWindowSizeMsg(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.library.Index.BulkLoad([]library.Track{
		{Hash: "a", Artist: "Alice", Title: "One"},
		{Hash: "b", Artist: "Bob", Title: "Two"},
		{Hash: "c", Artist: "Charlie", Title: "Three"},
	})
	m = typeText(pressSlash(m), "Charlie")
	m = pressEsc(pressEnter(m))
	if track, ok := m.library.Panel().FocusedTrack(); !ok || track.Hash != "c" {
		t.Fatalf("clearing filter lost focused track: %+v", track)
	}
}

func TestNoMatchesRestoresPositionAfterFilterReopen(t *testing.T) {
	m := newRichFixture()
	m = m.handleWindowSizeMsg(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.library.Cursor = 3
	before, _ := m.library.Panel().FocusedTrack()
	m = pressEnter(typeText(pressSlash(m), "zzzzzz"))
	m = pressSlash(m) // re-edit the committed search without losing its origin
	m = pressEsc(m)
	after, _ := m.library.Panel().FocusedTrack()
	if after.Hash != before.Hash {
		t.Fatalf("got %q, want original %q", after.Hash, before.Hash)
	}
}

func TestClearQueueFilterPreservesDuplicateOccurrence(t *testing.T) {
	m := New(context.Background())
	m.ActiveView = viewQueue
	m.queue.Entries = []queue.Entry{
		{ID: 10, Hash: "other", Title: "Other"},
		{ID: 20, Hash: "same", Title: "Same"},
		{ID: 30, Hash: "same", Title: "Same"},
	}
	m = pressEnter(typeText(pressSlash(m), "Same"))
	m.queue.Cursor = 1
	m = pressEsc(m)
	if m.queue.Cursor != 2 {
		t.Fatalf("cursor = %d; want second occurrence at 2", m.queue.Cursor)
	}
}

func TestClearBrowseFilterPreservesCollection(t *testing.T) {
	m := newRichFixture()
	m.ActiveView = viewBrowse
	m.browse.Mode = browse.BrowseModeArtists
	m.browse.Artists.Artists = []string{"Alice", "Bob", "Charlie"}
	m = pressEsc(pressEnter(typeText(pressSlash(m), "Charlie")))
	if m.browse.Artists.Cursor != 2 {
		t.Fatalf("cursor = %d, want 2", m.browse.Artists.Cursor)
	}
}
