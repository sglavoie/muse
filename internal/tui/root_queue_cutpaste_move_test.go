package tui

import (
	"context"
	"testing"
)

// TestCutThenPasteRemovesOriginals verifies that pasting a cut buffer moves
// tracks: inserts copies at new position and removes originals.
func TestCutThenPasteRemovesOriginals(t *testing.T) {
	qa := &stubQueueAppender{}
	be := &stubBatchEnqueuer{}
	m := New(context.Background(), WithQueueWriter(qa), WithBatchEnqueuer(be))
	m.activeView = viewQueue
	m.queue.entries = sampleQueueEntries() // [A=101, B=102, C=103, D=104]
	m.queue.cursor = 0

	// Cut track A (ID=101).
	rm, _, _ := m.handleQueueKeyAction(actionCut)
	m = rm.(Model)

	// Move cursor to after D and paste.
	m.queue.cursor = 3
	rm, cmd, _ := m.handleQueueKeyAction(actionPaste)
	m = rm.(Model)
	msg := cmd().(QueuePastedMsg)
	m = m.handleQueuePastedMsg(msg)

	// Queue should still have 4 entries (1 removed + 1 added = net 0).
	if len(m.queue.entries) != 4 {
		t.Fatalf("expected 4 entries after cut-paste, got %d", len(m.queue.entries))
	}
	// Track A should now be at the end.
	wantOrder := []string{"h2", "h3", "h4", "h1"}
	for i, want := range wantOrder {
		if m.queue.entries[i].Hash != want {
			t.Errorf("entry[%d] hash = %q, want %q", i, m.queue.entries[i].Hash, want)
		}
	}
	// Original ID 101 should have been soft-deleted.
	if len(be.dequeueTracksSoftIDs) != 1 || len(be.dequeueTracksSoftIDs[0]) != 1 {
		t.Fatalf("expected 1 soft-delete call with 1 ID, got %v", be.dequeueTracksSoftIDs)
	}
	if be.dequeueTracksSoftIDs[0][0] != 101 {
		t.Errorf("expected soft-deleted ID 101, got %d", be.dequeueTracksSoftIDs[0][0])
	}
	// yankIsCut should be cleared after paste.
	if m.yankIsCut {
		t.Error("yankIsCut should be false after cut-paste")
	}
}

// TestCutPasteAdjustsPlaybackIndex verifies that cut-paste correctly adjusts
// the playback index for both the insertion and removal.
func TestCutPasteAdjustsPlaybackIndex(t *testing.T) {
	qa := &stubQueueAppender{}
	be := &stubBatchEnqueuer{}
	m := New(context.Background(), WithQueueWriter(qa), WithBatchEnqueuer(be))
	m.activeView = viewQueue
	m.queue.entries = sampleQueueEntries() // [A=101, B=102, C=103, D=104]
	m.playback.currentIndex = 2            // playing C

	// Cut A (index 0, before currently playing).
	m.queue.cursor = 0
	rm, _, _ := m.handleQueueKeyAction(actionCut)
	m = rm.(Model)

	// Paste at end.
	m.queue.cursor = 3
	rm, cmd, _ := m.handleQueueKeyAction(actionPaste)
	m = rm.(Model)
	msg := cmd().(QueuePastedMsg)
	m = m.handleQueuePastedMsg(msg)

	// After insert at pos 4: [A, B, C, D, A'] -> currentIndex shifts to 3 (insert at 4 > 2, no shift)
	// Wait, insert at cursor+1=4, which is > currentIndex=2, no shift from insert.
	// After removing original A at index 0 (before currentIndex=2): currentIndex = 2-1 = 1.
	// Result: [B, C, D, A'] with currentIndex=1 (still C).
	if m.playback.currentIndex != 1 {
		t.Errorf("expected currentIndex 1 (still playing C), got %d", m.playback.currentIndex)
	}
	if m.queue.entries[m.playback.currentIndex].Hash != "h3" {
		t.Errorf("expected playing track to be h3, got %q", m.queue.entries[m.playback.currentIndex].Hash)
	}
}

// TestCutPasteSecondPasteDuplicates verifies that after a cut-paste, a second
// paste duplicates instead of moving (cut state is cleared).
func TestCutPasteSecondPasteDuplicates(t *testing.T) {
	qa := &stubQueueAppender{}
	be := &stubBatchEnqueuer{}
	m := New(context.Background(), WithQueueWriter(qa), WithBatchEnqueuer(be))
	m.activeView = viewQueue
	m.queue.entries = sampleQueueEntries() // [A, B, C, D]
	m.queue.cursor = 0

	// Cut A.
	rm, _, _ := m.handleQueueKeyAction(actionCut)
	m = rm.(Model)

	// First paste (moves A).
	m.queue.cursor = 3
	rm, cmd, _ := m.handleQueueKeyAction(actionPaste)
	m = rm.(Model)
	msg := cmd().(QueuePastedMsg)
	m = m.handleQueuePastedMsg(msg)

	if len(m.queue.entries) != 4 {
		t.Fatalf("expected 4 entries after cut-paste, got %d", len(m.queue.entries))
	}

	// Second paste should duplicate (yankIsCut is cleared).
	rm, cmd, _ = m.handleQueueKeyAction(actionPaste)
	m = rm.(Model)
	msg = cmd().(QueuePastedMsg)
	m = m.handleQueuePastedMsg(msg)

	if len(m.queue.entries) != 5 {
		t.Fatalf("expected 5 entries after second paste (duplicate), got %d", len(m.queue.entries))
	}
}

