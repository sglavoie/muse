package player

import (
	"errors"
	"os"
	"sync"
	"testing"
)

// shuffleTestQueue builds an n-entry queue backed by real temp files (so
// PlayQueueEntry's os.Stat succeeds) with sequential QueueIDs 1..n. Paths are
// captured so tests can assert which entry played via currentIndex.
func shuffleTestQueue(t *testing.T, n int) []QueueTrack {
	t.Helper()
	dir := t.TempDir()
	q := make([]QueueTrack, n)
	for i := range q {
		path := dir + "/" + string(rune('a'+i)) + ".flac"
		if err := os.WriteFile(path, []byte{}, 0o600); err != nil {
			t.Fatalf("write temp file: %v", err)
		}
		q[i] = QueueTrack{Path: path, Hash: "h" + string(rune('0'+i)), QueueID: int64(i + 1)}
	}
	return q
}

// currentIdx reads the manager's current queue index under the lock.
func currentIdx(m *Manager) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.currentIndex
}

// reversePermute returns a permutation of [0,n) in descending order, giving a
// deterministic non-identity order for traversal assertions.
func reversePermute(n int) []int {
	p := make([]int, n)
	for i := range p {
		p[i] = n - 1 - i
	}
	return p
}

// TestShuffleTraversal_VisitsEveryIdentityOnce drives a full shuffle cycle via
// Next and asserts every queue identity plays exactly once, in the injected
// permutation order, before the cycle is exhausted.
func TestShuffleTraversal_VisitsEveryIdentityOnce(t *testing.T) {
	var mu sync.Mutex
	var received []ipcTestCommand
	m := newConnectedManager(t, "shuf-cycle", &mu, &received)

	m.mu.Lock()
	m.queue = shuffleTestQueue(t, 4)
	m.currentIndex = 0 // index 0 is the anchor (currently playing)
	m.mu.Unlock()

	// Fixed permutation: reverse order over the 3 non-anchor entries [1,2,3].
	m.SetPermuteFunc(reversePermute)

	if err := m.SetShuffle(true); err != nil {
		t.Fatalf("SetShuffle(true): %v", err)
	}

	// Anchor is index 0; reverse permutation of [1,2,3] -> visit 3, 2, 1.
	visited := []int{currentIdx(m)}
	want := []int{0, 3, 2, 1}
	for range 3 {
		if err := m.Next(); err != nil {
			t.Fatalf("Next: unexpected error: %v", err)
		}
		visited = append(visited, currentIdx(m))
	}

	if len(visited) != len(want) {
		t.Fatalf("visited %v, want %v", visited, want)
	}
	for i := range want {
		if visited[i] != want[i] {
			t.Fatalf("visited = %v, want %v", visited, want)
		}
	}

	// The cycle is now exhausted: another Next must fail without revisiting.
	if err := m.Next(); !errors.Is(err, ErrPlaybackFailed) {
		t.Fatalf("Next after exhaustion: want ErrPlaybackFailed, got %v", err)
	}

	// Every distinct index visited exactly once.
	seen := map[int]int{}
	for _, v := range visited {
		seen[v]++
	}
	if len(seen) != 4 {
		t.Fatalf("visited %d distinct indices, want 4: %v", len(seen), visited)
	}
	for idx, count := range seen {
		if count != 1 {
			t.Errorf("index %d visited %d times, want 1", idx, count)
		}
	}
}

