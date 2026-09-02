package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/sglavoie/muse/internal/tui/queue"
)

// TestUndoDequeueShowsNotification verifies that undoing a dequeue sets a
// confirmation notification in the status bar.
func TestUndoDequeueShowsNotification(t *testing.T) {
	d := &stubQueueDequeuer{}
	m := New(context.Background(), WithQueueDequeuer(d))
	m.queue.Entries = []queue.Entry{
		{ID: 10, Path: "/a.flac", Display: "A"},
		{ID: 20, Path: "/b.flac", Display: "B"},
	}
	next, _ := m.Update(queue.DequeueTrackMsg{Index: 0})
	nm := next.(Model)
	next, _ = nm.Update(UndoMsg{})
	nm = next.(Model)

	if nm.statusBar.TopNotification().Msg == "" {
		t.Fatal("undo dequeue must set a status bar notification")
	}
	if !strings.Contains(nm.statusBar.TopNotification().Msg, "Undone") {
		t.Errorf("expected undo notification containing 'Undone', got %q", nm.statusBar.TopNotification().Msg)
	}
}

// TestUndoEnqueueShowsNotification verifies that undoing an enqueue sets a
// confirmation notification in the status bar.
func TestUndoEnqueueShowsNotification(t *testing.T) {
	d := &stubQueueDequeuer{}
	m := New(context.Background(), WithQueueDequeuer(d))
	m.queue.Entries = []queue.Entry{{ID: 10, Path: "/a.flac", Hash: "aaa", Display: "Artist – Song"}}
	m.setUndoSlot(undoEnqueueAction{id: 10, entry: m.queue.Entries[0], idx: 0}, nil)

	nm := m.handleUndo()

	if nm.statusBar.TopNotification().Msg == "" {
		t.Fatal("undo enqueue must set a status bar notification")
	}
	if !strings.Contains(nm.statusBar.TopNotification().Msg, "Undone") {
		t.Errorf("expected undo notification containing 'Undone', got %q", nm.statusBar.TopNotification().Msg)
	}
}

// TestUndoClearQueueShowsNotification verifies that undoing a queue clear sets
// a confirmation notification in the status bar.
func TestUndoClearQueueShowsNotification(t *testing.T) {
	d := &stubQueueDequeuer{}
	m := New(context.Background(), WithQueueDequeuer(d))
	m.queue.Entries = []queue.Entry{
		{ID: 10, Path: "/a.flac", Display: "A"},
		{ID: 20, Path: "/b.flac", Display: "B"},
	}

	savedEntries := make([]queue.Entry, len(m.queue.Entries))
	copy(savedEntries, m.queue.Entries)

	m.setUndoSlot(undoClearAction{ids: []int64{10, 20}, entries: savedEntries}, nil)
	m.queue.Entries = nil

	nm := m.handleUndo()

	if nm.statusBar.TopNotification().Msg == "" {
		t.Fatal("undo clear must set a status bar notification")
	}
	if !strings.Contains(nm.statusBar.TopNotification().Msg, "Undone") {
		t.Errorf("expected undo notification containing 'Undone', got %q", nm.statusBar.TopNotification().Msg)
	}
	if !strings.Contains(nm.statusBar.TopNotification().Msg, "2") {
		t.Errorf("expected undo notification to mention track count, got %q", nm.statusBar.TopNotification().Msg)
	}
}

// TestUndoBulkDequeueShowsNotification verifies that undoing a bulk dequeue
// sets a confirmation notification in the status bar.
func TestUndoBulkDequeueShowsNotification(t *testing.T) {
	d := &stubQueueDequeuer{}
	m := New(context.Background(), WithQueueDequeuer(d))

	removed := []queue.Entry{
		{ID: 10, Path: "/a.flac", Display: "A"},
		{ID: 20, Path: "/b.flac", Display: "B"},
	}
	m.setUndoSlot(undoBulkDequeueAction{entries: removed, indices: []int{0, 1}, ids: []int64{10, 20}}, nil)

	nm := m.handleUndo()

	if nm.statusBar.TopNotification().Msg == "" {
		t.Fatal("undo bulk dequeue must set a status bar notification")
	}
	if !strings.Contains(nm.statusBar.TopNotification().Msg, "Undone") {
		t.Errorf("expected undo notification containing 'Undone', got %q", nm.statusBar.TopNotification().Msg)
	}
}

// TestUndoBatchEnqueueShowsNotification verifies that undoing a batch enqueue
// sets a confirmation notification in the status bar.
func TestUndoBatchEnqueueShowsNotification(t *testing.T) {
	be := &stubBatchEnqueuer{}
	d := &stubQueueDequeuer{}
	m := New(context.Background(), WithBatchEnqueuer(be), WithQueueDequeuer(d))
	m.queue.Entries = []queue.Entry{
		{ID: 100, Path: "/x.flac", Hash: "xxx", Display: "X"},
		{ID: 101, Path: "/y.flac", Hash: "yyy", Display: "Y"},
	}

	m.setUndoSlot(undoBatchEnqueueAction{
		ids:          []int64{100, 101},
		count:        2,
		startIdx:     0,
		playlistName: "Test",
	}, nil)

	nm := m.handleUndo()

	if nm.statusBar.TopNotification().Msg == "" {
		t.Fatal("undo batch enqueue must set a status bar notification")
	}
	if !strings.Contains(nm.statusBar.TopNotification().Msg, "Undone") {
		t.Errorf("expected undo notification containing 'Undone', got %q", nm.statusBar.TopNotification().Msg)
	}
}
