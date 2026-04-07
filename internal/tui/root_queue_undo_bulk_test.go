package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// TestUndoBulkDequeueRestoresEntries verifies that undoing a bulk dequeue
// restores all removed entries at their original indices and calls UndoDequeue.
func TestUndoBulkDequeueRestoresEntries(t *testing.T) {
	d := &stubQueueDequeuer{}
	ps := &stubPlayerQueueSyncer{}
	m := New(context.Background(), WithPlayer(ps), WithQueueDequeuer(d))
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{
		{ID: 10, Path: "/a.flac", Display: "A"},
		{ID: 20, Path: "/b.flac", Display: "B", Selected: true},
		{ID: 30, Path: "/c.flac", Display: "C"},
		{ID: 40, Path: "/d.flac", Display: "D", Selected: true},
		{ID: 50, Path: "/e.flac", Display: "E"},
	}

	next, _ := m.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	nm := next.(Model)
	if len(nm.queue.entries) != 3 {
		t.Fatalf("after bulk dequeue: expected 3 entries, got %d", len(nm.queue.entries))
	}

	next, _ = nm.Update(UndoMsg{})
	nm = next.(Model)

	if len(nm.queue.entries) != 5 {
		t.Fatalf("after undo: expected 5 entries, got %d", len(nm.queue.entries))
	}
	wantIDs := []int64{10, 20, 30, 40, 50}
	for i, want := range wantIDs {
		if nm.queue.entries[i].ID != want {
			t.Errorf("entry[%d].ID = %d, want %d", i, nm.queue.entries[i].ID, want)
		}
	}
	if len(d.undoDequeueCallIDs) != 2 {
		t.Fatalf("UndoDequeue calls = %d, want 2", len(d.undoDequeueCallIDs))
	}
	if d.undoDequeueCallIDs[0] != 20 || d.undoDequeueCallIDs[1] != 40 {
		t.Fatalf("UndoDequeue IDs = %v, want [20 40]", d.undoDequeueCallIDs)
	}
	if len(ps.setQueueCalls) == 0 {
		t.Fatal("expected SetQueue to be called after undo")
	}
	if nm.undo.slot.action != nil {
		t.Fatal("undo slot must be cleared after successful bulk dequeue undo")
	}
}

// TestUndoBulkDequeueExpiredIsNoOp verifies that undoing a bulk dequeue after
// the undo window has expired shows "Nothing to undo" and does not restore entries.
func TestUndoBulkDequeueExpiredIsNoOp(t *testing.T) {
	d := &stubQueueDequeuer{}
	m := New(context.Background(), WithQueueDequeuer(d))
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{
		{ID: 10, Path: "/a.flac", Display: "A"},
		{ID: 20, Path: "/b.flac", Display: "B", Selected: true},
	}

	next, _ := m.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	nm := next.(Model)
	nm.undo.slot.expiresAt = time.Now().Add(-1 * time.Second)

	next, _ = nm.Update(UndoMsg{})
	nm = next.(Model)

	if len(nm.queue.entries) != 1 {
		t.Fatalf("after expired undo: expected 1 entry, got %d", len(nm.queue.entries))
	}
	if len(d.undoDequeueCallIDs) != 0 {
		t.Fatalf("UndoDequeue must not be called after expiry, got %v", d.undoDequeueCallIDs)
	}
	if !strings.Contains(nm.statusBar.notification.msg, "Nothing to undo") {
		t.Fatalf("expected 'Nothing to undo' notification, got %q", nm.statusBar.notification.msg)
	}
}