// TestCutPasteCursorPointsAtPastedEntry verifies that after a cut-paste where
// the cut originals are after the paste position, the cursor lands on the last
// pasted entry (not incorrectly shifted by originals removed after the cursor).
func TestCutPasteCursorPointsAtPastedEntry(t *testing.T) {
	qa := &stubQueueAppender{}
	be := &stubBatchEnqueuer{}
	m := New(context.Background(), WithQueueWriter(qa), WithBatchEnqueuer(be))
	m.activeView = viewQueue
	m.queue.entries = sampleQueueEntries() // [A=101, B=102, C=103, D=104]
	m.queue.cursor = 3                     // on D

	// Cut D (last track).
	rm, _, _ := m.handleQueueKeyAction(actionCut)
	m = rm.(Model)

	// Paste after A (cursor=0).
	m.queue.cursor = 0
	rm, cmd, _ := m.handleQueueKeyAction(actionPaste)
	m = rm.(Model)
	msg := cmd().(QueuePastedMsg)
	m = m.handleQueuePastedMsg(msg)

	// Expected queue: [A, D', B, C] (original D removed, copy inserted at 1).
	if len(m.queue.entries) != 4 {
		t.Fatalf("expected 4 entries after cut-paste, got %d", len(m.queue.entries))
	}
	wantOrder := []string{"h1", "h4", "h2", "h3"}
	for i, want := range wantOrder {
		if m.queue.entries[i].Hash != want {
			t.Errorf("entry[%d] hash = %q, want %q", i, m.queue.entries[i].Hash, want)
		}
	}
	// Cursor should point at D' (index 1), the pasted entry.
	if m.queue.cursor != 1 {
		t.Errorf("expected cursor at 1 (pasted entry), got %d", m.queue.cursor)
	}
}

// TestCutPasteMultipleCursorPointsAtLastPasted verifies that cutting multiple
// tracks from after the paste position leaves the cursor on the last pasted
// entry, not incorrectly shifted backward.
func TestCutPasteMultipleCursorPointsAtLastPasted(t *testing.T) {
	qa := &stubQueueAppender{}
	be := &stubBatchEnqueuer{}
	m := New(context.Background(), WithQueueWriter(qa), WithBatchEnqueuer(be))
	m.activeView = viewQueue
	m.queue.entries = sampleQueueEntries() // [A=101, B=102, C=103, D=104]

	// Select and cut C and D (indices 2 and 3).
	m.queue.entries[2].Selected = true
	m.queue.entries[3].Selected = true
	m.queue.cursor = 2
	rm, _, _ := m.handleQueueKeyAction(actionCut)
	m = rm.(Model)

	// Paste after A (cursor=0).
	m.queue.cursor = 0
	rm, cmd, _ := m.handleQueueKeyAction(actionPaste)
	m = rm.(Model)
	msg := cmd().(QueuePastedMsg)
	m = m.handleQueuePastedMsg(msg)

	// Expected queue: [A, C', D', B] - originals at indices 3,4 removed.
	if len(m.queue.entries) != 4 {
		t.Fatalf("expected 4 entries after cut-paste, got %d", len(m.queue.entries))
	}
	wantOrder := []string{"h1", "h3", "h4", "h2"}
	for i, want := range wantOrder {
		if m.queue.entries[i].Hash != want {
			t.Errorf("entry[%d] hash = %q, want %q", i, m.queue.entries[i].Hash, want)
		}
	}
	// Cursor should point at D' (index 2), the last pasted entry.
	if m.queue.cursor != 2 {
		t.Errorf("expected cursor at 2 (last pasted entry), got %d", m.queue.cursor)
	}
}

// TestCutPastePreservesPurgeCancelChannel verifies that a cut-paste preserves the
// purge cancel channel so that the soft-deleted originals are eventually purged.
// Before the fix, setUndoSlot(..., nil) would close the cancel channel created by
// schedulePurge, permanently preventing the purge timer from firing.
func TestCutPastePreservesPurgeCancelChannel(t *testing.T) {
	qa := &stubQueueAppender{}
	be := &stubBatchEnqueuer{}
	dequeuer := &stubQueueDequeuer{}
	m := New(context.Background(), WithQueueWriter(qa), WithBatchEnqueuer(be))
	m.undoer = dequeuer
	m.activeView = viewQueue
	m.queue.entries = sampleQueueEntries() // [A=101, B=102, C=103, D=104]
	m.queue.cursor = 0

	// Cut A.
	rm, _, _ := m.handleQueueKeyAction(actionCut)
	m = rm.(Model)

	// Paste after D.
	m.queue.cursor = 3
	rm, cmd, _ := m.handleQueueKeyAction(actionPaste)
	m = rm.(Model)
	msg := cmd().(QueuePastedMsg)
	m = m.handleQueuePastedMsg(msg)

	// The undo slot should have a non-nil cancelPurge channel - this is the
	// channel that keeps the purge timer alive for the soft-deleted originals.
	if m.undo.slot.cancelPurge == nil {
		t.Fatal("cut-paste undo slot should have a non-nil cancelPurge channel; " +
			"without it the purge timer for soft-deleted originals is immediately cancelled")
	}
	// The channel should not be closed (select on it should not succeed).
	select {
	case <-m.undo.slot.cancelPurge:
		t.Fatal("cancelPurge channel should not be closed")
	default:
		// expected: channel is open
	}
}
