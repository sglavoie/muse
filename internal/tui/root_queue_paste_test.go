package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/sglavoie/muse/internal/tui/keys"
	"github.com/sglavoie/muse/internal/tui/queue"
)

// TestPasteSingleTrack verifies that ctrl+p inserts a copy below the cursor.
func TestPasteSingleTrack(t *testing.T) {
	qa := &stubQueueAppender{}
	m := New(context.Background(), WithQueueWriter(qa))
	m.activeView = viewQueue
	m.queue.Entries = sampleQueueEntries()
	m.queue.Cursor = 1
	m.yankBuffer = []queue.Entry{m.queue.Entries[0]}

	rm, cmd, handled := m.handleQueueKeyAction(keys.ActionPaste)
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
	if len(m.queue.Entries) != 5 {
		t.Fatalf("expected 5 entries after paste, got %d", len(m.queue.Entries))
	}
	// Pasted entry should be at position 2 (after cursor=1).
	if m.queue.Entries[2].Hash != "h1" {
		t.Errorf("expected pasted entry at index 2 with hash h1, got %q", m.queue.Entries[2].Hash)
	}
	// Cursor should move to the last pasted entry.
	if m.queue.Cursor != 2 {
		t.Errorf("expected cursor at 2, got %d", m.queue.Cursor)
	}
}

// TestPastePersistsFinalQueueOrder verifies that cursor-position paste does not
// leave the DB order at AppendTrack's tail position.
func TestPastePersistsFinalQueueOrder(t *testing.T) {
	qa := &stubQueueAppender{}
	r := &stubQueueReorderer{}
	m := New(context.Background(), WithQueueWriter(qa), WithQueueReorderer(r))
	m.activeView = viewQueue
	m.queue.Entries = sampleQueueEntries()
	m.queue.Cursor = 1
	m.yankBuffer = []queue.Entry{m.queue.Entries[0]}

	_, cmd, _ := m.handleQueueKeyAction(keys.ActionPaste)
	msg := cmd().(QueuePastedMsg)
	m = m.handleQueuePastedMsg(msg)

	if len(r.setQueueOrderCalls) != 1 {
		t.Fatalf("expected 1 SetQueueOrder call, got %d", len(r.setQueueOrderCalls))
	}
	want := []int64{101, 102, msg.Entries[0].ID, 103, 104}
	got := r.setQueueOrderCalls[0]
	if len(got) != len(want) {
		t.Fatalf("SetQueueOrder IDs length = %d, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("SetQueueOrder IDs[%d] = %d, want %d; full order %v", i, got[i], want[i], got)
		}
	}
}

// TestPasteMultipleTracks verifies that pasting multiple tracks inserts them
// contiguously below the cursor.
func TestPasteMultipleTracks(t *testing.T) {
	qa := &stubQueueAppender{}
	m := New(context.Background(), WithQueueWriter(qa))
	m.activeView = viewQueue
	m.queue.Entries = sampleQueueEntries()
	m.queue.Cursor = 0
	m.yankBuffer = []queue.Entry{m.queue.Entries[2], m.queue.Entries[3]}

	_, cmd, _ := m.handleQueueKeyAction(keys.ActionPaste)
	msg := cmd().(QueuePastedMsg)
	m = m.handleQueuePastedMsg(msg)

	if len(m.queue.Entries) != 6 {
		t.Fatalf("expected 6 entries after paste, got %d", len(m.queue.Entries))
	}
	// Entries at positions 1 and 2 should be the pasted tracks.
	if m.queue.Entries[1].Hash != "h3" || m.queue.Entries[2].Hash != "h4" {
		t.Errorf("pasted entries at wrong positions: [1]=%q [2]=%q",
			m.queue.Entries[1].Hash, m.queue.Entries[2].Hash)
	}
	if m.queue.Cursor != 2 {
		t.Errorf("cursor should be at last pasted entry (2), got %d", m.queue.Cursor)
	}
}

