package tui

import (
	"context"
	"strings"
	"testing"
)

// TestPasteSingleTrack verifies that ctrl+p inserts a copy below the cursor.
func TestPasteSingleTrack(t *testing.T) {
	qa := &stubQueueAppender{}
	m := New(context.Background(), WithQueueWriter(qa))
	m.activeView = viewQueue
	m.queue.entries = sampleQueueEntries()
	m.queue.cursor = 1
	m.yankBuffer = []QueueEntry{m.queue.entries[0]}

	rm, cmd, handled := m.handleQueueKeyAction(actionPaste)
	m = rm.(Model)

	if !handled {
		t.Fatal("actionPaste should be handled")
	}
	if cmd == nil {
		t.Fatal("expected non-nil cmd for paste")
	}

	// Execute the command to get the QueuePastedMsg.
	msg := cmd().(QueuePastedMsg)
	if msg.Err != nil {
		t.Fatalf("paste cmd returned error: %v", msg.Err)
	}
	if len(msg.Entries) != 1 {
		t.Fatalf("expected 1 pasted entry, got %d", len(msg.Entries))
	}
	if msg.InsertPos != 2 {
		t.Errorf("expected InsertPos 2, got %d", msg.InsertPos)
	}
	// The pasted entry should have a non-zero DB ID assigned by the writer.
	if msg.Entries[0].ID == 0 {
		t.Error("pasted entry should have a non-zero DB ID from the writer")
	}

	// Feed the message through the handler.
	m = m.handleQueuePastedMsg(msg)
	if len(m.queue.entries) != 5 {
		t.Fatalf("expected 5 entries after paste, got %d", len(m.queue.entries))
	}
	// Pasted entry should be at position 2 (after cursor=1).
	if m.queue.entries[2].Hash != "h1" {
		t.Errorf("expected pasted entry at index 2 with hash h1, got %q", m.queue.entries[2].Hash)
	}
	// Cursor should move to the last pasted entry.
	if m.queue.cursor != 2 {
		t.Errorf("expected cursor at 2, got %d", m.queue.cursor)
	}
}

// TestPasteMultipleTracks verifies that pasting multiple tracks inserts them
// contiguously below the cursor.
func TestPasteMultipleTracks(t *testing.T) {
	qa := &stubQueueAppender{}
	m := New(context.Background(), WithQueueWriter(qa))
	m.activeView = viewQueue
	m.queue.entries = sampleQueueEntries()
	m.queue.cursor = 0
	m.yankBuffer = []QueueEntry{m.queue.entries[2], m.queue.entries[3]}

	_, cmd, _ := m.handleQueueKeyAction(actionPaste)
	msg := cmd().(QueuePastedMsg)
	m = m.handleQueuePastedMsg(msg)

	if len(m.queue.entries) != 6 {
		t.Fatalf("expected 6 entries after paste, got %d", len(m.queue.entries))
	}
	// Entries at positions 1 and 2 should be the pasted tracks.
	if m.queue.entries[1].Hash != "h3" || m.queue.entries[2].Hash != "h4" {
		t.Errorf("pasted entries at wrong positions: [1]=%q [2]=%q",
			m.queue.entries[1].Hash, m.queue.entries[2].Hash)
	}
	if m.queue.cursor != 2 {
		t.Errorf("cursor should be at last pasted entry (2), got %d", m.queue.cursor)
	}
}

// TestPasteTwice verifies that the yank buffer persists across multiple pastes.
func TestPasteTwice(t *testing.T) {
	qa := &stubQueueAppender{}
	m := New(context.Background(), WithQueueWriter(qa))
	m.activeView = viewQueue
	m.queue.entries = sampleQueueEntries()
	m.queue.cursor = 0
	m.yankBuffer = []QueueEntry{m.queue.entries[0]}

	// First paste.
	_, cmd, _ := m.handleQueueKeyAction(actionPaste)
	msg := cmd().(QueuePastedMsg)
	m = m.handleQueuePastedMsg(msg)
	if len(m.queue.entries) != 5 {
		t.Fatalf("expected 5 after first paste, got %d", len(m.queue.entries))
	}

	// Second paste (cursor moved to 1 after first paste).
	_, cmd, _ = m.handleQueueKeyAction(actionPaste)
	msg = cmd().(QueuePastedMsg)
	m = m.handleQueuePastedMsg(msg)
	if len(m.queue.entries) != 6 {
		t.Fatalf("expected 6 after second paste, got %d", len(m.queue.entries))
	}
}

