package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/tui/playback"
)

// TestBulkDequeueCreatesUndoSlot verifies that bulk-dequeueing selected entries
// sets up an undoKindBulkDequeue slot with the correct entries, indices, and IDs.
func TestBulkDequeueCreatesUndoSlot(t *testing.T) {
	d := &stubQueueDequeuer{}
	m := New(context.Background(), WithQueueDequeuer(d))
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{
		{ID: 10, Path: "/a.flac", Display: "A"},
		{ID: 20, Path: "/b.flac", Display: "B", Selected: true},
		{ID: 30, Path: "/c.flac", Display: "C"},
		{ID: 40, Path: "/d.flac", Display: "D", Selected: true},
	}

	next, _ := m.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	nm := next.(Model)

	if len(nm.queue.entries) != 2 {
		t.Fatalf("after bulk dequeue: expected 2 entries, got %d", len(nm.queue.entries))
	}
	if nm.queue.entries[0].ID != 10 || nm.queue.entries[1].ID != 30 {
		t.Fatalf("remaining entries should be ID=10,30; got %d,%d",
			nm.queue.entries[0].ID, nm.queue.entries[1].ID)
	}
	bulkAction, ok := nm.undo.slot.action.(undoBulkDequeueAction)
	if !ok {
		t.Fatalf("expected undoBulkDequeueAction, got %T", nm.undo.slot.action)
	}
	if len(bulkAction.ids) != 2 || bulkAction.ids[0] != 20 || bulkAction.ids[1] != 40 {
		t.Fatalf("bulkDequeueIDs = %v, want [20 40]", bulkAction.ids)
	}
	if len(bulkAction.indices) != 2 || bulkAction.indices[0] != 1 || bulkAction.indices[1] != 3 {
		t.Fatalf("bulkDequeueIndices = %v, want [1 3]", bulkAction.indices)
	}
	if !strings.Contains(nm.statusBar.topNotification().msg, "undo") {
		t.Fatalf("notification should contain undo hint, got %q", nm.statusBar.topNotification().msg)
	}
	if len(d.dequeueTrackSoftCalls) != 2 {
		t.Fatalf("DequeueTrackSoft calls = %d, want 2", len(d.dequeueTrackSoftCalls))
	}
}

// TestBulkDequeueDuplicateHashKeepsPlayback verifies that bulk-removing
// selected entries does not stop playback when another entry with the
// currently-playing hash remains.
func TestBulkDequeueDuplicateHashKeepsPlayback(t *testing.T) {
	d := &stubQueueDequeuer{}
	m := New(context.Background(), WithQueueDequeuer(d))
	m.activeView = viewQueue
	m.playback.state = playback.StatePlaying
	m.playback.currentHash = "abc123"
	m.statusBar.artist = "Artist"
	m.statusBar.title = "Title"
	m.queue.entries = []QueueEntry{
		{ID: 10, Path: "/music/a.flac", Display: "A", Hash: "abc123", Selected: true},
		{ID: 20, Path: "/music/a.flac", Display: "A (dup)", Hash: "abc123"},
		{ID: 30, Path: "/music/b.flac", Display: "B", Hash: "other"},
	}

	next, _ := m.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	nm := next.(Model)

	if nm.playback.state != playback.StatePlaying {
		t.Errorf("playbackState = %v, want PlaybackPlaying (another entry with same hash remains)", nm.playback.state)
	}
	if nm.playback.currentHash != "abc123" {
		t.Errorf("currentHash = %q, want %q", nm.playback.currentHash, "abc123")
	}
}

// TestBulkDequeueLastInstanceOfHashStopsPlayback verifies that bulk-removing
// all entries with the currently-playing hash stops playback.
func TestBulkDequeueLastInstanceOfHashStopsPlayback(t *testing.T) {
	d := &stubQueueDequeuer{}
	m := New(context.Background(), WithQueueDequeuer(d))
	m.activeView = viewQueue
	m.playback.state = playback.StatePlaying
	m.playback.currentHash = "abc123"
	m.statusBar.artist = "Artist"
	m.statusBar.title = "Title"
	m.queue.entries = []QueueEntry{
		{ID: 10, Path: "/music/a.flac", Display: "A", Hash: "abc123", Selected: true},
		{ID: 20, Path: "/music/b.flac", Display: "B", Hash: "other"},
	}

	next, _ := m.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	nm := next.(Model)

	if nm.playback.state != playback.StateIdle {
		t.Errorf("playbackState = %v, want PlaybackIdle", nm.playback.state)
	}
	if nm.playback.currentHash != "" {
		t.Errorf("currentHash = %q, want empty", nm.playback.currentHash)
	}
}

// TestBulkDequeueAdjustsCurrentIndex verifies that bulk-dequeueing selected
// entries adjusts playback.currentIndex so the now-playing highlight stays on
// the correct entry.
func TestBulkDequeueAdjustsCurrentIndex(t *testing.T) {
	d := &stubQueueDequeuer{}
	m := New(context.Background(), WithQueueDequeuer(d))
	m.activeView = viewQueue
	m.playback.currentIndex = 3
	m.playback.currentHash = "ddd"
	m.queue.entries = []QueueEntry{
		{ID: 10, Path: "/a.flac", Display: "A", Hash: "aaa", Selected: true},
		{ID: 20, Path: "/b.flac", Display: "B", Hash: "bbb"},
		{ID: 30, Path: "/c.flac", Display: "C", Hash: "ccc", Selected: true},
		{ID: 40, Path: "/d.flac", Display: "D", Hash: "ddd"},
		{ID: 50, Path: "/e.flac", Display: "E", Hash: "eee"},
	}

	next, _ := m.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	nm := next.(Model)

	if len(nm.queue.entries) != 3 {
		t.Fatalf("after bulk dequeue: expected 3 entries, got %d", len(nm.queue.entries))
	}
	if nm.playback.currentIndex != 1 {
		t.Errorf("currentIndex = %d, want 1 (adjusted after removing 2 entries before it)", nm.playback.currentIndex)
	}
}

// TestBulkDequeueCurrentTrackSelectedResetsIndex verifies that bulk-dequeueing
// the currently-playing entry sets currentIndex to -1.
func TestBulkDequeueCurrentTrackSelectedResetsIndex(t *testing.T) {
	d := &stubQueueDequeuer{}
	m := New(context.Background(), WithQueueDequeuer(d))
	m.activeView = viewQueue
	m.playback.currentIndex = 1
	m.playback.currentHash = "bbb"
	m.queue.entries = []QueueEntry{
		{ID: 10, Path: "/a.flac", Display: "A", Hash: "aaa"},
		{ID: 20, Path: "/b.flac", Display: "B", Hash: "bbb", Selected: true},
		{ID: 30, Path: "/c.flac", Display: "C", Hash: "ccc"},
	}

	next, _ := m.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	nm := next.(Model)

	if len(nm.queue.entries) != 2 {
		t.Fatalf("after bulk dequeue: expected 2 entries, got %d", len(nm.queue.entries))
	}
	if nm.playback.currentIndex != -1 {
		t.Errorf("currentIndex = %d, want -1 (currently-playing track was removed)", nm.playback.currentIndex)
	}
}
