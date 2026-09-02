package tui

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/sorting"
)

// TestUndoMsgRestoresDequeueWithinWindow verifies that receiving UndoMsg within
// the undo window calls UndoDequeue on the DB writer and restores the entry to
// the TUI slice at its original index.
func TestUndoMsgRestoresDequeueWithinWindow(t *testing.T) {
	d := &stubQueueDequeuer{}
	m := New(context.Background(), WithQueueDequeuer(d))
	m.queue.entries = []QueueEntry{
		{ID: 10, Path: "/music/a.flac", Display: "A"},
		{ID: 20, Path: "/music/b.flac", Display: "B"},
		{ID: 30, Path: "/music/c.flac", Display: "C"},
	}

	next, _ := m.Update(DequeueTrackMsg{Index: 1})
	nm := next.(Model)
	if len(nm.queue.entries) != 2 {
		t.Fatalf("after dequeue: expected 2 entries, got %d", len(nm.queue.entries))
	}

	next, _ = nm.Update(UndoMsg{})
	nm = next.(Model)

	if len(nm.queue.entries) != 3 {
		t.Fatalf("after undo: expected 3 entries, got %d", len(nm.queue.entries))
	}
	if nm.queue.entries[1].ID != 20 {
		t.Fatalf("after undo: expected restored entry at index 1 with ID=20, got ID=%d", nm.queue.entries[1].ID)
	}
	if len(d.undoDequeueCallIDs) != 1 || d.undoDequeueCallIDs[0] != 20 {
		t.Fatalf("UndoDequeue must be called with ID=20, got calls=%v", d.undoDequeueCallIDs)
	}
	if nm.undo.slot.action != nil {
		t.Fatal("undo slot must be cleared after a successful undo")
	}
}

// TestUndoMsgIsNoOpAfterWindowExpiry verifies that UndoMsg received after the
// 5-second window has elapsed is a no-op.
func TestUndoMsgIsNoOpAfterWindowExpiry(t *testing.T) {
	d := &stubQueueDequeuer{}
	m := New(context.Background(), WithQueueDequeuer(d))
	m.queue.entries = []QueueEntry{
		{ID: 10, Path: "/music/a.flac", Display: "A"},
		{ID: 20, Path: "/music/b.flac", Display: "B"},
	}

	next, _ := m.Update(DequeueTrackMsg{Index: 0})
	nm := next.(Model)
	nm.undo.slot.expiresAt = time.Now().Add(-1 * time.Second)

	next, _ = nm.Update(UndoMsg{})
	nm = next.(Model)

	if len(nm.queue.entries) != 1 {
		t.Fatalf("undo after expiry: expected 1 entry (no restore), got %d", len(nm.queue.entries))
	}
	if len(d.undoDequeueCallIDs) != 0 {
		t.Fatalf("undo after expiry: UndoDequeue must not be called, got calls=%v", d.undoDequeueCallIDs)
	}
}

// TestNewDestructiveActionReplacesUndoSlot verifies that a second destructive
// action before the first undo window expires overwrites the undo slot.
func TestNewDestructiveActionReplacesUndoSlot(t *testing.T) {
	d := &stubQueueDequeuer{}
	m := New(context.Background(), WithQueueDequeuer(d))
	m.queue.entries = []QueueEntry{
		{ID: 10, Path: "/music/a.flac", Display: "A"},
		{ID: 20, Path: "/music/b.flac", Display: "B"},
		{ID: 30, Path: "/music/c.flac", Display: "C"},
	}

	next, _ := m.Update(DequeueTrackMsg{Index: 0})
	nm := next.(Model)
	firstSlotID := nm.undo.slot.action.(undoDequeueAction).id

	next, _ = nm.Update(DequeueTrackMsg{Index: 0})
	nm = next.(Model)

	if nm.undo.slot.action.(undoDequeueAction).id == firstSlotID {
		t.Fatal("second dequeue must replace the undo slot with the new entry's ID")
	}
	if nm.undo.slot.action.(undoDequeueAction).id != 20 {
		t.Fatalf("undo slot must hold ID=20 after second dequeue, got %d", nm.undo.slot.action.(undoDequeueAction).id)
	}
}

