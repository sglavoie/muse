package tui

import (
	"context"
	"strings"
	"testing"
)

// TestCutPasteSetsUndoSlot verifies that a cut-paste sets an undoKindCutPaste
// undo slot.
func TestCutPasteSetsUndoSlot(t *testing.T) {
	qa := &stubQueueAppender{}
	be := &stubBatchEnqueuer{}
	m := New(context.Background(), WithQueueWriter(qa), WithBatchEnqueuer(be))
	m.activeView = viewQueue
	m.queue.entries = sampleQueueEntries()
	m.queue.cursor = 0

	rm, _, _ := m.handleQueueKeyAction(actionCut)
	m = rm.(Model)

	m.queue.cursor = 2
	rm, cmd, _ := m.handleQueueKeyAction(actionPaste)
	m = rm.(Model)
	msg := cmd().(QueuePastedMsg)
	m = m.handleQueuePastedMsg(msg)

	if m.undo.slot.action == nil {
		t.Fatal("cut-paste should set an undo slot")
	}
	if m.undo.slot.action.kind() != undoKindCutPaste {
		t.Errorf("expected undoKindCutPaste, got %v", m.undo.slot.action.kind())
	}
	if !strings.Contains(m.undo.slot.action.description(), "Moved") {
		t.Errorf("unexpected undo description: %q", m.undo.slot.action.description())
	}
}

// TestUndoCutPasteRestoresOriginals verifies that undoing a cut-paste removes
// the pasted copies and re-inserts the originals.
func TestUndoCutPasteRestoresOriginals(t *testing.T) {
	qa := &stubQueueAppender{}
	be := &stubBatchEnqueuer{}
	m := New(context.Background(), WithQueueWriter(qa), WithBatchEnqueuer(be))
	m.activeView = viewQueue
	m.queue.entries = sampleQueueEntries() // [A, B, C, D]
	m.queue.cursor = 0

	// Cut A.
	rm, _, _ := m.handleQueueKeyAction(actionCut)
	m = rm.(Model)

	// Paste after C (cursor=2 in the original, but we move it).
	m.queue.cursor = 2
	rm, cmd, _ := m.handleQueueKeyAction(actionPaste)
	m = rm.(Model)
	msg := cmd().(QueuePastedMsg)
	m = m.handleQueuePastedMsg(msg)

	// Verify cut-paste worked: A moved from index 0 to after C.
	if len(m.queue.entries) != 4 {
		t.Fatalf("expected 4 entries after cut-paste, got %d", len(m.queue.entries))
	}

	// Undo the cut-paste.
	next, _ := m.Update(UndoMsg{})
	m = next.(Model)

	if len(m.queue.entries) != 4 {
		t.Fatalf("expected 4 entries after undo, got %d", len(m.queue.entries))
	}
	// Original order should be restored: A, B, C, D.
	wantOrder := []string{"h1", "h2", "h3", "h4"}
	for i, want := range wantOrder {
		if m.queue.entries[i].Hash != want {
			t.Errorf("entry[%d] hash = %q, want %q", i, m.queue.entries[i].Hash, want)
		}
	}
}

// TestUndoCutPasteMultipleNonAdjacentRestoresOrder verifies that cutting
// multiple non-adjacent tracks, pasting them elsewhere, and then undoing
// restores the original queue order. This catches a bug where the undo used
// post-insert indices instead of pre-paste positions.
func TestUndoCutPasteMultipleNonAdjacentRestoresOrder(t *testing.T) {
	qa := &stubQueueAppender{}
	be := &stubBatchEnqueuer{}
	m := New(context.Background(), WithQueueWriter(qa), WithBatchEnqueuer(be))
	m.activeView = viewQueue
	m.queue.entries = sampleQueueEntries() // [A(101), B(102), C(103), D(104)]

	// Bulk-select B and D (indices 1, 3).
	m.queue.entries[1].Selected = true
	m.queue.entries[3].Selected = true

	// Cut the selection.
	rm, _, _ := m.handleQueueKeyAction(actionCut)
	m = rm.(Model)
	if len(m.yankBuffer) != 2 {
		t.Fatalf("expected 2 cut entries, got %d", len(m.yankBuffer))
	}

	// Paste after A (cursor=0 -> insertPos=1). Queue is [A, C] before paste.
	m.queue.cursor = 0
	rm, cmd, _ := m.handleQueueKeyAction(actionPaste)
	m = rm.(Model)
	msg := cmd().(QueuePastedMsg)
	m = m.handleQueuePastedMsg(msg)

	if len(m.queue.entries) != 4 {
		t.Fatalf("expected 4 entries after cut-paste, got %d", len(m.queue.entries))
	}

	// Undo the cut-paste.
	next, _ := m.Update(UndoMsg{})
	m = next.(Model)

	if len(m.queue.entries) != 4 {
		t.Fatalf("expected 4 entries after undo, got %d", len(m.queue.entries))
	}
	// Original order should be restored: A, B, C, D.
	wantOrder := []string{"h1", "h2", "h3", "h4"}
	for i, want := range wantOrder {
		if m.queue.entries[i].Hash != want {
			t.Errorf("entry[%d] hash = %q, want %q", i, m.queue.entries[i].Hash, want)
		}
	}
}

// TestUndoCutPasteThenPasteMoves verifies that after cut -> paste -> undo -> paste,
// the tracks are moved (not duplicated). The undo must restore yankIsCut and
// update yankBuffer IDs to match the re-inserted originals.
func TestUndoCutPasteThenPasteMoves(t *testing.T) {
	qa := &stubQueueAppender{}
	be := &stubBatchEnqueuer{}
	m := New(context.Background(), WithQueueWriter(qa), WithBatchEnqueuer(be))
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

	// Undo the cut-paste.
	next, _ := m.Update(UndoMsg{})
	m = next.(Model)

	if !m.yankIsCut {
		t.Fatal("yankIsCut should be restored after undo")
	}
	if len(m.queue.entries) != 4 {
		t.Fatalf("expected 4 entries after undo, got %d", len(m.queue.entries))
	}

	// Paste again after C - should move, not duplicate.
	m.queue.cursor = 2
	rm, cmd, _ = m.handleQueueKeyAction(actionPaste)
	m = rm.(Model)
	msg = cmd().(QueuePastedMsg)
	m = m.handleQueuePastedMsg(msg)

	if len(m.queue.entries) != 4 {
		t.Fatalf("expected 4 entries after re-paste (move), got %d", len(m.queue.entries))
	}
	// A should now be after C: [B, C, A, D].
	wantOrder := []string{"h2", "h3", "h1", "h4"}
	for i, want := range wantOrder {
		if m.queue.entries[i].Hash != want {
			t.Errorf("entry[%d] hash = %q, want %q", i, m.queue.entries[i].Hash, want)
		}
	}
}
