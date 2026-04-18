package tui

import (
	"context"
	"testing"
)

// TestShuffleClearsSelectionsAndVisualMode verifies that shuffling the queue
// clears all bulk selections and exits visual mode so stale index-based state
// does not survive the reorder.
func TestShuffleClearsSelectionsAndVisualMode(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{
		{ID: 1, Hash: "a", Display: "A", Selected: true},
		{ID: 2, Hash: "b", Display: "B"},
		{ID: 3, Hash: "c", Display: "C", Selected: true},
	}
	m.visual = visualState{
		active: true,
		view:   viewQueue,
		anchor: 0,
	}

	m, _ = m.handleQueueShuffleMsg()

	for i, e := range m.queue.entries {
		if e.Selected {
			t.Errorf("entry %d (%s) should not be selected after shuffle", i, e.Display)
		}
	}
	if m.visual.active {
		t.Error("visual mode should be inactive after shuffle")
	}
}

// TestDedupClearsSelectionsAndVisualMode verifies that removing duplicates
// clears all bulk selections and exits visual mode.
func TestDedupClearsSelectionsAndVisualMode(t *testing.T) {
	d := &stubQueueDequeuer{}
	m := New(context.Background(), WithQueueDequeuer(d))
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{
		{ID: 1, Hash: "a", Display: "A", Selected: true},
		{ID: 2, Hash: "b", Display: "B"},
		{ID: 3, Hash: "a", Display: "A2"}, // duplicate of hash "a"
	}
	m.visual = visualState{
		active: true,
		view:   viewQueue,
		anchor: 0,
	}

	m, _ = m.handleQueueDedupMsg()

	if len(m.queue.entries) != 2 {
		t.Fatalf("expected 2 entries after dedup, got %d", len(m.queue.entries))
	}
	for i, e := range m.queue.entries {
		if e.Selected {
			t.Errorf("entry %d (%s) should not be selected after dedup", i, e.Display)
		}
	}
	if m.visual.active {
		t.Error("visual mode should be inactive after dedup")
	}
}

// TestShufflePreservesPlaybackIndex verifies that the current-playing track
// index is correctly updated after a shuffle.
func TestShufflePreservesPlaybackIndex(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{
		{ID: 1, Hash: "a", Display: "A"},
		{ID: 2, Hash: "b", Display: "B"},
		{ID: 3, Hash: "c", Display: "C"},
	}
	m.playback.currentHash = "b"
	m.playback.currentIndex = 1

	m, _ = m.handleQueueShuffleMsg()

	// After shuffle, currentIndex should point to the entry with hash "b".
	if m.playback.currentIndex < 0 || m.playback.currentIndex >= len(m.queue.entries) {
		t.Fatalf("currentIndex out of bounds: %d", m.playback.currentIndex)
	}
	if m.queue.entries[m.playback.currentIndex].Hash != "b" {
		t.Errorf("currentIndex points to hash %q, want \"b\"",
			m.queue.entries[m.playback.currentIndex].Hash)
	}
}

// TestDedupRemovesDuplicatesKeepsFirst verifies that dedup keeps the first
// occurrence and removes subsequent duplicates.
func TestDedupRemovesDuplicatesKeepsFirst(t *testing.T) {
	d := &stubQueueDequeuer{}
	m := New(context.Background(), WithQueueDequeuer(d))
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{
		{ID: 1, Hash: "x", Display: "X1"},
		{ID: 2, Hash: "y", Display: "Y"},
		{ID: 3, Hash: "x", Display: "X2"},
		{ID: 4, Hash: "x", Display: "X3"},
	}

	m, _ = m.handleQueueDedupMsg()

	if len(m.queue.entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(m.queue.entries))
	}
	if m.queue.entries[0].ID != 1 {
		t.Errorf("first entry should be ID=1 (first occurrence), got %d", m.queue.entries[0].ID)
	}
	if m.queue.entries[1].ID != 2 {
		t.Errorf("second entry should be ID=2, got %d", m.queue.entries[1].ID)
	}
}
