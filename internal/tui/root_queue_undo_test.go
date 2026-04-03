package tui

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
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
	if nm.undo.slot.action != nil {
		t.Fatal("undo slot must be cleared after a successful undo")
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

// TestUndoEnqueueClampsScrollOffset verifies that undoing a single-track
// enqueue clamps scrollOffset so it does not point past the end of the
// shortened queue.
func TestUndoEnqueueClampsScrollOffset(t *testing.T) {
	d := &stubQueueDequeuer{}
	m := New(context.Background(), WithQueueDequeuer(d))
	m.queue.entries = []QueueEntry{
		{ID: 10, Path: "/a.flac", Display: "A"},
		{ID: 20, Path: "/b.flac", Display: "B"},
	}
	m.undo.slot = undoSlot{
		action:    undoEnqueueAction{id: 20, entry: m.queue.entries[1], idx: 1},
		expiresAt: time.Now().Add(undoWindowDuration),
	}
	m.queue.scrollOffset = 1
	m.queue.cursor = 1

	next, _ := m.Update(UndoMsg{})
	nm := next.(Model)

	if len(nm.queue.entries) != 1 {
		t.Fatalf("after undo enqueue: expected 1 entry, got %d", len(nm.queue.entries))
	}
	if nm.queue.scrollOffset > 0 {
		t.Fatalf("scrollOffset must be clamped to 0 after removing last entry, got %d",
			nm.queue.scrollOffset)
	}
}

// TestUndoBatchEnqueueClampsScrollOffset verifies that undoing a batch enqueue
// clamps scrollOffset so it does not point past the end of the shortened queue.
func TestUndoBatchEnqueueClampsScrollOffset(t *testing.T) {
	d := &stubQueueDequeuer{}
	be := &stubBatchEnqueuer{}
	m := New(context.Background(), WithQueueDequeuer(d), WithBatchEnqueuer(be))
	m.queue.entries = []QueueEntry{
		{ID: 10, Path: "/a.flac", Display: "A"},
		{ID: 100, Path: "/b.flac", Display: "B"},
		{ID: 101, Path: "/c.flac", Display: "C"},
	}
	m.undo.slot = undoSlot{
		action: undoBatchEnqueueAction{
			ids:          []int64{100, 101},
			count:        2,
			startIdx:     1,
			playlistName: "test",
		},
		expiresAt: time.Now().Add(undoWindowDuration),
	}
	m.queue.scrollOffset = 2
	m.queue.cursor = 2

	next, _ := m.Update(UndoMsg{})
	nm := next.(Model)

	if len(nm.queue.entries) != 1 {
		t.Fatalf("after undo batch enqueue: expected 1 entry, got %d", len(nm.queue.entries))
	}
	if nm.queue.scrollOffset > 0 {
		t.Fatalf("scrollOffset must be clamped to 0 after batch undo, got %d",
			nm.queue.scrollOffset)
	}
}

// TestUndoEnqueueSchedulesPurge verifies that undoing a single-track enqueue
// schedules a purge that eventually fires.
func TestUndoEnqueueSchedulesPurge(t *testing.T) {
	d := &stubQueueDequeuer{}
	m := New(context.Background(), WithQueueDequeuer(d))
	m.queue.entries = []QueueEntry{
		{ID: 10, Path: "/a.flac", Display: "A"},
		{ID: 20, Path: "/b.flac", Display: "B"},
	}
	m.undo.slot = undoSlot{
		action:    undoEnqueueAction{id: 20, entry: m.queue.entries[1], idx: 1},
		expiresAt: time.Now().Add(undoWindowDuration),
	}

	next, _ := m.Update(UndoMsg{})
	nm := next.(Model)

	if len(d.dequeueTrackSoftCalls) != 1 || d.dequeueTrackSoftCalls[0] != 20 {
		t.Fatalf("undo enqueue must soft-delete the entry at the stored index (ID=20); got calls=%v",
			d.dequeueTrackSoftCalls)
	}
	if nm.undo.slot.action != nil {
		t.Fatal("undo slot must be cleared after successful undo")
	}
	if nm.undo.slot.cancelPurge == nil {
		t.Fatal("cancelPurge must be preserved after undo so the purge timer can be cancelled by a future action")
	}
	select {
	case <-nm.undo.slot.cancelPurge:
		t.Fatal("cancelPurge must not be closed — the purge timer should still be able to fire")
	default:
	}
}

// TestUndoBatchEnqueueSchedulesPurge verifies that undoing a batch enqueue
// schedules a purge that is not immediately cancelled.
func TestUndoBatchEnqueueSchedulesPurge(t *testing.T) {
	d := &stubQueueDequeuer{}
	be := &stubBatchEnqueuer{}
	m := New(context.Background(), WithQueueDequeuer(d), WithBatchEnqueuer(be))
	m.queue.entries = []QueueEntry{
		{ID: 100, Path: "/a.flac", Display: "A"},
		{ID: 101, Path: "/b.flac", Display: "B"},
	}
	m.undo.slot = undoSlot{
		action: undoBatchEnqueueAction{
			ids:      []int64{100, 101},
			count:    2,
			startIdx: 0,
		},
		expiresAt: time.Now().Add(undoWindowDuration),
	}

	next, _ := m.Update(UndoMsg{})
	nm := next.(Model)

	if len(be.dequeueTracksSoftIDs) != 1 {
		t.Fatalf("undo batch enqueue must call DequeueTracksSoft once, got %d calls",
			len(be.dequeueTracksSoftIDs))
	}
	if nm.undo.slot.cancelPurge == nil {
		t.Fatal("cancelPurge must be preserved after undo so the purge timer can be cancelled by a future action")
	}
	select {
	case <-nm.undo.slot.cancelPurge:
		t.Fatal("cancelPurge must not be closed — the purge timer should still be able to fire")
	default:
	}
	if nm.undo.slot.action != nil {
		t.Fatal("undo slot must be cleared after successful undo")
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

// TestUndoEnqueueFindsEntryByIDAfterReorder verifies that undoing an enqueue
// removes the correct entry even if the queue has been reordered since enqueue.
func TestUndoEnqueueFindsEntryByIDAfterReorder(t *testing.T) {
	d := &stubQueueDequeuer{}
	m := New(context.Background(), WithQueueDequeuer(d))
	m.queue.entries = []QueueEntry{
		{ID: 10, Path: "/a.flac", Hash: "aaa", Display: "A"},
		{ID: 20, Path: "/b.flac", Hash: "bbb", Display: "B"},
		{ID: 30, Path: "/c.flac", Hash: "ccc", Display: "C"},
	}

	m.setUndoSlot(undoEnqueueAction{id: 30, entry: m.queue.entries[2], idx: 2}, nil)
	m.queue.entries = []QueueEntry{
		{ID: 30, Path: "/c.flac", Hash: "ccc", Display: "C"},
		{ID: 10, Path: "/a.flac", Hash: "aaa", Display: "A"},
		{ID: 20, Path: "/b.flac", Hash: "bbb", Display: "B"},
	}

	nm := m.handleUndo()

	if len(nm.queue.entries) != 2 {
		t.Fatalf("expected 2 entries after undo, got %d", len(nm.queue.entries))
	}
	for _, e := range nm.queue.entries {
		if e.ID == 30 {
			t.Fatal("ID=30 should have been removed by undo enqueue")
		}
	}
	if len(d.dequeueTrackSoftCalls) != 1 || d.dequeueTrackSoftCalls[0] != 30 {
		t.Fatalf("expected soft-delete of ID=30, got %v", d.dequeueTrackSoftCalls)
	}
}

// TestUndoBatchEnqueueRemovesByIDNotPosition verifies that undoing a batch
// enqueue removes entries by their database IDs, not by positional index range.
func TestUndoBatchEnqueueRemovesByIDNotPosition(t *testing.T) {
	be := &stubBatchEnqueuer{}
	d := &stubQueueDequeuer{}
	m := New(context.Background(), WithBatchEnqueuer(be), WithQueueDequeuer(d))
	m.queue.entries = []QueueEntry{
		{ID: 1, Path: "/old.flac", Hash: "old", Display: "Old"},
		{ID: 100, Path: "/x.flac", Hash: "xxx", Display: "X"},
		{ID: 101, Path: "/y.flac", Hash: "yyy", Display: "Y"},
	}

	m.setUndoSlot(undoBatchEnqueueAction{
		ids:          []int64{100, 101},
		count:        2,
		startIdx:     1,
		playlistName: "Test",
	}, nil)
	m.queue.entries = []QueueEntry{
		{ID: 101, Path: "/y.flac", Hash: "yyy", Display: "Y"},
		{ID: 1, Path: "/old.flac", Hash: "old", Display: "Old"},
		{ID: 100, Path: "/x.flac", Hash: "xxx", Display: "X"},
	}

	nm := m.handleUndo()

	if len(nm.queue.entries) != 1 {
		t.Fatalf("expected 1 entry after batch undo, got %d", len(nm.queue.entries))
	}
	if nm.queue.entries[0].ID != 1 {
		t.Fatalf("expected remaining entry to be ID=1, got ID=%d", nm.queue.entries[0].ID)
	}
	if len(be.dequeueTracksSoftIDs) != 1 {
		t.Fatalf("expected 1 batch soft-delete call, got %d", len(be.dequeueTracksSoftIDs))
	}
}

// TestUndoDequeueShowsNotification verifies that undoing a dequeue sets a
// confirmation notification in the status bar.
func TestUndoDequeueShowsNotification(t *testing.T) {
	d := &stubQueueDequeuer{}
	m := New(context.Background(), WithQueueDequeuer(d))
	m.queue.entries = []QueueEntry{
		{ID: 10, Path: "/a.flac", Display: "A"},
		{ID: 20, Path: "/b.flac", Display: "B"},
	}
	next, _ := m.Update(DequeueTrackMsg{Index: 0})
	nm := next.(Model)
	next, _ = nm.Update(UndoMsg{})
	nm = next.(Model)

	if nm.statusBar.notification.msg == "" {
		t.Fatal("undo dequeue must set a status bar notification")
	}
	if !strings.Contains(nm.statusBar.notification.msg, "Undone") {
		t.Errorf("expected undo notification containing 'Undone', got %q", nm.statusBar.notification.msg)
	}
}

// TestUndoEnqueueShowsNotification verifies that undoing an enqueue sets a
// confirmation notification in the status bar.
func TestUndoEnqueueShowsNotification(t *testing.T) {
	d := &stubQueueDequeuer{}
	m := New(context.Background(), WithQueueDequeuer(d))
	m.queue.entries = []QueueEntry{{ID: 10, Path: "/a.flac", Hash: "aaa", Display: "Artist – Song"}}
	m.setUndoSlot(undoEnqueueAction{id: 10, entry: m.queue.entries[0], idx: 0}, nil)

	nm := m.handleUndo()

	if nm.statusBar.notification.msg == "" {
		t.Fatal("undo enqueue must set a status bar notification")
	}
	if !strings.Contains(nm.statusBar.notification.msg, "Undone") {
		t.Errorf("expected undo notification containing 'Undone', got %q", nm.statusBar.notification.msg)
	}
}

// TestUndoClearQueueShowsNotification verifies that undoing a queue clear sets
// a confirmation notification in the status bar.
func TestUndoClearQueueShowsNotification(t *testing.T) {
	d := &stubQueueDequeuer{}
	m := New(context.Background(), WithQueueDequeuer(d))
	m.queue.entries = []QueueEntry{
		{ID: 10, Path: "/a.flac", Display: "A"},
		{ID: 20, Path: "/b.flac", Display: "B"},
	}

	savedEntries := make([]QueueEntry, len(m.queue.entries))
	copy(savedEntries, m.queue.entries)

	m.setUndoSlot(undoClearAction{ids: []int64{10, 20}, entries: savedEntries}, nil)
	m.queue.entries = nil

	nm := m.handleUndo()

	if nm.statusBar.notification.msg == "" {
		t.Fatal("undo clear must set a status bar notification")
	}
	if !strings.Contains(nm.statusBar.notification.msg, "Undone") {
		t.Errorf("expected undo notification containing 'Undone', got %q", nm.statusBar.notification.msg)
	}
	if !strings.Contains(nm.statusBar.notification.msg, "2") {
		t.Errorf("expected undo notification to mention track count, got %q", nm.statusBar.notification.msg)
	}
}

// TestUndoBulkDequeueShowsNotification verifies that undoing a bulk dequeue
// sets a confirmation notification in the status bar.
func TestUndoBulkDequeueShowsNotification(t *testing.T) {
	d := &stubQueueDequeuer{}
	m := New(context.Background(), WithQueueDequeuer(d))

	removed := []QueueEntry{
		{ID: 10, Path: "/a.flac", Display: "A"},
		{ID: 20, Path: "/b.flac", Display: "B"},
	}
	m.setUndoSlot(undoBulkDequeueAction{entries: removed, indices: []int{0, 1}, ids: []int64{10, 20}}, nil)

	nm := m.handleUndo()

	if nm.statusBar.notification.msg == "" {
		t.Fatal("undo bulk dequeue must set a status bar notification")
	}
	if !strings.Contains(nm.statusBar.notification.msg, "Undone") {
		t.Errorf("expected undo notification containing 'Undone', got %q", nm.statusBar.notification.msg)
	}
}

// TestUndoBatchEnqueueShowsNotification verifies that undoing a batch enqueue
// sets a confirmation notification in the status bar.
func TestUndoBatchEnqueueShowsNotification(t *testing.T) {
	be := &stubBatchEnqueuer{}
	d := &stubQueueDequeuer{}
	m := New(context.Background(), WithBatchEnqueuer(be), WithQueueDequeuer(d))
	m.queue.entries = []QueueEntry{
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

	if nm.statusBar.notification.msg == "" {
		t.Fatal("undo batch enqueue must set a status bar notification")
	}
	if !strings.Contains(nm.statusBar.notification.msg, "Undone") {
		t.Errorf("expected undo notification containing 'Undone', got %q", nm.statusBar.notification.msg)
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

	m.library.sortState = SortState{Field: SortArtist, Dir: SortAsc}
	m.queue.sortState = SortState{Field: SortTitle, Dir: SortDesc}
	m.playlists.sortState = SortState{Field: SortName, Dir: SortAsc}
	m.artists.sortState = SortState{Field: SortName, Dir: SortDesc}
	m.albums.sortState = SortState{Field: SortName, Dir: SortAsc}
	m.favorites.sortState = SortState{Field: SortTitle, Dir: SortDesc}

	nm := m.handleUndo()

	if nm.library.sortState.Field != SortDefault {
		t.Errorf("library sort not reset: got field %d", nm.library.sortState.Field)
	}
	if nm.queue.sortState.Field != SortDefault {
		t.Errorf("queue sort not reset: got field %d", nm.queue.sortState.Field)
	}
	if nm.playlists.sortState.Field != SortDefault {
		t.Errorf("playlists sort not reset: got field %d", nm.playlists.sortState.Field)
	}
	if nm.artists.sortState.Field != SortDefault {
		t.Errorf("artists sort not reset: got field %d", nm.artists.sortState.Field)
	}
	if nm.albums.sortState.Field != SortDefault {
		t.Errorf("albums sort not reset: got field %d", nm.albums.sortState.Field)
	}
	if nm.favorites.sortState.Field != SortDefault {
		t.Errorf("favorites sort not reset: got field %d", nm.favorites.sortState.Field)
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

// TestUndoEnqueueResetsPlaybackWhenPlayingTrackRemoved verifies that undoing
// an enqueue stops playback when the removed track was the currently-playing track.
func TestUndoEnqueueResetsPlaybackWhenPlayingTrackRemoved(t *testing.T) {
	d := &stubQueueDequeuer{}
	p := &stubPlayerQueueRemover{}
	m := New(context.Background(), WithQueueDequeuer(d), WithPlayer(p))
	m.queue.entries = []QueueEntry{{ID: 42, Path: "/song.flac", Hash: "abc123", Display: "Artist – Song"}}
	m.playback.state = PlaybackPlaying
	m.playback.currentHash = "abc123"
	m.statusBar.artist = "Artist"
	m.statusBar.title = "Song"

	m.setUndoSlot(undoEnqueueAction{id: 42, entry: m.queue.entries[0], idx: 0}, nil)

	nm := m.handleUndo()

	if len(nm.queue.entries) != 0 {
		t.Fatalf("expected empty queue after undo enqueue, got %d entries", len(nm.queue.entries))
	}
	if nm.playback.state != PlaybackIdle {
		t.Errorf("expected PlaybackIdle after undo enqueue removed playing track, got %v", nm.playback.state)
	}
	if nm.playback.currentHash != "" {
		t.Errorf("expected empty currentHash after undo enqueue, got %q", nm.playback.currentHash)
	}
	if nm.statusBar.artist != "" {
		t.Errorf("expected empty status bar artist after undo enqueue, got %q", nm.statusBar.artist)
	}
	if nm.statusBar.title != "" {
		t.Errorf("expected empty status bar title after undo enqueue, got %q", nm.statusBar.title)
	}
}

// TestUndoEnqueueKeepsPlaybackWhenOtherTrackPlaying verifies that undoing an
// enqueue does NOT stop playback when a different track is currently playing.
func TestUndoEnqueueKeepsPlaybackWhenOtherTrackPlaying(t *testing.T) {
	d := &stubQueueDequeuer{}
	p := &stubPlayerQueueRemover{}
	m := New(context.Background(), WithQueueDequeuer(d), WithPlayer(p))
	m.queue.entries = []QueueEntry{
		{ID: 1, Path: "/a.flac", Hash: "aaa", Display: "A"},
		{ID: 2, Path: "/b.flac", Hash: "bbb", Display: "B"},
	}
	m.playback.state = PlaybackPlaying
	m.playback.currentHash = "aaa"
	m.statusBar.artist = "A"
	m.statusBar.title = "Track A"

	m.setUndoSlot(undoEnqueueAction{id: 2, entry: m.queue.entries[1], idx: 1}, nil)

	nm := m.handleUndo()

	if len(nm.queue.entries) != 1 {
		t.Fatalf("expected 1 entry after undo enqueue, got %d", len(nm.queue.entries))
	}
	if nm.playback.state != PlaybackPlaying {
		t.Errorf("playback should remain Playing when a non-playing track is removed, got %v", nm.playback.state)
	}
	if nm.statusBar.artist != "A" {
		t.Errorf("status bar artist should be unchanged, got %q", nm.statusBar.artist)
	}
}

// TestUndoBatchEnqueueResetsPlaybackWhenQueueEmptied verifies that undoing a
// batch enqueue stops playback when the removal empties the queue entirely.
func TestUndoBatchEnqueueResetsPlaybackWhenQueueEmptied(t *testing.T) {
	be := &stubBatchEnqueuer{}
	d := &stubQueueDequeuer{}
	p := &stubPlayerQueueRemover{}
	m := New(context.Background(), WithBatchEnqueuer(be), WithQueueDequeuer(d), WithPlayer(p))
	m.queue.entries = []QueueEntry{
		{ID: 100, Path: "/x.flac", Hash: "xxx", Display: "X"},
		{ID: 101, Path: "/y.flac", Hash: "yyy", Display: "Y"},
	}
	m.playback.state = PlaybackPlaying
	m.playback.currentHash = "xxx"
	m.statusBar.artist = "X-Artist"
	m.statusBar.title = "X-Title"

	m.setUndoSlot(undoBatchEnqueueAction{
		ids:          []int64{100, 101},
		count:        2,
		startIdx:     0,
		playlistName: "TestPL",
	}, nil)

	nm := m.handleUndo()

	if len(nm.queue.entries) != 0 {
		t.Fatalf("expected empty queue after undo batch enqueue, got %d", len(nm.queue.entries))
	}
	if nm.playback.state != PlaybackIdle {
		t.Errorf("expected PlaybackIdle after undo batch enqueue emptied queue, got %v", nm.playback.state)
	}
	if nm.playback.currentHash != "" {
		t.Errorf("expected empty currentHash after undo batch enqueue, got %q", nm.playback.currentHash)
	}
	if nm.statusBar.artist != "" {
		t.Errorf("expected empty status bar artist, got %q", nm.statusBar.artist)
	}
}

// TestUndoBatchEnqueueKeepsPlaybackWhenPlayingTrackSurvives verifies that
// undoing a batch enqueue does NOT stop playback when the currently-playing
// track was not part of the batch and remains in the queue.
func TestUndoBatchEnqueueKeepsPlaybackWhenPlayingTrackSurvives(t *testing.T) {
	be := &stubBatchEnqueuer{}
	d := &stubQueueDequeuer{}
	m := New(context.Background(), WithBatchEnqueuer(be), WithQueueDequeuer(d))
	m.queue.entries = []QueueEntry{
		{ID: 1, Path: "/keeper.flac", Hash: "keep", Display: "Keeper"},
		{ID: 100, Path: "/x.flac", Hash: "xxx", Display: "X"},
		{ID: 101, Path: "/y.flac", Hash: "yyy", Display: "Y"},
	}
	m.playback.state = PlaybackPlaying
	m.playback.currentHash = "keep"
	m.statusBar.artist = "Keeper-Artist"
	m.statusBar.title = "Keeper-Title"

	m.setUndoSlot(undoBatchEnqueueAction{
		ids:          []int64{100, 101},
		count:        2,
		startIdx:     1,
		playlistName: "TestPL",
	}, nil)

	nm := m.handleUndo()

	if len(nm.queue.entries) != 1 {
		t.Fatalf("expected 1 entry after undo, got %d", len(nm.queue.entries))
	}
	if nm.playback.state != PlaybackPlaying {
		t.Errorf("playback should remain Playing when playing track survives, got %v", nm.playback.state)
	}
	if nm.statusBar.artist != "Keeper-Artist" {
		t.Errorf("status bar artist should be unchanged, got %q", nm.statusBar.artist)
	}
}
