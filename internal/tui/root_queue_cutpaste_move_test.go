package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/tui/keys"
	"github.com/sglavoie/muse/internal/tui/queue"
)

// TestCutThenPasteRemovesOriginals verifies that pasting a cut buffer moves
// tracks: inserts copies at new position and removes originals.
func TestCutThenPasteRemovesOriginals(t *testing.T) {
	qa := &stubQueueAppender{}
	be := &stubBatchEnqueuer{}
	m := New(context.Background(), WithQueueWriter(qa), WithBatchEnqueuer(be))
	m.ActiveView = viewQueue
	m.queue.Entries = sampleQueueEntries() // [A=101, B=102, C=103, D=104]
	m.queue.Cursor = 0

	// Cut track A (ID=101).
	rm, _, _ := m.handleQueueKeyAction(keys.ActionCut)
	m = rm.(Model)

	// Move cursor to after D and paste.
	m.queue.Cursor = 3
	rm, cmd, _ := m.handleQueueKeyAction(keys.ActionPaste)
	m = rm.(Model)
	msg := cmd().(QueuePastedMsg)
	m = m.handleQueuePastedMsg(msg)

	// Queue should still have 4 entries (1 removed + 1 added = net 0).
	if len(m.queue.Entries) != 4 {
		t.Fatalf("expected 4 entries after cut-paste, got %d", len(m.queue.Entries))
	}
	// Track A should now be at the end.
	wantOrder := []string{"h2", "h3", "h4", "h1"}
	for i, want := range wantOrder {
		if m.queue.Entries[i].Hash != want {
			t.Errorf("entry[%d] hash = %q, want %q", i, m.queue.Entries[i].Hash, want)
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
	m.ActiveView = viewQueue
	m.queue.Entries = sampleQueueEntries() // [A=101, B=102, C=103, D=104]
	m.playback.currentIndex = 2            // playing C

	// Cut A (index 0, before currently playing).
	m.queue.Cursor = 0
	rm, _, _ := m.handleQueueKeyAction(keys.ActionCut)
	m = rm.(Model)

	// Paste at end.
	m.queue.Cursor = 3
	rm, cmd, _ := m.handleQueueKeyAction(keys.ActionPaste)
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
	if m.queue.Entries[m.playback.currentIndex].Hash != "h3" {
		t.Errorf("expected playing track to be h3, got %q", m.queue.Entries[m.playback.currentIndex].Hash)
	}
}

// TestCutPasteSecondPasteDuplicates verifies that after a cut-paste, a second
// paste duplicates instead of moving (cut state is cleared).
func TestCutPasteSecondPasteDuplicates(t *testing.T) {
	qa := &stubQueueAppender{}
	be := &stubBatchEnqueuer{}
	m := New(context.Background(), WithQueueWriter(qa), WithBatchEnqueuer(be))
	m.ActiveView = viewQueue
	m.queue.Entries = sampleQueueEntries() // [A, B, C, D]
	m.queue.Cursor = 0

	// Cut A.
	rm, _, _ := m.handleQueueKeyAction(keys.ActionCut)
	m = rm.(Model)

	// First paste (moves A).
	m.queue.Cursor = 3
	rm, cmd, _ := m.handleQueueKeyAction(keys.ActionPaste)
	m = rm.(Model)
	msg := cmd().(QueuePastedMsg)
	m = m.handleQueuePastedMsg(msg)

	if len(m.queue.Entries) != 4 {
		t.Fatalf("expected 4 entries after cut-paste, got %d", len(m.queue.Entries))
	}

	// Second paste should duplicate (yankIsCut is cleared).
	rm, cmd, _ = m.handleQueueKeyAction(keys.ActionPaste)
	m = rm.(Model)
	msg = cmd().(QueuePastedMsg)
	m = m.handleQueuePastedMsg(msg)

	if len(m.queue.Entries) != 5 {
		t.Fatalf("expected 5 entries after second paste (duplicate), got %d", len(m.queue.Entries))
	}
}

// TestCutPasteCursorPointsAtPastedEntry verifies that after a cut-paste where
// the cut originals are after the paste position, the cursor lands on the last
// pasted entry (not incorrectly shifted by originals removed after the cursor).
func TestCutPasteCursorPointsAtPastedEntry(t *testing.T) {
	qa := &stubQueueAppender{}
	be := &stubBatchEnqueuer{}
	m := New(context.Background(), WithQueueWriter(qa), WithBatchEnqueuer(be))
	m.ActiveView = viewQueue
	m.queue.Entries = sampleQueueEntries() // [A=101, B=102, C=103, D=104]
	m.queue.Cursor = 3                     // on D

	// Cut D (last track).
	rm, _, _ := m.handleQueueKeyAction(keys.ActionCut)
	m = rm.(Model)

	// Paste after A (cursor=0).
	m.queue.Cursor = 0
	rm, cmd, _ := m.handleQueueKeyAction(keys.ActionPaste)
	m = rm.(Model)
	msg := cmd().(QueuePastedMsg)
	m = m.handleQueuePastedMsg(msg)

	// Expected queue: [A, D', B, C] (original D removed, copy inserted at 1).
	if len(m.queue.Entries) != 4 {
		t.Fatalf("expected 4 entries after cut-paste, got %d", len(m.queue.Entries))
	}
	wantOrder := []string{"h1", "h4", "h2", "h3"}
	for i, want := range wantOrder {
		if m.queue.Entries[i].Hash != want {
			t.Errorf("entry[%d] hash = %q, want %q", i, m.queue.Entries[i].Hash, want)
		}
	}
	// Cursor should point at D' (index 1), the pasted entry.
	if m.queue.Cursor != 1 {
		t.Errorf("expected cursor at 1 (pasted entry), got %d", m.queue.Cursor)
	}
}

// TestCutPasteMultipleCursorPointsAtLastPasted verifies that cutting multiple
// tracks from after the paste position leaves the cursor on the last pasted
// entry, not incorrectly shifted backward.
func TestCutPasteMultipleCursorPointsAtLastPasted(t *testing.T) {
	qa := &stubQueueAppender{}
	be := &stubBatchEnqueuer{}
	m := New(context.Background(), WithQueueWriter(qa), WithBatchEnqueuer(be))
	m.ActiveView = viewQueue
	m.queue.Entries = sampleQueueEntries() // [A=101, B=102, C=103, D=104]

	// Select and cut C and D (indices 2 and 3).
	m.queue.Entries[2].Selected = true
	m.queue.Entries[3].Selected = true
	m.queue.Cursor = 2
	rm, _, _ := m.handleQueueKeyAction(keys.ActionCut)
	m = rm.(Model)

	// Paste after A (cursor=0).
	m.queue.Cursor = 0
	rm, cmd, _ := m.handleQueueKeyAction(keys.ActionPaste)
	m = rm.(Model)
	msg := cmd().(QueuePastedMsg)
	m = m.handleQueuePastedMsg(msg)

	// Expected queue: [A, C', D', B] - originals at indices 3,4 removed.
	if len(m.queue.Entries) != 4 {
		t.Fatalf("expected 4 entries after cut-paste, got %d", len(m.queue.Entries))
	}
	wantOrder := []string{"h1", "h3", "h4", "h2"}
	for i, want := range wantOrder {
		if m.queue.Entries[i].Hash != want {
			t.Errorf("entry[%d] hash = %q, want %q", i, m.queue.Entries[i].Hash, want)
		}
	}
	// Cursor should point at D' (index 2), the last pasted entry.
	if m.queue.Cursor != 2 {
		t.Errorf("expected cursor at 2 (last pasted entry), got %d", m.queue.Cursor)
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
	m.ActiveView = viewQueue
	m.queue.Entries = sampleQueueEntries() // [A=101, B=102, C=103, D=104]
	m.queue.Cursor = 0

	// Cut A.
	rm, _, _ := m.handleQueueKeyAction(keys.ActionCut)
	m = rm.(Model)

	// Paste after D.
	m.queue.Cursor = 3
	rm, cmd, _ := m.handleQueueKeyAction(keys.ActionPaste)
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

// TestCutPastePlayingTrackRelocatesCurrentIndex verifies that cutting and
// re-pasting the currently-playing track relocates playback.currentIndex to
// the pasted clone (same hash, new ID) instead of leaving it at -1. Before
// the fix, removeCutOriginals would call adjustPlaybackIndexAfterRemovals
// which sets currentIndex=-1 when the playing row is removed; the pasted
// clone with the same hash existed but the TUI lost its playback anchor.
func TestCutPastePlayingTrackRelocatesCurrentIndex(t *testing.T) {
	qa := &stubQueueAppender{}
	be := &stubBatchEnqueuer{}
	m := New(context.Background(), WithQueueWriter(qa), WithBatchEnqueuer(be))
	m.ActiveView = viewQueue
	m.queue.Entries = sampleQueueEntries() // [A=101, B=102, C=103, D=104]
	m.playback.currentIndex = 0            // playing A
	m.playback.currentHash = "h1"

	// Cut A (the currently playing track).
	m.queue.Cursor = 0
	rm, _, _ := m.handleQueueKeyAction(keys.ActionCut)
	m = rm.(Model)

	// Paste after D (cursor=3, insertPos=4).
	m.queue.Cursor = 3
	rm, cmd, _ := m.handleQueueKeyAction(keys.ActionPaste)
	m = rm.(Model)
	msg := cmd().(QueuePastedMsg)
	m = m.handleQueuePastedMsg(msg)

	// Expected queue: [B, C, D, A'] — A's clone is now last.
	if len(m.queue.Entries) != 4 {
		t.Fatalf("expected 4 entries, got %d", len(m.queue.Entries))
	}
	if m.queue.Entries[3].Hash != "h1" {
		t.Fatalf("expected pasted clone of A at index 3, got hash %q", m.queue.Entries[3].Hash)
	}
	// currentIndex must point at the pasted clone, NOT -1.
	if m.playback.currentIndex != 3 {
		t.Errorf("expected currentIndex 3 (pasted clone of playing track), got %d", m.playback.currentIndex)
	}
	if m.playback.currentHash != "h1" {
		t.Errorf("expected currentHash preserved as h1, got %q", m.playback.currentHash)
	}
}

// TestBulkDequeuePlayingTrackWithDuplicateRelocatesCurrentIndex verifies that
// removing the currently-playing row from a bulk selection while another
// queue row shares the same hash re-points playback.currentIndex at the
// surviving duplicate, rather than leaving it at -1.
func TestBulkDequeuePlayingTrackWithDuplicateRelocatesCurrentIndex(t *testing.T) {
	be := &stubBatchEnqueuer{}
	m := New(context.Background(), WithBatchEnqueuer(be))
	m.ActiveView = viewQueue
	m.queue.Entries = []queue.Entry{
		{ID: 1, Hash: "h1", Display: "A1"},
		{ID: 2, Hash: "h2", Display: "B"},
		{ID: 3, Hash: "h1", Display: "A2"}, // duplicate of h1
		{ID: 4, Hash: "h3", Display: "C"},
	}
	// Playing the first occurrence of h1.
	m.playback.currentIndex = 0
	m.playback.currentHash = "h1"

	// Select index 0 only (the playing row).
	m.queue.Entries[0].Selected = true

	rm, _, _ := m.handleSelectedQueueDequeueAction()
	m = rm.(Model)

	// After removal: [B, A2, C] — A2 (duplicate of h1) still present.
	if len(m.queue.Entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(m.queue.Entries))
	}
	// currentIndex should now point at A2 (index 1), not -1.
	if m.playback.currentIndex != 1 {
		t.Errorf("expected currentIndex 1 (surviving duplicate of h1), got %d", m.playback.currentIndex)
	}
	if m.playback.currentHash != "h1" {
		t.Errorf("expected currentHash preserved as h1, got %q", m.playback.currentHash)
	}
}
