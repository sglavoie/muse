package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/tui/queue"
)

// TestShuffleClearsSelectionsAndVisualMode verifies that shuffling the queue
// clears all bulk selections and exits visual mode so stale index-based state
// does not survive the reorder.
func TestShuffleClearsSelectionsAndVisualMode(t *testing.T) {
	m := New(context.Background())
	m.ActiveView = viewQueue
	m.queue.Entries = []queue.Entry{
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

	for i, e := range m.queue.Entries {
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
	m.ActiveView = viewQueue
	m.queue.Entries = []queue.Entry{
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

	if len(m.queue.Entries) != 2 {
		t.Fatalf("expected 2 entries after dedup, got %d", len(m.queue.Entries))
	}
	for i, e := range m.queue.Entries {
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
	m.ActiveView = viewQueue
	m.queue.Entries = []queue.Entry{
		{ID: 1, Hash: "a", Display: "A"},
		{ID: 2, Hash: "b", Display: "B"},
		{ID: 3, Hash: "c", Display: "C"},
	}
	m.playback.currentHash = "b"
	m.playback.currentIndex = 1

	m, _ = m.handleQueueShuffleMsg()

	// After shuffle, currentIndex should point to the entry with hash "b".
	if m.playback.currentIndex < 0 || m.playback.currentIndex >= len(m.queue.Entries) {
		t.Fatalf("currentIndex out of bounds: %d", m.playback.currentIndex)
	}
	if m.queue.Entries[m.playback.currentIndex].Hash != "b" {
		t.Errorf("currentIndex points to hash %q, want \"b\"",
			m.queue.Entries[m.playback.currentIndex].Hash)
	}
}

// TestShufflePreservesPlaybackIndexDuplicateHashes verifies that when the
// queue contains duplicate hashes, shuffle relocates the playback index to
// the exact entry (by database ID) that was playing, not just the first
// entry with the same hash.
func TestShufflePreservesPlaybackIndexDuplicateHashes(t *testing.T) {
	m := New(context.Background())
	m.ActiveView = viewQueue
	m.queue.Entries = []queue.Entry{
		{ID: 1, Hash: "a", Display: "A1"},
		{ID: 2, Hash: "a", Display: "A2"},
		{ID: 3, Hash: "a", Display: "A3"},
	}
	// Playback is on the second copy (ID=2).
	m.playback.currentHash = "a"
	m.playback.currentIndex = 1

	m, _ = m.handleQueueShuffleMsg()

	if m.playback.currentIndex < 0 || m.playback.currentIndex >= len(m.queue.Entries) {
		t.Fatalf("currentIndex out of bounds: %d", m.playback.currentIndex)
	}
	got := m.queue.Entries[m.playback.currentIndex]
	if got.ID != 2 {
		t.Errorf("currentIndex points to entry ID=%d, want ID=2 (the exact playing entry)", got.ID)
	}
}

// TestDedupRelocatesPlaybackIndex verifies that after dedup the playback
// index points to the surviving first copy of the currently-playing track,
// even when playback was on a later duplicate that got removed.
func TestDedupRelocatesPlaybackIndex(t *testing.T) {
	d := &stubQueueDequeuer{}
	m := New(context.Background(), WithQueueDequeuer(d))
	m.ActiveView = viewQueue
	m.queue.Entries = []queue.Entry{
		{ID: 1, Hash: "a", Display: "A1"},
		{ID: 2, Hash: "b", Display: "B"},
		{ID: 3, Hash: "a", Display: "A2"}, // duplicate — currently playing
	}
	m.playback.currentHash = "a"
	m.playback.currentIndex = 2 // playing the duplicate

	m, _ = m.handleQueueDedupMsg()

	if len(m.queue.Entries) != 2 {
		t.Fatalf("expected 2 entries after dedup, got %d", len(m.queue.Entries))
	}
	// currentIndex must now point to the surviving first copy (index 0).
	if m.playback.currentIndex != 0 {
		t.Errorf("currentIndex = %d, want 0 (surviving first copy)", m.playback.currentIndex)
	}
	if m.queue.Entries[m.playback.currentIndex].Hash != "a" {
		t.Errorf("entry at currentIndex has hash %q, want \"a\"",
			m.queue.Entries[m.playback.currentIndex].Hash)
	}
}

// TestDedupPreservesPlaybackIndexNonDuplicate verifies that when the
// currently-playing entry is NOT a duplicate, its index is correctly
// adjusted after earlier entries are removed by dedup.
func TestDedupPreservesPlaybackIndexNonDuplicate(t *testing.T) {
	d := &stubQueueDequeuer{}
	m := New(context.Background(), WithQueueDequeuer(d))
	m.ActiveView = viewQueue
	m.queue.Entries = []queue.Entry{
		{ID: 1, Hash: "a", Display: "A1"},
		{ID: 2, Hash: "a", Display: "A2"}, // duplicate, removed
		{ID: 3, Hash: "b", Display: "B"},  // currently playing
	}
	m.playback.currentHash = "b"
	m.playback.currentIndex = 2

	m, _ = m.handleQueueDedupMsg()

	if len(m.queue.Entries) != 2 {
		t.Fatalf("expected 2 entries after dedup, got %d", len(m.queue.Entries))
	}
	// "b" was at index 2, one duplicate before it was removed, so new index = 1.
	if m.playback.currentIndex != 1 {
		t.Errorf("currentIndex = %d, want 1", m.playback.currentIndex)
	}
	if m.queue.Entries[m.playback.currentIndex].Hash != "b" {
		t.Errorf("entry at currentIndex has hash %q, want \"b\"",
			m.queue.Entries[m.playback.currentIndex].Hash)
	}
}

// TestShuffleSyncsPlayerCurrentIndex verifies that the player's current index
// is updated after a shuffle so auto-advance uses the correct position.
func TestShuffleSyncsPlayerCurrentIndex(t *testing.T) {
	ps := &stubPlayerQueueSyncer{}
	m := New(context.Background(), WithPlayer(ps))
	m.ActiveView = viewQueue
	m.queue.Entries = []queue.Entry{
		{ID: 1, Hash: "a", Display: "A"},
		{ID: 2, Hash: "b", Display: "B"},
		{ID: 3, Hash: "c", Display: "C"},
	}
	m.playback.currentHash = "b"
	m.playback.currentIndex = 1

	m, _ = m.handleQueueShuffleMsg()

	if len(ps.setCurrentIndexCalls) == 0 {
		t.Fatal("SetCurrentIndex was never called after shuffle")
	}
	lastIdx := ps.setCurrentIndexCalls[len(ps.setCurrentIndexCalls)-1]
	if lastIdx != m.playback.currentIndex {
		t.Errorf("player index = %d, TUI index = %d; they should match after shuffle",
			lastIdx, m.playback.currentIndex)
	}
}

// TestDedupSyncsPlayerCurrentIndex verifies that the player's current index
// is updated after dedup so auto-advance uses the correct position.
func TestDedupSyncsPlayerCurrentIndex(t *testing.T) {
	ps := &stubPlayerQueueSyncer{}
	d := &stubQueueDequeuer{}
	m := New(context.Background(), WithPlayer(ps), WithQueueDequeuer(d))
	m.ActiveView = viewQueue
	m.queue.Entries = []queue.Entry{
		{ID: 1, Hash: "a", Display: "A1"},
		{ID: 2, Hash: "b", Display: "B"},
		{ID: 3, Hash: "a", Display: "A2"}, // duplicate
	}
	m.playback.currentHash = "b"
	m.playback.currentIndex = 1

	m, _ = m.handleQueueDedupMsg()

	if len(ps.setCurrentIndexCalls) == 0 {
		t.Fatal("SetCurrentIndex was never called after dedup")
	}
	lastIdx := ps.setCurrentIndexCalls[len(ps.setCurrentIndexCalls)-1]
	if lastIdx != m.playback.currentIndex {
		t.Errorf("player index = %d, TUI index = %d; they should match after dedup",
			lastIdx, m.playback.currentIndex)
	}
}

// TestDedupSyncsPlayerCurrentIndexWhenCurrentDuplicate verifies that dedup
// updates the player after relocating playback from a removed duplicate to the
// surviving first copy.
func TestDedupSyncsPlayerCurrentIndexWhenCurrentDuplicate(t *testing.T) {
	ps := &stubPlayerQueueSyncer{}
	d := &stubQueueDequeuer{}
	m := New(context.Background(), WithPlayer(ps), WithQueueDequeuer(d))
	m.ActiveView = viewQueue
	m.queue.Entries = []queue.Entry{
		{ID: 1, Hash: "a", Display: "A1"},
		{ID: 2, Hash: "b", Display: "B"},
		{ID: 3, Hash: "a", Display: "A2"}, // duplicate currently playing
	}
	m.playback.currentHash = "a"
	m.playback.currentIndex = 2

	m, _ = m.handleQueueDedupMsg()

	if m.playback.currentIndex != 0 {
		t.Fatalf("currentIndex = %d, want 0 after dedup relocates playback", m.playback.currentIndex)
	}
	if len(ps.setCurrentIndexCalls) == 0 {
		t.Fatal("SetCurrentIndex was never called after dedup")
	}
	lastIdx := ps.setCurrentIndexCalls[len(ps.setCurrentIndexCalls)-1]
	if lastIdx != 0 {
		t.Errorf("player index = %d, want 0 after dedup relocates playback", lastIdx)
	}
}

// TestMoveTrackSyncsPlayerCurrentIndex verifies that the player's current index
// is updated after moving a track up/down.
func TestMoveTrackSyncsPlayerCurrentIndex(t *testing.T) {
	ps := &stubPlayerQueueSyncer{}
	r := &stubQueueReorderer{}
	m := New(context.Background(), WithPlayer(ps), WithQueueReorderer(r))
	m.ActiveView = viewQueue
	m.queue.Entries = []queue.Entry{
		{ID: 10, Hash: "ha", Artist: "A", Title: "Ta"},
		{ID: 20, Hash: "hb", Artist: "B", Title: "Tb"},
		{ID: 30, Hash: "hc", Artist: "C", Title: "Tc"},
	}
	m.playback.currentHash = "ha"
	m.playback.currentIndex = 0
	m.queue.Cursor = 0

	// Move the playing track down.
	m, _ = m.handleMoveTrackDownMsg(queue.MoveTrackDownMsg{Index: 0})

	if len(ps.setCurrentIndexCalls) == 0 {
		t.Fatal("SetCurrentIndex was never called after move-down")
	}
	lastIdx := ps.setCurrentIndexCalls[len(ps.setCurrentIndexCalls)-1]
	if lastIdx != m.playback.currentIndex {
		t.Errorf("player index = %d, TUI index = %d; they should match after move-down",
			lastIdx, m.playback.currentIndex)
	}
	if m.playback.currentIndex != 1 {
		t.Errorf("TUI index = %d, want 1 after moving playing track down", m.playback.currentIndex)
	}
}

// TestDedupRemovesDuplicatesKeepsFirst verifies that dedup keeps the first
// occurrence and removes subsequent duplicates.
func TestDedupRemovesDuplicatesKeepsFirst(t *testing.T) {
	d := &stubQueueDequeuer{}
	m := New(context.Background(), WithQueueDequeuer(d))
	m.ActiveView = viewQueue
	m.queue.Entries = []queue.Entry{
		{ID: 1, Hash: "x", Display: "X1"},
		{ID: 2, Hash: "y", Display: "Y"},
		{ID: 3, Hash: "x", Display: "X2"},
		{ID: 4, Hash: "x", Display: "X3"},
	}

	m, _ = m.handleQueueDedupMsg()

	if len(m.queue.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(m.queue.Entries))
	}
	if m.queue.Entries[0].ID != 1 {
		t.Errorf("first entry should be ID=1 (first occurrence), got %d", m.queue.Entries[0].ID)
	}
	if m.queue.Entries[1].ID != 2 {
		t.Errorf("second entry should be ID=2, got %d", m.queue.Entries[1].ID)
	}
}
