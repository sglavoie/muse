package tui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
)

// TestLibraryCursorDownJKey verifies that pressing "j" while the library view
// is active advances the cursor by one.
func TestLibraryCursorDownJKey(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Artist: "A", Title: "One"})
	m.library.index.Insert(library.Track{Artist: "B", Title: "Two"})

	next, _ := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	nm := next.(Model)
	if nm.library.cursor != 1 {
		t.Errorf("j key: expected cursor 1, got %d", nm.library.cursor)
	}
}

// TestLibraryCursorUpKKey verifies that pressing "k" while the library view
// is active moves the cursor up by one.
func TestLibraryCursorUpKKey(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Artist: "A", Title: "One"})
	m.library.index.Insert(library.Track{Artist: "B", Title: "Two"})
	m.library.cursor = 1

	next, _ := m.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	nm := next.(Model)
	if nm.library.cursor != 0 {
		t.Errorf("k key: expected cursor 0, got %d", nm.library.cursor)
	}
}

// TestLibraryCursorDownArrow verifies that the down arrow key advances the
// cursor in the library view, behaving identically to "j".
func TestLibraryCursorDownArrow(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Artist: "A", Title: "One"})
	m.library.index.Insert(library.Track{Artist: "B", Title: "Two"})

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown, Text: "down"})
	nm := next.(Model)
	if nm.library.cursor != 1 {
		t.Errorf("down arrow: expected cursor 1, got %d", nm.library.cursor)
	}
}

// TestLibraryCursorUpArrow verifies that the up arrow key moves the cursor up
// in the library view, behaving identically to "k".
func TestLibraryCursorUpArrow(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Artist: "A", Title: "One"})
	m.library.index.Insert(library.Track{Artist: "B", Title: "Two"})
	m.library.cursor = 1

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyUp, Text: "up"})
	nm := next.(Model)
	if nm.library.cursor != 0 {
		t.Errorf("up arrow: expected cursor 0, got %d", nm.library.cursor)
	}
}

// TestLibraryCursorClampAtBottom verifies that the cursor stops at the last
// track without wrapping when the library view is active.
func TestLibraryCursorClampAtBottom(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Artist: "A", Title: "One"})
	m.library.index.Insert(library.Track{Artist: "B", Title: "Two"})
	m.library.cursor = 1

	next, _ := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	nm := next.(Model)
	if nm.library.cursor != 1 {
		t.Errorf("j at last: expected cursor 1 (clamped), got %d", nm.library.cursor)
	}
}

// TestLibraryCursorClampAtTop verifies that the cursor stops at the first
// track without wrapping when the library view is active.
func TestLibraryCursorClampAtTop(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Artist: "A", Title: "One"})
	m.library.index.Insert(library.Track{Artist: "B", Title: "Two"})

	next, _ := m.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	nm := next.(Model)
	if nm.library.cursor != 0 {
		t.Errorf("k at first: expected cursor 0 (clamped), got %d", nm.library.cursor)
	}
}

// TestLibraryCursorNotMovedWhenQueueActive verifies that j/k in the queue view
// do not affect the library cursor.
func TestLibraryCursorNotMovedWhenQueueActive(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Artist: "A", Title: "One"})
	m.library.index.Insert(library.Track{Artist: "B", Title: "Two"})
	m.activeView = viewQueue

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown, Text: "down"})
	nm := next.(Model)
	if nm.library.cursor != 0 {
		t.Errorf("down in queue view must not move library cursor, got %d", nm.library.cursor)
	}
}

// TestRootLibraryXTogglesSelectedHash verifies that pressing 'x' while the
// library view is active toggles the focused track's hash into selectedHashes.
func TestRootLibraryXTogglesSelectedHash(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Artist: "A", Title: "One", Hash: "hash-one"})
	m.library.index.Insert(library.Track{Artist: "B", Title: "Two", Hash: "hash-two"})
	m.library.cursor = 0

	next, _ := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	nm := next.(Model)

	if _, ok := nm.library.selectedHashes["hash-one"]; !ok {
		t.Errorf("'x' on focused track: expected hash-one in selectedHashes, got %v", nm.library.selectedHashes)
	}
	if _, ok := nm.library.selectedHashes["hash-two"]; ok {
		t.Errorf("'x' on focused track: hash-two must not be in selectedHashes")
	}

	next2, _ := nm.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	nm2 := next2.(Model)
	if _, ok := nm2.library.selectedHashes["hash-one"]; ok {
		t.Errorf("second 'x': expected hash-one to be deselected, but it is still in selectedHashes")
	}
}