// TestPasteTwice verifies that the yank buffer persists across multiple pastes.
func TestPasteTwice(t *testing.T) {
	qa := &stubQueueAppender{}
	m := New(context.Background(), WithQueueWriter(qa))
	m.activeView = viewQueue
	m.queue.Entries = sampleQueueEntries()
	m.queue.Cursor = 0
	m.yankBuffer = []queue.Entry{m.queue.Entries[0]}

	// First paste.
	_, cmd, _ := m.handleQueueKeyAction(keys.ActionPaste)
	msg := cmd().(QueuePastedMsg)
	m = m.handleQueuePastedMsg(msg)
	if len(m.queue.Entries) != 5 {
		t.Fatalf("expected 5 after first paste, got %d", len(m.queue.Entries))
	}

	// Second paste (cursor moved to 1 after first paste).
	_, cmd, _ = m.handleQueueKeyAction(keys.ActionPaste)
	msg = cmd().(QueuePastedMsg)
	m = m.handleQueuePastedMsg(msg)
	if len(m.queue.Entries) != 6 {
		t.Fatalf("expected 6 after second paste, got %d", len(m.queue.Entries))
	}
}

// TestPasteEmptyBuffer verifies notification when nothing has been yanked.
func TestPasteEmptyBuffer(t *testing.T) {
	qa := &stubQueueAppender{}
	m := New(context.Background(), WithQueueWriter(qa))
	m.activeView = viewQueue
	m.queue.Entries = sampleQueueEntries()

	rm, cmd, handled := m.handleQueueKeyAction(keys.ActionPaste)
	m = rm.(Model)

	if !handled {
		t.Fatal("actionPaste should be handled")
	}
	if cmd != nil {
		t.Fatal("expected nil cmd when yank buffer is empty")
	}
	if !strings.Contains(m.statusBar.TopNotification().Msg, "Nothing to paste") {
		t.Errorf("expected 'Nothing to paste' notification, got %q", m.statusBar.TopNotification().Msg)
	}
}

// TestPasteWhileSorted verifies paste is blocked when queue is sorted.
func TestPasteWhileSorted(t *testing.T) {
	qa := &stubQueueAppender{}
	m := New(context.Background(), WithQueueWriter(qa))
	m.activeView = viewQueue
	m.queue.Entries = sampleQueueEntries()
	m.yankBuffer = []queue.Entry{m.queue.Entries[0]}
	m.queue.DisplayOrder = []int{3, 2, 1, 0} // non-nil = sorted

	rm, cmd, handled := m.handleQueueKeyAction(keys.ActionPaste)
	m = rm.(Model)

	if !handled {
		t.Fatal("actionPaste should be handled")
	}
	if cmd != nil {
		t.Fatal("expected nil cmd when sorted")
	}
	if !strings.Contains(m.statusBar.TopNotification().Msg, "Cannot paste") {
		t.Errorf("expected 'Cannot paste' notification, got %q", m.statusBar.TopNotification().Msg)
	}
}

// TestPasteWhileFiltered verifies paste is blocked when filter is active.
func TestPasteWhileFiltered(t *testing.T) {
	qa := &stubQueueAppender{}
	m := New(context.Background(), WithQueueWriter(qa))
	m.activeView = viewQueue
	m.queue.Entries = sampleQueueEntries()
	m.yankBuffer = []queue.Entry{m.queue.Entries[0]}
	m.queue.Filter.Text = "artist"

	rm, cmd, handled := m.handleQueueKeyAction(keys.ActionPaste)
	m = rm.(Model)

	if !handled {
		t.Fatal("actionPaste should be handled")
	}
	if cmd != nil {
		t.Fatal("expected nil cmd when filtered")
	}
	if !strings.Contains(m.statusBar.TopNotification().Msg, "Cannot paste") {
		t.Errorf("expected 'Cannot paste' notification, got %q", m.statusBar.TopNotification().Msg)
	}
}

