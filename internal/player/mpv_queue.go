package player

import (
	"fmt"

	"github.com/sglavoie/muse/internal/safego"
)

// SetQueue replaces the ordered set of tracks used by PlayFromIndex. Call
// whenever the playback queue changes (e.g. after loading from the database).
func (m *Manager) SetQueue(entries []QueueTrack) {
	m.mu.Lock()
	m.queue = make([]QueueTrack, len(entries))
	copy(m.queue, entries)
	if m.currentIndex >= len(m.queue) {
		if len(m.queue) > 0 {
			m.currentIndex = len(m.queue) - 1
		} else {
			m.currentIndex = -1
		}
	}
	m.reconcileShuffleLocked()
	m.mu.Unlock()
	m.clearPrefetch()
	safego.Go("player.prefetchNext", m.prefetchNext)
}

// SetCurrentIndex sets the current queue index used by Next, Previous, and
// auto-advance. Call this during session restore after SetQueue so that
// navigation starts from the correct position rather than defaulting to 0.
// The index is clamped to [0, len(queue)-1]; negative indices are clamped
// to 0. If the queue is empty, currentIndex is set to -1.
//
// Moving the playback position invalidates the previously prefetched track
// (the "next" track is now relative to a different index), so the prefetch
// is cleared and a fresh prefetchNext is scheduled when the index actually
// changes. This prevents shuffle/dedup/move/undo-clear from leaving mpv with
// a stale gapless prefetch pointing at the wrong track.
func (m *Manager) SetCurrentIndex(index int) {
	m.mu.Lock()
	oldIdx := m.currentIndex
	switch {
	case len(m.queue) == 0:
		m.currentIndex = -1
	case index < 0:
		m.currentIndex = 0
	case index >= len(m.queue):
		m.currentIndex = len(m.queue) - 1
	default:
		m.currentIndex = index
	}
	changed := m.currentIndex != oldIdx
	m.mu.Unlock()

	if changed {
		m.clearPrefetch()
		safego.Go("player.prefetchNext", m.prefetchNext)
	}
}

// RestoreShuffleAnchor sets the current queue index during session restore and,
// when shuffle is enabled, reanchors the shuffle cycle around that index so the
// restored track becomes the sole history entry and every other queue identity
// is freshly permuted into the unseen pool. The first Next then advances into
// the remaining queue instead of replaying the restored track.
//
// This is the restore-time counterpart to SetCurrentIndex, which is deliberately
// position-only because it is also on the hot dedup/move/reorder path where
// reanchoring would wrongly wipe playback history. Shuffle history is not
// persisted across restarts, so a fresh permutation is exactly what a restored
// session needs. With shuffle off this behaves like SetCurrentIndex: it only
// updates the index. The index is clamped to [0, len(queue)-1]; an empty queue
// sets currentIndex to -1.
func (m *Manager) RestoreShuffleAnchor(index int) {
	m.mu.Lock()
	switch {
	case len(m.queue) == 0:
		m.currentIndex = -1
	case index < 0:
		m.currentIndex = 0
	case index >= len(m.queue):
		m.currentIndex = len(m.queue) - 1
	default:
		m.currentIndex = index
	}
	if m.shuffleEnabled && m.currentIndex >= 0 {
		m.anchorShuffleLocked(m.currentIndex)
	}
	m.mu.Unlock()

	m.clearPrefetch()
	safego.Go("player.prefetchNext", m.prefetchNext)
}

// AppendToQueue appends a single entry to the in-memory queue used by
// PlayFromIndex. Call this after persisting the entry to the database so that
// the player and TUI queues stay in sync without a full SetQueue replacement.
func (m *Manager) AppendToQueue(entry QueueTrack) {
	m.mu.Lock()
	m.queue = append(m.queue, entry)
	m.reconcileShuffleLocked()
	m.mu.Unlock()
	safego.Go("player.prefetchNext", m.prefetchNext)
}

// RemoveFromQueue removes the queue entry at index from the in-memory queue
// and adjusts currentIndex atomically. If the removed index precedes
// currentIndex, currentIndex is decremented by one so it continues pointing
// at the same track. If the removed index equals currentIndex, currentIndex
// is rewound to the previous queue slot because Next and EOF auto-advance
// load currentIndex+1. Returns an error if index is negative or >= len(m.queue).
func (m *Manager) RemoveFromQueue(index int) error {
	m.mu.Lock()

	if index < 0 || index >= len(m.queue) {
		m.mu.Unlock()
		return fmt.Errorf("index %d out of range", index)
	}

	// Build a new backing array so that any concurrent reader that captured
	// the old slice header via RLock is not affected by this mutation.
	newQ := make([]QueueTrack, 0, len(m.queue)-1)
	newQ = append(newQ, m.queue[:index]...)
	newQ = append(newQ, m.queue[index+1:]...)
	m.queue = newQ
	if index < m.currentIndex {
		m.currentIndex--
	} else if index == m.currentIndex {
		m.currentIndex = index - 1
	}
	if len(m.queue) == 0 {
		m.currentIndex = -1
	}
	m.reconcileShuffleLocked()
	m.mu.Unlock()

	// Clear the prefetch under prefetchMu so that the playlist-remove
	// cannot race with a concurrent prefetchNext appending a new entry.
	m.clearPrefetch()
	safego.Go("player.prefetchNext", m.prefetchNext)

	return nil
}