// TestShuffleTraversal_DuplicatesStayDistinct verifies that two queue rows that
// share the same hash and path (distinct QueueIDs) are treated as separate
// identities: a full cycle visits both.
func TestShuffleTraversal_DuplicatesStayDistinct(t *testing.T) {
	var mu sync.Mutex
	var received []ipcTestCommand
	m := newConnectedManager(t, "shuf-dup", &mu, &received)

	dir := t.TempDir()
	path := dir + "/dup.flac"
	if err := os.WriteFile(path, []byte{}, 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	m.mu.Lock()
	// Three rows: indices 0 and 2 are the *same* track (hash/path), distinct
	// QueueIDs; index 1 is different.
	other := dir + "/other.flac"
	if err := os.WriteFile(other, []byte{}, 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	m.queue = []QueueTrack{
		{Path: path, Hash: "dup", QueueID: 1},
		{Path: other, Hash: "other", QueueID: 2},
		{Path: path, Hash: "dup", QueueID: 3},
	}
	m.currentIndex = 0
	m.mu.Unlock()

	if err := m.SetShuffle(true); err != nil {
		t.Fatalf("SetShuffle(true): %v", err)
	}

	visited := map[int]int{currentIdx(m): 1}
	for range 2 {
		if err := m.Next(); err != nil {
			t.Fatalf("Next: %v", err)
		}
		visited[currentIdx(m)]++
	}

	if len(visited) != 3 {
		t.Fatalf("visited %d distinct indices, want 3 (duplicates distinct): %v", len(visited), visited)
	}
	// Both duplicate rows (0 and 2) must have been visited.
	if visited[0] == 0 || visited[2] == 0 {
		t.Errorf("both duplicate rows must be visited; got %v", visited)
	}
}

// TestShuffleTraversal_PreviousAndForwardHistory verifies that Previous walks
// the exact playback history backward and that Next then re-walks forward
// history (in the same order) before consuming a new unseen entry.
func TestShuffleTraversal_PreviousAndForwardHistory(t *testing.T) {
	var mu sync.Mutex
	var received []ipcTestCommand
	m := newConnectedManager(t, "shuf-prev", &mu, &received)

	m.mu.Lock()
	m.queue = shuffleTestQueue(t, 4)
	m.currentIndex = 0
	m.mu.Unlock()

	m.SetPermuteFunc(reversePermute) // unseen order over [1,2,3] -> 3,2,1
	if err := m.SetShuffle(true); err != nil {
		t.Fatalf("SetShuffle(true): %v", err)
	}

	// Advance: 0 -> 3 -> 2.
	mustNext := func() {
		if err := m.Next(); err != nil {
			t.Fatalf("Next: %v", err)
		}
	}
	mustNext()
	mustNext()
	if got := currentIdx(m); got != 2 {
		t.Fatalf("after two Next, currentIndex = %d, want 2", got)
	}

	// Previous walks history backward: 2 -> 3 -> 0.
	if err := m.Previous(); err != nil {
		t.Fatalf("Previous: %v", err)
	}
	if got := currentIdx(m); got != 3 {
		t.Fatalf("Previous #1: currentIndex = %d, want 3", got)
	}
	if err := m.Previous(); err != nil {
		t.Fatalf("Previous: %v", err)
	}
	if got := currentIdx(m); got != 0 {
		t.Fatalf("Previous #2: currentIndex = %d, want 0 (anchor)", got)
	}
	// At the start of history: Previous must fail.
	if err := m.Previous(); !errors.Is(err, ErrPlaybackFailed) {
		t.Fatalf("Previous at history start: want ErrPlaybackFailed, got %v", err)
	}

	// Next now re-walks forward history: 0 -> 3 -> 2, then consumes the last
	// unseen entry (1).
	mustNext()
	if got := currentIdx(m); got != 3 {
		t.Fatalf("forward-history Next #1: currentIndex = %d, want 3", got)
	}
	mustNext()
	if got := currentIdx(m); got != 2 {
		t.Fatalf("forward-history Next #2: currentIndex = %d, want 2", got)
	}
	mustNext()
	if got := currentIdx(m); got != 1 {
		t.Fatalf("unseen Next: currentIndex = %d, want 1", got)
	}
}

// TestShuffleTraversal_ExplicitSelectionReanchors verifies that an explicit
// PlayFromIndex reanchors the shuffle cycle: the selected identity becomes the
// sole history entry and a fresh permutation of the remaining entries follows.
func TestShuffleTraversal_ExplicitSelectionReanchors(t *testing.T) {
	var mu sync.Mutex
	var received []ipcTestCommand
	m := newConnectedManager(t, "shuf-anchor", &mu, &received)

	m.mu.Lock()
	m.queue = shuffleTestQueue(t, 4)
	m.currentIndex = 0
	m.mu.Unlock()

	m.SetPermuteFunc(reversePermute)
	if err := m.SetShuffle(true); err != nil {
		t.Fatalf("SetShuffle(true): %v", err)
	}

	// Advance a couple of steps so history is non-trivial.
	if err := m.Next(); err != nil {
		t.Fatalf("Next: %v", err)
	}

	// Explicitly select index 1. This reanchors the cycle at identity 1.
	if err := m.PlayFromIndex(1); err != nil {
		t.Fatalf("PlayFromIndex(1): %v", err)
	}
	if got := currentIdx(m); got != 1 {
		t.Fatalf("after PlayFromIndex(1), currentIndex = %d, want 1", got)
	}

	m.mu.RLock()
	histLen := len(m.shuffleHistory)
	pos := m.shufflePos
	m.mu.RUnlock()
	if histLen != 1 || pos != 0 {
		t.Fatalf("after reanchor: history len = %d, pos = %d; want 1, 0", histLen, pos)
	}

	// Previous must fail immediately: the reanchor discarded prior history.
	if err := m.Previous(); !errors.Is(err, ErrPlaybackFailed) {
		t.Fatalf("Previous after reanchor: want ErrPlaybackFailed, got %v", err)
	}

	// A full cycle from the new anchor visits the remaining 3 entries once.
	visited := map[int]int{currentIdx(m): 1}
	for range 3 {
		if err := m.Next(); err != nil {
			t.Fatalf("Next: %v", err)
		}
		visited[currentIdx(m)]++
	}
	if len(visited) != 4 {
		t.Fatalf("reanchored cycle visited %d distinct indices, want 4: %v", len(visited), visited)
	}
}

// TestShuffleTraversal_DisableResumesSequential verifies that disabling shuffle
// immediately returns Next/Previous to visible-order sequencing from the
// current queue index.
func TestShuffleTraversal_DisableResumesSequential(t *testing.T) {
	var mu sync.Mutex
	var received []ipcTestCommand
	m := newConnectedManager(t, "shuf-disable", &mu, &received)

	m.mu.Lock()
	m.queue = shuffleTestQueue(t, 4)
	m.currentIndex = 0
	m.mu.Unlock()

	m.SetPermuteFunc(reversePermute)
	if err := m.SetShuffle(true); err != nil {
		t.Fatalf("SetShuffle(true): %v", err)
	}
	// Jump around under shuffle so currentIndex is not 0.
	if err := m.Next(); err != nil { // -> 3
		t.Fatalf("Next: %v", err)
	}
	if got := currentIdx(m); got != 3 {
		t.Fatalf("under shuffle currentIndex = %d, want 3", got)
	}

	// Disable shuffle: Next/Previous must now be sequential from index 3.
	if err := m.SetShuffle(false); err != nil {
		t.Fatalf("SetShuffle(false): %v", err)
	}
	m.mu.RLock()
	histLen := len(m.shuffleHistory)
	m.mu.RUnlock()
	if histLen != 0 {
		t.Errorf("shuffle history not cleared after disable: len = %d", histLen)
	}

	if err := m.Previous(); err != nil { // 3 -> 2
		t.Fatalf("Previous: %v", err)
	}
	if got := currentIdx(m); got != 2 {
		t.Fatalf("sequential Previous: currentIndex = %d, want 2", got)
	}
	if err := m.Next(); err != nil { // 2 -> 3
		t.Fatalf("Next: %v", err)
	}
	if got := currentIdx(m); got != 3 {
		t.Fatalf("sequential Next: currentIndex = %d, want 3", got)
	}
}

// TestShuffleTraversal_SkipsMissingCandidates verifies that missing files are
// marked and skipped in traversal order until a playable identity is found.
func TestShuffleTraversal_SkipsMissingCandidates(t *testing.T) {
	var mu sync.Mutex
	var received []ipcTestCommand
	m := newConnectedManager(t, "shuf-missing", &mu, &received)

	dir := t.TempDir()
	anchor := dir + "/anchor.flac"
	playable := dir + "/playable.flac"
	for _, p := range []string{anchor, playable} {
		if err := os.WriteFile(p, []byte{}, 0o600); err != nil {
			t.Fatalf("write temp file: %v", err)
		}
	}

	m.mu.Lock()
	// Index 0 anchor (playable); indices 1,2 missing; index 3 playable.
	m.queue = []QueueTrack{
		{Path: anchor, Hash: "h0", QueueID: 1},
		{Path: "/nonexistent-shuf-1.flac", Hash: "h1", QueueID: 2},
		{Path: "/nonexistent-shuf-2.flac", Hash: "h2", QueueID: 3},
		{Path: playable, Hash: "h3", QueueID: 4},
	}
	m.currentIndex = 0
	m.mu.Unlock()

	marker := &countingMarker{}
	m.SetMissingMarker(marker)

	// Identity-order unseen: [1,2,3]. Next skips 1 and 2 (missing) to 3.
	if err := m.SetShuffle(true); err != nil {
		t.Fatalf("SetShuffle(true): %v", err)
	}
	if err := m.Next(); err != nil {
		t.Fatalf("Next: %v", err)
	}
	if got := currentIdx(m); got != 3 {
		t.Fatalf("Next skipping missing: currentIndex = %d, want 3", got)
	}
	if marker.trackCalls() < 2 {
		t.Errorf("MarkTrackMissing called %d times, want >= 2", marker.trackCalls())
	}

	// Cycle now exhausted (1 and 2 dropped, 3 played): Next fails.
	if err := m.Next(); !errors.Is(err, ErrPlaybackFailed) {
		t.Fatalf("Next after skip+exhaust: want ErrPlaybackFailed, got %v", err)
	}
}

// countingMarker counts missing-mark calls without mutating the queue.
type countingMarker struct {
	mu     sync.Mutex
	tracks int
	queues int
}

func (c *countingMarker) MarkTrackMissing(_ string) error {
	c.mu.Lock()
	c.tracks++
	c.mu.Unlock()
	return nil
}

func (c *countingMarker) MarkQueueEntryMissing(_ int64) error {
	c.mu.Lock()
	c.queues++
	c.mu.Unlock()
	return nil
}

func (c *countingMarker) trackCalls() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.tracks
}

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