// TestPasteWhileTagFiltered verifies paste is blocked when a cross-view tag
// filter is active because the visible cursor is not an underlying insert
// position.
func TestPasteWhileTagFiltered(t *testing.T) {
	qa := &stubQueueAppender{}
	m := New(context.Background(), WithQueueWriter(qa))
	m.activeView = viewQueue
	m.queue.Entries = sampleQueueEntries()
	m.yankBuffer = []queue.Entry{m.queue.Entries[0]}
	m.queue.TagFilterHashes = map[string]struct{}{"h2": {}, "h3": {}}

	rm, cmd, handled := m.handleQueueKeyAction(keys.ActionPaste)
	m = rm.(Model)

	if !handled {
		t.Fatal("actionPaste should be handled")
	}
	if cmd != nil {
		t.Fatal("expected nil cmd when tag-filtered")
	}
	if !strings.Contains(m.statusBar.TopNotification().Msg, "Cannot paste") {
		t.Errorf("expected 'Cannot paste' notification, got %q", m.statusBar.TopNotification().Msg)
	}
}

// TestPasteAdjustsCurrentIndex verifies that pasting before the currently
// playing track shifts currentIndex forward.
func TestPasteAdjustsCurrentIndex(t *testing.T) {
	qa := &stubQueueAppender{}
	m := New(context.Background(), WithQueueWriter(qa))
	m.activeView = viewQueue
	m.queue.Entries = sampleQueueEntries()
	m.playback.currentIndex = 2
	m.queue.Cursor = 0
	m.yankBuffer = []queue.Entry{m.queue.Entries[0]}

	_, cmd, _ := m.handleQueueKeyAction(keys.ActionPaste)
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
	m.queue.Entries = sampleQueueEntries()
	m.queue.Cursor = 3 // last entry
	m.yankBuffer = []queue.Entry{m.queue.Entries[0]}

	_, cmd, _ := m.handleQueueKeyAction(keys.ActionPaste)
	msg := cmd().(QueuePastedMsg)
	m = m.handleQueuePastedMsg(msg)

	if len(m.queue.Entries) != 5 {
		t.Fatalf("expected 5 entries, got %d", len(m.queue.Entries))
	}
	// Pasted entry should be at the end.
	if m.queue.Entries[4].Hash != "h1" {
		t.Errorf("expected pasted entry at end with hash h1, got %q", m.queue.Entries[4].Hash)
	}
	if m.queue.Cursor != 4 {
		t.Errorf("expected cursor at 4, got %d", m.queue.Cursor)
	}
}

// TestPasteSetsUndoSlot verifies that paste sets up an undo slot.
func TestPasteSetsUndoSlot(t *testing.T) {
	qa := &stubQueueAppender{}
	m := New(context.Background(), WithQueueWriter(qa))
	m.activeView = viewQueue
	m.queue.Entries = sampleQueueEntries()
	m.queue.Cursor = 1
	m.yankBuffer = []queue.Entry{m.queue.Entries[0]}

	_, cmd, _ := m.handleQueueKeyAction(keys.ActionPaste)
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
	m.queue.Entries = sampleQueueEntries()
	m.queue.Cursor = 1
	m.yankBuffer = []queue.Entry{m.queue.Entries[0], m.queue.Entries[2]}

	_, cmd, _ := m.handleQueueKeyAction(keys.ActionPaste)
	msg := cmd().(QueuePastedMsg)
	m = m.handleQueuePastedMsg(msg)

	if len(m.queue.Entries) != 6 {
		t.Fatalf("expected 6 entries after paste, got %d", len(m.queue.Entries))
	}

	// Undo the paste.
	next, _ := m.Update(UndoMsg{})
	m = next.(Model)

	if len(m.queue.Entries) != 4 {
		t.Fatalf("expected 4 entries after undo, got %d", len(m.queue.Entries))
	}
	// Original entries should be intact.
	for i, want := range []string{"h1", "h2", "h3", "h4"} {
		if m.queue.Entries[i].Hash != want {
			t.Errorf("entry[%d] hash = %q, want %q", i, m.queue.Entries[i].Hash, want)
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
	m.queue.Entries = sampleQueueEntries()
	m.playback.currentIndex = 2
	m.queue.Cursor = 0
	m.yankBuffer = []queue.Entry{m.queue.Entries[0]}

	_, cmd, _ := m.handleQueueKeyAction(keys.ActionPaste)
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
