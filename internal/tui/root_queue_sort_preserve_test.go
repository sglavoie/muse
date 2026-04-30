package tui

import (
	"context"
	"testing"
)

// TestCutPastePreservesQueueSort verifies that handleQueuePastedMsg with
// CutIDs and an active sort recomputes displayOrder after removeCutOriginals
// shrinks the entries slice. Before the fix, the displayOrder computed before
// the removal remained stale (wrong length), causing filteredEntries to
// silently nil it out and drop back to insertion order.
func TestCutPastePreservesQueueSort(t *testing.T) {
	be := &stubBatchEnqueuer{}
	m := New(context.Background(), WithBatchEnqueuer(be))

	// Start with 4 entries: [A=101, B=102, C=103, D=104].
	m.queue.entries = sampleQueueEntries()

	// Activate a sort so that displayOrder is non-nil.
	m.queue.sortState = SortState{Field: SortTitle, Dir: SortAsc}
	m.queue = m.queue.recomputeDisplayOrder()
	if m.queue.displayOrder == nil {
		t.Fatal("precondition: displayOrder should be non-nil after sort")
	}

	// Simulate a cut-paste: paste a copy of A (with new ID 200) at position 4,
	// and mark the original A (ID=101) as a cut original to be removed.
	msg := QueuePastedMsg{
		InsertPos: 4,
		Entries: []QueueEntry{
			{ID: 200, Path: "/a.mp3", Hash: "h1", Display: "Artist - A", Artist: "Artist", Title: "A", Album: "Album", DurationMs: 180000},
		},
		CutIDs: []int64{101},
	}
	m = m.handleQueuePastedMsg(msg)

	// After cut-paste the queue still has 4 entries (1 removed + 1 added = net 0).
	if len(m.queue.entries) != 4 {
		t.Fatalf("expected 4 entries, got %d", len(m.queue.entries))
	}

	// The critical assertion: displayOrder must still be valid (non-nil and
	// correct length) because preserveQueueSort was called after the removal.
	if m.queue.displayOrder == nil {
		t.Fatal("displayOrder should be non-nil; sort was silently dropped")
	}
	if len(m.queue.displayOrder) != len(m.queue.entries) {
		t.Errorf("displayOrder length = %d, want %d",
			len(m.queue.displayOrder), len(m.queue.entries))
	}
}

// TestUndoBulkDequeuePreservesSort verifies that undoing a bulk dequeue
// recomputes displayOrder when a sort is active.
func TestUndoBulkDequeuePreservesSort(t *testing.T) {
	m := New(context.Background())
	m.queue.entries = []QueueEntry{
		{ID: 1, Title: "C", Hash: "h1"},
		{ID: 2, Title: "A", Hash: "h2"},
		{ID: 3, Title: "B", Hash: "h3"},
	}

	// Activate title sort.
	m.queue.sortState = SortState{Field: SortTitle, Dir: SortAsc}
	m.queue = m.queue.recomputeDisplayOrder()

	// Simulate removing entry ID=2 at index 1, then undoing.
	action := undoBulkDequeueAction{
		entries: []QueueEntry{{ID: 2, Title: "A", Hash: "h2"}},
		indices: []int{1},
		ids:     []int64{2},
	}

	// Remove the entry first so undo has something to restore.
	m.queue.entries = []QueueEntry{
		{ID: 1, Title: "C", Hash: "h1"},
		{ID: 3, Title: "B", Hash: "h3"},
	}

	m, _ = action.perform(m)

	if len(m.queue.entries) != 3 {
		t.Fatalf("expected 3 entries after undo, got %d", len(m.queue.entries))
	}
	if m.queue.displayOrder == nil {
		t.Fatal("displayOrder should be non-nil after undo with active sort")
	}
	if len(m.queue.displayOrder) != 3 {
		t.Errorf("displayOrder length = %d, want 3", len(m.queue.displayOrder))
	}
}

// TestUndoBatchEnqueuePreservesSort verifies that undoing a batch enqueue
// recomputes displayOrder when a sort is active.
func TestUndoBatchEnqueuePreservesSort(t *testing.T) {
	be := &stubBatchEnqueuer{}
	dequeuer := &stubQueueDequeuer{}
	m := New(context.Background(), WithBatchEnqueuer(be))
	m.undoer = dequeuer
	m.queue.entries = []QueueEntry{
		{ID: 1, Title: "C", Hash: "h1"},
		{ID: 2, Title: "A", Hash: "h2"},
		{ID: 3, Title: "B", Hash: "h3"},
	}

	// Activate sort.
	m.queue.sortState = SortState{Field: SortTitle, Dir: SortAsc}
	m.queue = m.queue.recomputeDisplayOrder()

	action := undoBatchEnqueueAction{
		ids:          []int64{2},
		count:        1,
		startIdx:     1,
		playlistName: "Test",
	}
	m, _ = action.perform(m)

	if len(m.queue.entries) != 2 {
		t.Fatalf("expected 2 entries after undo, got %d", len(m.queue.entries))
	}
	if m.queue.displayOrder == nil {
		t.Fatal("displayOrder should be non-nil after undo with active sort")
	}
	if len(m.queue.displayOrder) != 2 {
		t.Errorf("displayOrder length = %d, want 2", len(m.queue.displayOrder))
	}
}

