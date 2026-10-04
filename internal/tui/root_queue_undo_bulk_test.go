package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/tui/queue"
)

// TestUndoBulkDequeueRestoresEntries verifies that undoing a bulk dequeue
// restores all removed entries at their original indices in one transaction.
func TestUndoBulkDequeueRestoresEntries(t *testing.T) {
	d := &stubQueueDequeuer{}
	ps := &stubPlayerQueueSyncer{}
	m := New(context.Background(), WithPlayer(ps), WithQueueDequeuer(d))
	m.ActiveView = viewQueue
	m.queue.Entries = []queue.Entry{
		{ID: 10, Path: "/a.flac", Display: "A"},
		{ID: 20, Path: "/b.flac", Display: "B", Selected: true},
		{ID: 30, Path: "/c.flac", Display: "C"},
		{ID: 40, Path: "/d.flac", Display: "D", Selected: true},
		{ID: 50, Path: "/e.flac", Display: "E"},
	}

	next, _ := m.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	nm := next.(Model)
	if len(nm.queue.Entries) != 3 {
		t.Fatalf("after bulk dequeue: expected 3 entries, got %d", len(nm.queue.Entries))
	}

	next, _ = nm.Update(UndoMsg{})
	nm = next.(Model)

	if len(nm.queue.Entries) != 5 {
		t.Fatalf("after undo: expected 5 entries, got %d", len(nm.queue.Entries))
	}
	wantIDs := []int64{10, 20, 30, 40, 50}
	for i, want := range wantIDs {
		if nm.queue.Entries[i].ID != want {
			t.Errorf("entry[%d].ID = %d, want %d", i, nm.queue.Entries[i].ID, want)
		}
	}
	if len(d.undoClearQueueCalls) != 1 {
		t.Fatalf("UndoClearQueue calls = %d, want 1", len(d.undoClearQueueCalls))
	}
	if ids := d.undoClearQueueCalls[0]; len(ids) != 2 || ids[0] != 20 || ids[1] != 40 {
		t.Fatalf("UndoClearQueue IDs = %v, want [20 40]", ids)
	}
	if len(ps.setQueueCalls) == 0 {
		t.Fatal("expected SetQueue to be called after undo")
	}
	if nm.undo.slot.action != nil {
		t.Fatal("undo slot must be cleared after successful bulk dequeue undo")
	}
}

// TestBulkDequeueSyncsPlayerIndexAfterAdjustment verifies that the player's
// current index is set to the adjusted value (not the stale pre-removal value)
// after bulk-dequeuing entries before the currently playing track.
func TestBulkDequeueSyncsPlayerIndexAfterAdjustment(t *testing.T) {
	p := &stubPlayerQueueSyncer{}
	d := &stubQueueDequeuer{}
	m := New(context.Background(), WithPlayer(p), WithQueueDequeuer(d))
	m.ActiveView = viewQueue
	m.queue.Entries = []queue.Entry{
		{ID: 10, Path: "/a.flac", Hash: "ha", Display: "A", Selected: true},
		{ID: 20, Path: "/b.flac", Hash: "hb", Display: "B", Selected: true},
		{ID: 30, Path: "/c.flac", Hash: "hc", Display: "C"},
		{ID: 40, Path: "/d.flac", Hash: "hd", Display: "D"},
	}
	m.playback.currentIndex = 3
	m.playback.currentHash = "hd"

	next, _ := m.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	nm := next.(Model)

	// After removing indices 0 and 1, the track at index 3 should now be at index 1.
	if nm.playback.currentIndex != 1 {
		t.Fatalf("after bulk dequeue: expected currentIndex 1, got %d", nm.playback.currentIndex)
	}
	// Verify the player received the adjusted index (1), not the stale index (3).
	if len(p.setCurrentIndexCalls) == 0 {
		t.Fatal("SetCurrentIndex must be called after bulk dequeue")
	}
	lastIdx := p.setCurrentIndexCalls[len(p.setCurrentIndexCalls)-1]
	if lastIdx != 1 {
		t.Fatalf("SetCurrentIndex: want 1, got %d", lastIdx)
	}
}

// TestUndoBulkDequeueExpiredIsNoOp verifies that undoing a bulk dequeue after
// the undo window has expired shows "Nothing to undo" and does not restore entries.
func TestUndoBulkDequeueExpiredIsNoOp(t *testing.T) {
	d := &stubQueueDequeuer{}
	m := New(context.Background(), WithQueueDequeuer(d))
	m.ActiveView = viewQueue
	m.queue.Entries = []queue.Entry{
		{ID: 10, Path: "/a.flac", Display: "A"},
		{ID: 20, Path: "/b.flac", Display: "B", Selected: true},
	}

	next, _ := m.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	nm := next.(Model)
	nm.undo.slot.expiresAt = time.Now().Add(-1 * time.Second)

	next, _ = nm.Update(UndoMsg{})
	nm = next.(Model)

	if len(nm.queue.Entries) != 1 {
		t.Fatalf("after expired undo: expected 1 entry, got %d", len(nm.queue.Entries))
	}
	if len(d.undoDequeueCallIDs) != 0 {
		t.Fatalf("UndoDequeue must not be called after expiry, got %v", d.undoDequeueCallIDs)
	}
	if !strings.Contains(nm.statusBar.TopNotification().Msg, "Nothing to undo") {
		t.Fatalf("expected 'Nothing to undo' notification, got %q", nm.statusBar.TopNotification().Msg)
	}
}
