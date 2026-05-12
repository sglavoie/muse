package tui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// TestRootQueueLocalDDispatchesDequeueMsg verifies the end-to-end path: pressing
// 'd' while the queue view is active returns a Cmd that emits DequeueTrackMsg.
func TestRootQueueLocalDDispatchesDequeueMsg(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{
		{Path: "/music/a.flac", Display: "A"},
		{Path: "/music/b.flac", Display: "B"},
	}
	m.queue.cursor = 1

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	if cmd == nil {
		t.Fatal("'d' in queue view: expected non-nil cmd")
	}
	msg, ok := cmd().(DequeueTrackMsg)
	if !ok {
		t.Fatalf("'d' in queue view: expected DequeueTrackMsg, got %T", cmd())
	}
	if msg.Index != 1 {
		t.Fatalf("'d' in queue view: expected index 1, got %d", msg.Index)
	}
}

// TestRootQueueLocalXDispatchesBulkSelectionToggleMsg verifies the end-to-end
// path: pressing 'x' while the queue view is active dispatches BulkSelectionToggleMsg.
func TestRootQueueLocalXDispatchesBulkSelectionToggleMsg(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{
		{Path: "/music/a.flac", Display: "A"},
	}
	m.queue.cursor = 0

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	if cmd == nil {
		t.Fatal("'x' in queue view: expected non-nil cmd")
	}
	if _, ok := cmd().(BulkSelectionToggleMsg); !ok {
		t.Fatalf("'x' in queue view: expected BulkSelectionToggleMsg, got %T", cmd())
	}
}

// TestRootQueueLocalCDispatchesClearQueueMsg verifies the end-to-end path:
// pressing 'C' (Shift+c) while the queue view is active immediately dispatches
// ClearQueueMsg — no two-press confirmation required since the undo window handles it.
func TestRootQueueLocalCDispatchesClearQueueMsg(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{
		{Path: "/music/a.flac", Display: "A"},
	}

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'C', Text: "C"})
	if cmd == nil {
		t.Fatal("'C' in queue view: expected non-nil cmd")
	}
	if _, ok := cmd().(ClearQueueMsg); !ok {
		t.Fatalf("'C' in queue view: expected ClearQueueMsg, got %T", cmd())
	}
}

// TestRootQueueLocalKDispatchesMoveTrackUpMsg verifies the end-to-end path:
// pressing 'K' (Shift+k) while the queue view is active dispatches MoveTrackUpMsg.
func TestRootQueueLocalKDispatchesMoveTrackUpMsg(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{
		{Path: "/music/a.flac", Display: "A"},
		{Path: "/music/b.flac", Display: "B"},
	}
	m.queue.cursor = 1

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'K', Text: "K"})
	if cmd == nil {
		t.Fatal("'K' in queue view: expected non-nil cmd")
	}
	msg, ok := cmd().(MoveTrackUpMsg)
	if !ok {
		t.Fatalf("'K' in queue view: expected MoveTrackUpMsg, got %T", cmd())
	}
	if msg.Index != 1 {
		t.Fatalf("'K' in queue view: expected index 1, got %d", msg.Index)
	}
}

// TestRootQueueLocalJDispatchesMoveTrackDownMsg verifies the end-to-end path:
// pressing 'J' (Shift+j) while the queue view is active dispatches MoveTrackDownMsg.
func TestRootQueueLocalJDispatchesMoveTrackDownMsg(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{
		{Path: "/music/a.flac", Display: "A"},
		{Path: "/music/b.flac", Display: "B"},
	}
	m.queue.cursor = 0

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'J', Text: "J"})
	if cmd == nil {
		t.Fatal("'J' in queue view: expected non-nil cmd")
	}
	msg, ok := cmd().(MoveTrackDownMsg)
	if !ok {
		t.Fatalf("'J' in queue view: expected MoveTrackDownMsg, got %T", cmd())
	}
	if msg.Index != 0 {
		t.Fatalf("'J' in queue view: expected index 0, got %d", msg.Index)
	}
}
