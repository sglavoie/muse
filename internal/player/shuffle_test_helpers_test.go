package player

import (
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