// TestUndoPastePreservesSort verifies that undoing a paste recomputes
// displayOrder when a sort is active.
func TestUndoPastePreservesSort(t *testing.T) {
	be := &stubBatchEnqueuer{}
	dequeuer := &stubQueueDequeuer{}
	m := New(context.Background(), WithBatchEnqueuer(be))
	m.undoer = dequeuer
	m.queue.entries = []QueueEntry{
		{ID: 1, Title: "C", Hash: "h1"},
		{ID: 2, Title: "A", Hash: "h2"},
		{ID: 3, Title: "B", Hash: "h3"},
	}

	// Activate sort.
	m.queue.sortState = SortState{Field: SortTitle, Dir: SortAsc}
	m.queue = m.queue.recomputeDisplayOrder()

	action := undoPasteAction{
		ids:   []int64{2},
		count: 1,
	}
	m, _ = action.perform(m)

	if len(m.queue.entries) != 2 {
		t.Fatalf("expected 2 entries after undo, got %d", len(m.queue.entries))
	}
	if m.queue.displayOrder == nil {
		t.Fatal("displayOrder should be non-nil after undo with active sort")
	}
	if len(m.queue.displayOrder) != 2 {
		t.Errorf("displayOrder length = %d, want 2", len(m.queue.displayOrder))
	}
}

// TestUndoCutPastePreservesSort verifies that undoing a cut-paste recomputes
// displayOrder when a sort is active.
func TestUndoCutPastePreservesSort(t *testing.T) {
	qa := &stubQueueAppender{}
	dequeuer := &stubQueueDequeuer{}
	be := &stubBatchEnqueuer{}
	m := New(context.Background(), WithQueueWriter(qa), WithBatchEnqueuer(be))
	m.undoer = dequeuer
	m.queue.entries = []QueueEntry{
		{ID: 10, Title: "B", Hash: "h2"},
		{ID: 11, Title: "C", Hash: "h3"},
	}

	// Activate sort.
	m.queue.sortState = SortState{Field: SortTitle, Dir: SortAsc}
	m.queue = m.queue.recomputeDisplayOrder()

	action := undoCutPasteAction{
		pastedIDs:       []int64{10},
		originalEntries: []QueueEntry{{ID: 99, Title: "A", Hash: "h1", Path: "/a.mp3"}},
		originalIndices: []int{0},
		count:           1,
	}
	m, _ = action.perform(m)

	// After undo: removed ID=10, re-inserted original at index 0.
	// Queue should have 2 entries: the restored original + ID=11.
	if len(m.queue.entries) != 2 {
		t.Fatalf("expected 2 entries after undo, got %d", len(m.queue.entries))
	}
	if m.queue.displayOrder == nil {
		t.Fatal("displayOrder should be non-nil after undo with active sort")
	}
	if len(m.queue.displayOrder) != 2 {
		t.Errorf("displayOrder length = %d, want 2", len(m.queue.displayOrder))
	}
}

// TestUndoEnqueuePreservesSort verifies that undoing a single enqueue
// recomputes displayOrder when a sort is active.
func TestUndoEnqueuePreservesSort(t *testing.T) {
	dequeuer := &stubQueueDequeuer{}
	m := New(context.Background())
	m.undoer = dequeuer
	m.queue.entries = []QueueEntry{
		{ID: 1, Title: "C", Hash: "h1"},
		{ID: 2, Title: "A", Hash: "h2"},
		{ID: 3, Title: "B", Hash: "h3"},
	}

	// Activate sort.
	m.queue.sortState = SortState{Field: SortTitle, Dir: SortAsc}
	m.queue = m.queue.recomputeDisplayOrder()

	action := undoEnqueueAction{
		id:    2,
		entry: QueueEntry{ID: 2, Title: "A", Hash: "h2"},
		idx:   1,
	}
	m, _ = action.perform(m)

	if len(m.queue.entries) != 2 {
		t.Fatalf("expected 2 entries after undo, got %d", len(m.queue.entries))
	}
	if m.queue.displayOrder == nil {
		t.Fatal("displayOrder should be non-nil after undo with active sort")
	}
	if len(m.queue.displayOrder) != 2 {
		t.Errorf("displayOrder length = %d, want 2", len(m.queue.displayOrder))
	}
}

// TestUndoDequeuePreservesSort verifies that undoing a single dequeue
// recomputes displayOrder when a sort is active.
func TestUndoDequeuePreservesSort(t *testing.T) {
	dequeuer := &stubQueueDequeuer{}
	p := &stubPlayerQueueSyncer{}
	m := New(context.Background(), WithPlayer(p))
	m.undoer = dequeuer
	m.queue.entries = []QueueEntry{
		{ID: 1, Title: "C", Hash: "h1"},
		{ID: 3, Title: "B", Hash: "h3"},
	}

	// Activate sort.
	m.queue.sortState = SortState{Field: SortTitle, Dir: SortAsc}
	m.queue = m.queue.recomputeDisplayOrder()

	action := undoDequeueAction{
		id:    2,
		entry: QueueEntry{ID: 2, Title: "A", Hash: "h2"},
		idx:   1,
	}
	m, _ = action.perform(m)

	if len(m.queue.entries) != 3 {
		t.Fatalf("expected 3 entries after undo, got %d", len(m.queue.entries))
	}
	if m.queue.displayOrder == nil {
		t.Fatal("displayOrder should be non-nil after undo with active sort")
	}
	if len(m.queue.displayOrder) != 3 {
		t.Errorf("displayOrder length = %d, want 3", len(m.queue.displayOrder))
	}
}