// TestUndoMsgRestoresClearQueueWithinWindow verifies that UndoMsg within the
// window calls UndoClearQueue on the DB writer and restores all entries.
func TestUndoMsgRestoresClearQueueWithinWindow(t *testing.T) {
	d := &stubQueueDequeuer{clearQueueSoftResult: []int64{10, 20}}
	m := New(context.Background(), WithQueueDequeuer(d))
	m.queue.entries = []QueueEntry{
		{ID: 10, Path: "/music/a.flac", Display: "A"},
		{ID: 20, Path: "/music/b.flac", Display: "B"},
	}

	next, _ := m.Update(ClearQueueMsg{})
	nm := next.(Model)
	if len(nm.queue.entries) != 0 {
		t.Fatalf("after clear: expected 0 entries, got %d", len(nm.queue.entries))
	}

	next, _ = nm.Update(UndoMsg{})
	nm = next.(Model)

	if len(nm.queue.entries) != 2 {
		t.Fatalf("after undo clear: expected 2 entries, got %d", len(nm.queue.entries))
	}
	if len(d.undoClearQueueCalls) != 1 {
		t.Fatalf("UndoClearQueue must be called once, got %d calls", len(d.undoClearQueueCalls))
	}
	// Verify the IDs passed to UndoClearQueue match the original entry IDs.
	gotIDs := d.undoClearQueueCalls[0]
	if len(gotIDs) != 2 || gotIDs[0] != 10 || gotIDs[1] != 20 {
		t.Fatalf("UndoClearQueue called with IDs %v, want [10 20]", gotIDs)
	}
	if nm.undo.slot.action != nil {
		t.Fatal("undo slot must be cleared after a successful undo")
	}
}

// TestUndoClearQueueRestoresPlaybackIndex verifies that undoing a queue clear
// restores the playback index, hash, and syncs the player's current index.
func TestUndoClearQueueRestoresPlaybackIndex(t *testing.T) {
	p := &stubPlayerQueueSyncer{}
	d := &stubQueueDequeuer{clearQueueSoftResult: []int64{10, 20}}
	m := New(context.Background(), WithPlayer(p), WithQueueDequeuer(d))
	m.queue.entries = []QueueEntry{
		{ID: 10, Path: "/music/a.flac", Hash: "ha", Artist: "A", Title: "Ta"},
		{ID: 20, Path: "/music/b.flac", Hash: "hb", Artist: "B", Title: "Tb"},
	}
	m.playback.currentIndex = 1
	m.playback.currentHash = "hb"
	m.playback.state = playback.StatePlaying

	next, _ := m.Update(ClearQueueMsg{})
	nm := next.(Model)
	if nm.playback.currentIndex != -1 {
		t.Fatalf("after clear: expected currentIndex -1, got %d", nm.playback.currentIndex)
	}

	next, _ = nm.Update(UndoMsg{})
	nm = next.(Model)

	if nm.playback.currentIndex != 1 {
		t.Fatalf("after undo clear: expected currentIndex 1, got %d", nm.playback.currentIndex)
	}
	if nm.playback.currentHash != "hb" {
		t.Fatalf("after undo clear: expected currentHash %q, got %q", "hb", nm.playback.currentHash)
	}
	// Verify the player's SetCurrentIndex was called with the restored index.
	if len(p.setCurrentIndexCalls) == 0 {
		t.Fatal("SetCurrentIndex must be called on undo clear")
	}
	lastIdx := p.setCurrentIndexCalls[len(p.setCurrentIndexCalls)-1]
	if lastIdx != 1 {
		t.Fatalf("SetCurrentIndex: want 1, got %d", lastIdx)
	}
}

// TestUndoMsgWithNoSlotIsNoOp verifies that UndoMsg when no destructive action
// has been performed is a no-op.
func TestUndoMsgWithNoSlotIsNoOp(t *testing.T) {
	m := New(context.Background())
	m.queue.entries = []QueueEntry{{ID: 10, Path: "/music/a.flac", Display: "A"}}

	next, _ := m.Update(UndoMsg{})
	nm := next.(Model)

	if len(nm.queue.entries) != 1 {
		t.Fatalf("UndoMsg with no slot: expected 1 entry unchanged, got %d", len(nm.queue.entries))
	}
}