// TestPasteEmptyBuffer verifies notification when nothing has been yanked.
func TestPasteEmptyBuffer(t *testing.T) {
	qa := &stubQueueAppender{}
	m := New(context.Background(), WithQueueWriter(qa))
	m.activeView = viewQueue
	m.queue.entries = sampleQueueEntries()

	rm, cmd, handled := m.handleQueueKeyAction(actionPaste)
	m = rm.(Model)

	if !handled {
		t.Fatal("actionPaste should be handled")
	}
	if cmd != nil {
		t.Fatal("expected nil cmd when yank buffer is empty")
	}
	if !strings.Contains(m.statusBar.notification.msg, "Nothing to paste") {
		t.Errorf("expected 'Nothing to paste' notification, got %q", m.statusBar.notification.msg)
	}
}

// TestPasteWhileSorted verifies paste is blocked when queue is sorted.
func TestPasteWhileSorted(t *testing.T) {
	qa := &stubQueueAppender{}
	m := New(context.Background(), WithQueueWriter(qa))
	m.activeView = viewQueue
	m.queue.entries = sampleQueueEntries()
	m.yankBuffer = []QueueEntry{m.queue.entries[0]}
	m.queue.displayOrder = []int{3, 2, 1, 0} // non-nil = sorted

	rm, cmd, handled := m.handleQueueKeyAction(actionPaste)
	m = rm.(Model)

	if !handled {
		t.Fatal("actionPaste should be handled")
	}
	if cmd != nil {
		t.Fatal("expected nil cmd when sorted")
	}
	if !strings.Contains(m.statusBar.notification.msg, "Cannot paste") {
		t.Errorf("expected 'Cannot paste' notification, got %q", m.statusBar.notification.msg)
	}
}

// TestPasteWhileFiltered verifies paste is blocked when filter is active.
func TestPasteWhileFiltered(t *testing.T) {
	qa := &stubQueueAppender{}
	m := New(context.Background(), WithQueueWriter(qa))
	m.activeView = viewQueue
	m.queue.entries = sampleQueueEntries()
	m.yankBuffer = []QueueEntry{m.queue.entries[0]}
	m.queue.filter.Text = "artist"

	rm, cmd, handled := m.handleQueueKeyAction(actionPaste)
	m = rm.(Model)

	if !handled {
		t.Fatal("actionPaste should be handled")
	}
	if cmd != nil {
		t.Fatal("expected nil cmd when filtered")
	}
	if !strings.Contains(m.statusBar.notification.msg, "Cannot paste") {
		t.Errorf("expected 'Cannot paste' notification, got %q", m.statusBar.notification.msg)
	}
}

// TestPasteAdjustsCurrentIndex verifies that pasting before the currently
// playing track shifts currentIndex forward.
func TestPasteAdjustsCurrentIndex(t *testing.T) {
	qa := &stubQueueAppender{}
	m := New(context.Background(), WithQueueWriter(qa))
	m.activeView = viewQueue
	m.queue.entries = sampleQueueEntries()
	m.playback.currentIndex = 2
	m.queue.cursor = 0
	m.yankBuffer = []QueueEntry{m.queue.entries[0]}

	_, cmd, _ := m.handleQueueKeyAction(actionPaste)
	msg := cmd().(QueuePastedMsg)
	m = m.handleQueuePastedMsg(msg)

	// Insert at position 1 (after cursor 0), which is before currentIndex=2.
	// currentIndex should shift by 1.
	if m.playback.currentIndex != 3 {
		t.Errorf("expected currentIndex 3, got %d", m.playback.currentIndex)
	}
}

// TestPasteAtEndOfQueue verifies paste when cursor is on the last entry.
func TestPasteAtEndOfQueue(t *testing.T) {
	qa := &stubQueueAppender{}
	m := New(context.Background(), WithQueueWriter(qa))
	m.activeView = viewQueue
	m.queue.entries = sampleQueueEntries()
	m.queue.cursor = 3 // last entry
	m.yankBuffer = []QueueEntry{m.queue.entries[0]}

	_, cmd, _ := m.handleQueueKeyAction(actionPaste)
	msg := cmd().(QueuePastedMsg)
	m = m.handleQueuePastedMsg(msg)

	if len(m.queue.entries) != 5 {
		t.Fatalf("expected 5 entries, got %d", len(m.queue.entries))
	}
	// Pasted entry should be at the end.
	if m.queue.entries[4].Hash != "h1" {
		t.Errorf("expected pasted entry at end with hash h1, got %q", m.queue.entries[4].Hash)
	}
	if m.queue.cursor != 4 {
		t.Errorf("expected cursor at 4, got %d", m.queue.cursor)
	}
}

