package tui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
)

// TestUDispatchesUndoMessage verifies that pressing 'u' returns a cmd that
// emits UndoMsg.
func TestUDispatchesUndoMessage(t *testing.T) {
	m := New(context.Background())

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'u', Text: "u"})
	if cmd == nil {
		t.Fatal("u key: expected non-nil cmd for undo dispatch")
	}
	msg := cmd()
	if _, ok := msg.(UndoMsg); !ok {
		t.Fatalf("u key: expected UndoMsg, got %T", msg)
	}
}

// TestSlashRoutesToActiveView verifies that pressing '/' returns a cmd that
// emits FilterBarMsg for routing to the active view.
func TestSlashRoutesToActiveView(t *testing.T) {
	m := New(context.Background())

	_, cmd := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	if cmd == nil {
		t.Fatal("/ key: expected non-nil cmd for filter bar activation")
	}
	msg := cmd()
	if _, ok := msg.(FilterBarMsg); !ok {
		t.Fatalf("/ key: expected FilterBarMsg, got %T", msg)
	}
}

func TestShiftArrowKeysControlSortDirection(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewLibrary

	next, _ := m.Update(library.ScanBatchMsg{Tracks: []library.Track{
		{Hash: "h1", Path: "/music/beta.flac", Artist: "Bravo", Title: "Beta"},
		{Hash: "h2", Path: "/music/alpha.flac", Artist: "Alpha", Title: "Alpha"},
	}})
	m = next.(Model)
	m.library.height = 12

	next, _ = m.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	nm := next.(Model)
	if nm.library.sortState.Field == SortDefault {
		t.Fatal("ctrl+s should activate a non-default library sort field")
	}

	next, _ = nm.Update(tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModShift})
	nm = next.(Model)
	if nm.library.sortState.Dir != SortDesc {
		t.Fatalf("shift+down: expected SortDesc, got %v", nm.library.sortState.Dir)
	}

	next, _ = nm.Update(tea.KeyPressMsg{Code: tea.KeyUp, Mod: tea.ModShift})
	nm = next.(Model)
	if nm.library.sortState.Dir != SortAsc {
		t.Fatalf("shift+up: expected SortAsc, got %v", nm.library.sortState.Dir)
	}
}

func TestBracketKeysSwitchViewsAndAltAliasesAreUnbound(t *testing.T) {
	reg := newRegistry()

	if got := reg.resolve(contextGlobal, "["); got != actionPrevView {
		t.Fatalf("resolve(contextGlobal, \"[\") = %v, want actionPrevView", got)
	}
	if got := reg.resolve(contextGlobal, "]"); got != actionNextView {
		t.Fatalf("resolve(contextGlobal, \"]\") = %v, want actionNextView", got)
	}

	for _, key := range []string{"alt+left", "alt+right", "alt+b", "alt+f"} {
		if got := reg.resolve(contextGlobal, key); got != actionNone {
			t.Fatalf("resolve(contextGlobal, %q) = %v, want actionNone", key, got)
		}
	}
}

func TestQueueHalfPageNavigationUsesCtrlDAndCtrlU(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewQueue
	m.queue.height = 14
	m.queue.entries = make([]QueueEntry, 20)

	next, _ := m.Update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	nm := next.(Model)
	// visibleRows = height(14) - reserve(3) = 11; HalfPageDown = 11/2 = 5
	if nm.queue.cursor != 5 {
		t.Fatalf("ctrl+d: expected cursor 5, got %d", nm.queue.cursor)
	}

	next, _ = nm.Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	nm = next.(Model)
	if nm.queue.cursor != 0 {
		t.Fatalf("ctrl+u: expected cursor 0, got %d", nm.queue.cursor)
	}
}

// TestHKeyIsUnbound verifies that 'h' is intentionally unbound: pressing it
// produces no command and leaves the model state unchanged.
func TestHKeyIsUnbound(t *testing.T) {
	m := New(context.Background())

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})

	if cmd != nil {
		t.Fatal("'h' key must be unbound: expected nil cmd")
	}
}

// TestLKeyIsUnbound verifies that 'l' is intentionally unbound: pressing it
// produces no command and leaves the model state unchanged.
func TestLKeyIsUnbound(t *testing.T) {
	m := New(context.Background())

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})

	if cmd != nil {
		t.Fatal("'l' key must be unbound: expected nil cmd")
	}
}