// TestUndoDequeueRebuildPlayerQueue verifies that undoing a dequeue calls
// SetQueue on the player so the player's in-memory queue stays in sync.
func TestUndoDequeueRebuildPlayerQueue(t *testing.T) {
	p := &stubPlayerQueueSyncer{}
	d := &stubQueueDequeuer{}
	m := New(context.Background(), WithPlayer(p), WithQueueDequeuer(d))
	m.queue.entries = []QueueEntry{
		{ID: 10, Path: "/music/a.flac", Hash: "ha", Artist: "A", Title: "Ta"},
		{ID: 20, Path: "/music/b.flac", Hash: "hb", Artist: "B", Title: "Tb"},
		{ID: 30, Path: "/music/c.flac", Hash: "hc", Artist: "C", Title: "Tc"},
	}

	next, _ := m.Update(DequeueTrackMsg{Index: 1})
	nm := next.(Model)
	next, _ = nm.Update(UndoMsg{})
	nm = next.(Model)

	if len(nm.queue.entries) != 3 {
		t.Fatalf("after undo: expected 3 entries, got %d", len(nm.queue.entries))
	}
	if len(p.setQueueCalls) == 0 {
		t.Fatal("undo dequeue must call SetQueue to rebuild the player queue")
	}
	lastCall := p.setQueueCalls[len(p.setQueueCalls)-1]
	if len(lastCall) != 3 {
		t.Fatalf("SetQueue called with %d tracks, want 3", len(lastCall))
	}
	if lastCall[1].Hash != "hb" {
		t.Errorf("restored player queue entry at index 1: want hash %q, got %q", "hb", lastCall[1].Hash)
	}
}

// TestUndoDequeueEnsuresCursorVisibility verifies that after undoing a dequeue,
// the scroll offset is adjusted so the restored entry is within the viewport.
func TestUndoDequeueEnsuresCursorVisibility(t *testing.T) {
	m := New(context.Background(), WithPlayer(&stubPlayer{}))
	m.queue.height = 10

	entries := make([]QueueEntry, 20)
	for i := range entries {
		entries[i] = QueueEntry{ID: int64(i + 1), Path: "/a.flac", Display: "Track"}
	}
	dequeued := entries[15]
	m.queue.entries = slices.Concat(entries[:15], entries[16:])
	m.queue.scrollOffset = 0
	m.queue.cursor = 0

	undoer := &stubQueueDequeuer{}
	m.undoer = undoer
	m.undo.slot = undoSlot{
		action:    undoDequeueAction{id: dequeued.ID, entry: dequeued, idx: 15},
		expiresAt: time.Now().Add(undoWindowDuration),
	}

	next, _ := m.Update(UndoMsg{})
	nm := next.(Model)

	if nm.queue.cursor != 15 {
		t.Fatalf("cursor = %d, want 15 after undo dequeue", nm.queue.cursor)
	}
	maxRows := nm.queue.visibleRows()
	if nm.queue.cursor < nm.queue.scrollOffset || nm.queue.cursor >= nm.queue.scrollOffset+maxRows {
		t.Errorf("cursor %d not visible: scrollOffset=%d, visibleRows=%d",
			nm.queue.cursor, nm.queue.scrollOffset, maxRows)
	}
}