// TestPasteSetsUndoSlot verifies that paste sets up an undo slot.
func TestPasteSetsUndoSlot(t *testing.T) {
	qa := &stubQueueAppender{}
	m := New(context.Background(), WithQueueWriter(qa))
	m.activeView = viewQueue
	m.queue.entries = sampleQueueEntries()
	m.queue.cursor = 1
	m.yankBuffer = []QueueEntry{m.queue.entries[0]}

	_, cmd, _ := m.handleQueueKeyAction(actionPaste)
	msg := cmd().(QueuePastedMsg)
	m = m.handleQueuePastedMsg(msg)

	if m.undo.slot.action == nil {
		t.Fatal("paste should set an undo slot")
	}
	if m.undo.slot.action.kind() != undoKindPaste {
		t.Errorf("expected undoKindPaste, got %v", m.undo.slot.action.kind())
	}
	if !strings.Contains(m.undo.slot.action.description(), "1 track") {
		t.Errorf("unexpected undo description: %q", m.undo.slot.action.description())
	}
}

// TestUndoPasteRemovesPastedEntries verifies that undoing a paste removes the
// pasted entries and restores the queue to its previous state.
func TestUndoPasteRemovesPastedEntries(t *testing.T) {
	qa := &stubQueueAppender{}
	be := &stubBatchEnqueuer{}
	m := New(context.Background(), WithQueueWriter(qa), WithBatchEnqueuer(be))
	m.activeView = viewQueue
	m.queue.entries = sampleQueueEntries()
	m.queue.cursor = 1
	m.yankBuffer = []QueueEntry{m.queue.entries[0], m.queue.entries[2]}

	_, cmd, _ := m.handleQueueKeyAction(actionPaste)
	msg := cmd().(QueuePastedMsg)
	m = m.handleQueuePastedMsg(msg)

	if len(m.queue.entries) != 6 {
		t.Fatalf("expected 6 entries after paste, got %d", len(m.queue.entries))
	}

	// Undo the paste.
	next, _ := m.Update(UndoMsg{})
	m = next.(Model)

	if len(m.queue.entries) != 4 {
		t.Fatalf("expected 4 entries after undo, got %d", len(m.queue.entries))
	}
	// Original entries should be intact.
	for i, want := range []string{"h1", "h2", "h3", "h4"} {
		if m.queue.entries[i].Hash != want {
			t.Errorf("entry[%d] hash = %q, want %q", i, m.queue.entries[i].Hash, want)
		}
	}
	// Batch enqueuer should have been called to soft-delete.
	if len(be.dequeueTracksSoftIDs) != 1 {
		t.Fatalf("expected 1 DequeueTracksSoft call, got %d", len(be.dequeueTracksSoftIDs))
	}
	if len(be.dequeueTracksSoftIDs[0]) != 2 {
		t.Errorf("expected 2 IDs soft-deleted, got %d", len(be.dequeueTracksSoftIDs[0]))
	}
}

// TestUndoPasteAdjustsCurrentIndex verifies that undoing a paste that was
// inserted before the playing track shifts currentIndex back.
func TestUndoPasteAdjustsCurrentIndex(t *testing.T) {
	qa := &stubQueueAppender{}
	be := &stubBatchEnqueuer{}
	m := New(context.Background(), WithQueueWriter(qa), WithBatchEnqueuer(be))
	m.activeView = viewQueue
	m.queue.entries = sampleQueueEntries()
	m.playback.currentIndex = 2
	m.queue.cursor = 0
	m.yankBuffer = []QueueEntry{m.queue.entries[0]}

	_, cmd, _ := m.handleQueueKeyAction(actionPaste)
	msg := cmd().(QueuePastedMsg)
	m = m.handleQueuePastedMsg(msg)

	// currentIndex shifted from 2 to 3 by paste.
	if m.playback.currentIndex != 3 {
		t.Fatalf("expected currentIndex 3 after paste, got %d", m.playback.currentIndex)
	}

	// Undo should shift it back.
	next, _ := m.Update(UndoMsg{})
	m = next.(Model)

	if m.playback.currentIndex != 2 {
		t.Errorf("expected currentIndex 2 after undo, got %d", m.playback.currentIndex)
	}
}
