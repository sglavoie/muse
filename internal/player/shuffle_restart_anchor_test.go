package player

import (
	"errors"
	"sync"
	"testing"
)

// TestRestartQueueCycle_ShuffleGeneratesNewPermutation verifies that
// RestartQueueCycle with shuffle on starts a fresh full cycle and, with two or
// more entries, does not immediately replay the just-played track.
func TestRestartQueueCycle_ShuffleGeneratesNewPermutation(t *testing.T) {
	var mu sync.Mutex
	var received []ipcTestCommand
	m := newConnectedManager(t, "shuf-restart", &mu, &received)

	m.mu.Lock()
	m.queue = shuffleTestQueue(t, 3)
	m.currentIndex = 0
	m.mu.Unlock()

	// Identity-order permutation would put index 0 first; RestartQueueCycle
	// must swap it away since >= 2 entries exist and 0 was just played.
	if err := m.SetShuffle(true); err != nil {
		t.Fatalf("SetShuffle(true): %v", err)
	}
	if err := m.RestartQueueCycle(); err != nil {
		t.Fatalf("RestartQueueCycle: %v", err)
	}
	if got := currentIdx(m); got == 0 {
		t.Fatalf("RestartQueueCycle replayed the prior track (index 0); want a different first entry")
	}

	// A fresh full cycle from here visits all three entries once.
	visited := map[int]int{currentIdx(m): 1}
	for {
		if err := m.Next(); err != nil {
			break
		}
		visited[currentIdx(m)]++
	}
	if len(visited) != 3 {
		t.Fatalf("restarted cycle visited %d distinct indices, want 3: %v", len(visited), visited)
	}
}

// TestRestoreShuffleAnchor_AnchorsRestoredTrack reproduces the session-restore
// path: SetQueue leaves currentIndex at -1, SetShuffle(true) then builds the
// cycle anchored at nothing (every identity is unseen), and RestoreShuffleAnchor
// finally anchors the restored track. It verifies the restored identity becomes
// the sole history entry and is NOT left in the unseen pool, so the first Next
// advances into the remaining queue instead of replaying the restored track.
func TestRestoreShuffleAnchor_AnchorsRestoredTrack(t *testing.T) {
	var mu sync.Mutex
	var received []ipcTestCommand
	m := newConnectedManager(t, "shuf-restore", &mu, &received)

	m.mu.Lock()
	m.queue = shuffleTestQueue(t, 4)
	m.currentIndex = -1 // mirror restore: SetQueue ran, no anchor yet
	m.mu.Unlock()

	m.SetPermuteFunc(reversePermute)
	if err := m.SetShuffle(true); err != nil {
		t.Fatalf("SetShuffle(true): %v", err)
	}

	// Restore anchors the saved current track (index 2).
	m.RestoreShuffleAnchor(2)
	if got := currentIdx(m); got != 2 {
		t.Fatalf("after RestoreShuffleAnchor(2), currentIndex = %d, want 2", got)
	}

	m.mu.RLock()
	histLen := len(m.shuffleHistory)
	pos := m.shufflePos
	anchoredID := ""
	if pos >= 0 && pos < len(m.shuffleHistory) {
		anchoredID = m.shuffleHistory[pos]
	}
	unseen := append([]string(nil), m.shuffleUnseen...)
	m.mu.RUnlock()
	if histLen != 1 || pos != 0 {
		t.Fatalf("after restore anchor: history len = %d, pos = %d; want 1, 0", histLen, pos)
	}
	for _, id := range unseen {
		if id == anchoredID {
			t.Fatalf("restored track identity %q must not remain in the unseen pool %v", anchoredID, unseen)
		}
	}

	// A full cycle from the restored anchor visits the other three entries once
	// and never replays index 2.
	visited := []int{currentIdx(m)}
	for range 3 {
		if err := m.Next(); err != nil {
			t.Fatalf("Next: %v", err)
		}
		if got := currentIdx(m); got == 2 {
			t.Fatalf("restored track (index 2) was replayed during the cycle: %v", append(visited, got))
		}
		visited = append(visited, currentIdx(m))
	}
	if err := m.Next(); !errors.Is(err, ErrPlaybackFailed) {
		t.Fatalf("Next after full cycle: want ErrPlaybackFailed, got %v", err)
	}
	seen := map[int]int{}
	for _, v := range visited {
		seen[v]++
	}
	if len(seen) != 4 {
		t.Fatalf("restored cycle visited %d distinct indices, want 4: %v", len(seen), visited)
	}
}

// TestRestoreShuffleAnchor_ShuffleOffSetsIndexOnly verifies that with shuffle
// disabled RestoreShuffleAnchor behaves like SetCurrentIndex: it only updates
// the current index and does not build any shuffle history.
func TestRestoreShuffleAnchor_ShuffleOffSetsIndexOnly(t *testing.T) {
	var mu sync.Mutex
	var received []ipcTestCommand
	m := newConnectedManager(t, "restore-noshuf", &mu, &received)

	m.mu.Lock()
	m.queue = shuffleTestQueue(t, 4)
	m.currentIndex = 0
	m.mu.Unlock()

	m.RestoreShuffleAnchor(3)
	if got := currentIdx(m); got != 3 {
		t.Fatalf("RestoreShuffleAnchor(3) with shuffle off: currentIndex = %d, want 3", got)
	}
	m.mu.RLock()
	histLen := len(m.shuffleHistory)
	m.mu.RUnlock()
	if histLen != 0 {
		t.Errorf("shuffle off: history len = %d, want 0 (no anchoring)", histLen)
	}
}

// TestRestartQueueCycle_SequentialFromZero verifies that with shuffle off
// RestartQueueCycle plays from index zero.
func TestRestartQueueCycle_SequentialFromZero(t *testing.T) {
	var mu sync.Mutex
	var received []ipcTestCommand
	m := newConnectedManager(t, "seq-restart", &mu, &received)

	m.mu.Lock()
	m.queue = shuffleTestQueue(t, 3)
	m.currentIndex = 2
	m.mu.Unlock()

	if err := m.RestartQueueCycle(); err != nil {
		t.Fatalf("RestartQueueCycle: %v", err)
	}
	if got := currentIdx(m); got != 0 {
		t.Fatalf("RestartQueueCycle (shuffle off): currentIndex = %d, want 0", got)
	}
}