// TestHandleUndoResetsAllSorts verifies that handleUndo resets sort state
// for all views, not just queue and playlists.
func TestHandleUndoResetsAllSorts(t *testing.T) {
	d := &stubQueueDequeuer{}
	m := New(context.Background(), WithQueueDequeuer(d))
	m.queue.entries = []QueueEntry{{ID: 1, Path: "/a.flac", Hash: "aaa", Display: "A"}}

	m.setUndoSlot(undoDequeueAction{
		id:    99,
		entry: QueueEntry{ID: 99, Path: "/b.flac", Hash: "bbb", Display: "B"},
		idx:   0,
	}, nil)

	m.library.SortState = sorting.SortState{Field: sorting.SortArtist, Dir: sorting.SortAsc}
	m.queue.sortState = sorting.SortState{Field: sorting.SortTitle, Dir: sorting.SortDesc}
	m.playlists.SortState = sorting.SortState{Field: sorting.SortName, Dir: sorting.SortAsc}
	m.browse.artists.sortState = sorting.SortState{Field: sorting.SortName, Dir: sorting.SortDesc}
	m.browse.albums.sortState = sorting.SortState{Field: sorting.SortName, Dir: sorting.SortAsc}
	m.favorites.SortState = sorting.SortState{Field: sorting.SortTitle, Dir: sorting.SortDesc}

	nm := m.handleUndo()

	if nm.library.SortState.Field != sorting.SortDefault {
		t.Errorf("library sort not reset: got field %d", nm.library.SortState.Field)
	}
	if nm.queue.sortState.Field != sorting.SortDefault {
		t.Errorf("queue sort not reset: got field %d", nm.queue.sortState.Field)
	}
	if nm.playlists.SortState.Field != sorting.SortDefault {
		t.Errorf("playlists sort not reset: got field %d", nm.playlists.SortState.Field)
	}
	if nm.browse.artists.sortState.Field != sorting.SortDefault {
		t.Errorf("artists sort not reset: got field %d", nm.browse.artists.sortState.Field)
	}
	if nm.browse.albums.sortState.Field != sorting.SortDefault {
		t.Errorf("albums sort not reset: got field %d", nm.browse.albums.sortState.Field)
	}
	if nm.favorites.SortState.Field != sorting.SortDefault {
		t.Errorf("favorites sort not reset: got field %d", nm.favorites.SortState.Field)
	}
}

// TestUndoExitsVisualMode verifies that performing an undo exits visual mode
// to prevent stale anchor/snapshot from referencing shifted indices.
func TestUndoExitsVisualMode(t *testing.T) {
	d := &stubQueueDequeuer{}
	m := New(context.Background(), WithQueueDequeuer(d))
	m.queue.entries = []QueueEntry{
		{ID: 10, Path: "/music/a.flac", Display: "A"},
		{ID: 20, Path: "/music/b.flac", Display: "B"},
		{ID: 30, Path: "/music/c.flac", Display: "C"},
	}

	// Dequeue entry at index 1 to set up an undo slot.
	next, _ := m.Update(DequeueTrackMsg{Index: 1})
	nm := next.(Model)

	// Enter visual mode after the dequeue (simulating user entering visual
	// mode while an undo slot is pending).
	nm.activeView = viewQueue
	nm.visual = visualState{active: true, view: viewQueue, anchor: 0}

	// Perform undo — visual mode must be exited.
	next, _ = nm.Update(UndoMsg{})
	nm = next.(Model)

	if nm.visual.active {
		t.Fatal("visual mode must be exited after undo to prevent stale anchor/snapshot")
	}
}

// TestUndoDequeueClampsCursorWithActiveFilter verifies that undoing a dequeue
// clamps the cursor against the filtered entry count when a filter is active.
func TestUndoDequeueClampsCursorWithActiveFilter(t *testing.T) {
	m := New(context.Background(), WithPlayer(&stubPlayer{}))
	m.queue.height = 20
	m.queue.entries = []QueueEntry{
		{ID: 1, Path: "/a.flac", Title: "Rock1", Artist: "Rock", Album: "rock"},
		{ID: 2, Path: "/b.flac", Title: "Pop1", Artist: "Pop", Album: "pop"},
		{ID: 3, Path: "/c.flac", Title: "Rock2", Artist: "Rock", Album: "rock"},
		{ID: 4, Path: "/d.flac", Title: "Rock3", Artist: "Rock", Album: "rock"},
	}

	m.queue.filter.Text = "rock"
	filtered := m.queue.filteredEntries()
	if len(filtered) != 3 {
		t.Fatalf("expected 3 filtered entries, got %d", len(filtered))
	}
	m.queue.cursor = 0

	dequeued := QueueEntry{ID: 99, Path: "/z.flac", Title: "Jazz1", Artist: "Jazz", Album: "jazz"}

	undoer := &stubQueueDequeuer{}
	m.undoer = undoer
	m.undo.slot = undoSlot{
		action:    undoDequeueAction{id: dequeued.ID, entry: dequeued, idx: 10},
		expiresAt: time.Now().Add(undoWindowDuration),
	}

	next, _ := m.Update(UndoMsg{})
	nm := next.(Model)

	filteredAfter := nm.queue.filteredEntries()
	if nm.queue.cursor >= len(filteredAfter) {
		t.Errorf("cursor %d out of bounds for filtered count %d", nm.queue.cursor, len(filteredAfter))
	}
}
