package player

import (
	"errors"
	"fmt"
)

// PlayFromIndex starts playback from the queue entry at index in response to an
// explicit user selection. When shuffle is off it plays the visible queue
// forward from index, skipping missing entries. When shuffle is on it reanchors
// the shuffle cycle around the selected identity (fresh permutation of every
// other entry) and plays it. The queue must have been populated via SetQueue.
// Returns ErrPlaybackFailed if index is out of range or no playable entry
// exists.
func (m *Manager) PlayFromIndex(index int) error {
	m.mu.RLock()
	shuffle := m.shuffleEnabled
	m.mu.RUnlock()
	if shuffle {
		return m.playShuffleAnchored(index)
	}
	return m.playSequentialFrom(index)
}

// playSequentialFrom plays the visible queue forward from index, skipping
// missing entries until a playable one is found. This is the plain
// (non-shuffle) traversal used by explicit selection, Next, and Previous.
func (m *Manager) playSequentialFrom(index int) error {
	m.mu.RLock()
	queue := m.queue
	marker := m.marker
	m.mu.RUnlock()

	if index < 0 || index >= len(queue) {
		// Do not clear the prefetch here: returning an error means we did
		// not change the playing track, so the existing prefetched "next"
		// is still valid. Clearing would force a redundant playlist-remove
		// against mpv on every bad call.
		return fmt.Errorf("%w: index %d out of range [0, %d)", ErrPlaybackFailed, index, len(queue))
	}

	m.clearPrefetch()

	mk := marker
	if mk == nil {
		mk = noopMarker{}
	}

	for i := index; i < len(queue); i++ {
		err := m.PlayQueueEntry(queue[i], mk)
		if err == nil {
			m.commitCurrentIndex(i)
			go m.prefetchNext()
			return nil
		}
		if !errors.Is(err, ErrMissingFile) {
			return err
		}
	}
	return fmt.Errorf("%w: no playable entry starting at index %d", ErrPlaybackFailed, index)
}

// commitCurrentIndex records i as the current queue index after a successful
// load, clamped against the live queue length. A concurrent SetQueue /
// RemoveFromQueue can shrink m.queue while a caller iterates over a snapshot,
// and setting an out-of-range currentIndex would leave Next/Previous and the
// event reader pointing past the end of the live queue.
func (m *Manager) commitCurrentIndex(i int) {
	m.mu.Lock()
	switch {
	case len(m.queue) == 0:
		m.currentIndex = -1
	case i >= len(m.queue):
		m.currentIndex = len(m.queue) - 1
	default:
		m.currentIndex = i
	}
	m.mu.Unlock()
}

// playShuffleAnchored reanchors the shuffle cycle around index and plays it.
// The selected identity becomes the sole history entry; every other queue
// identity is re-permuted into a fresh unseen pool. If the anchor file is
// missing it is marked and dropped, and traversal advances into the shuffled
// remainder.
func (m *Manager) playShuffleAnchored(index int) error {
	m.mu.Lock()
	if index < 0 || index >= len(m.queue) {
		m.mu.Unlock()
		return fmt.Errorf("%w: index %d out of range [0, %d)", ErrPlaybackFailed, index, len(m.queue))
	}
	snap := m.snapshotShuffleLocked()
	m.anchorShuffleLocked(index)
	entry := m.queue[index]
	marker := m.marker
	m.mu.Unlock()

	m.clearPrefetch()

	mk := marker
	if mk == nil {
		mk = noopMarker{}
	}
	err := m.PlayQueueEntry(entry, mk)
	if err == nil {
		m.commitCurrentIndex(index)
		go m.prefetchNext()
		return nil
	}
	if !errors.Is(err, ErrMissingFile) {
		// The anchor load failed for a reason other than a missing file: roll
		// back the reanchor so the prior cycle is preserved and a retry
		// re-attempts the same anchor.
		m.mu.Lock()
		m.restoreShuffleLocked(snap)
		m.mu.Unlock()
		return err
	}
	// Anchor unplayable: drop it from history and advance into the remainder.
	m.mu.Lock()
	m.dropCurrentLocked()
	m.mu.Unlock()
	return m.advanceForward()
}

// advanceForward moves one step forward through the shuffle traversal (forward
// history first, then the unseen pool) and plays the next playable identity.
// Missing or removed candidates are skipped in traversal order until a playable
// identity is found or the cycle is exhausted (ErrPlaybackFailed).
func (m *Manager) advanceForward() error {
	m.clearPrefetch()
	for {
		m.mu.Lock()
		snap := m.snapshotShuffleLocked()
		id, ok := m.stepForwardLocked()
		if !ok {
			m.mu.Unlock()
			return fmt.Errorf("%w: shuffle cycle exhausted", ErrPlaybackFailed)
		}
		idx := identityIndex(m.queue, id)
		var entry QueueTrack
		if idx >= 0 {
			entry = m.queue[idx]
		}
		marker := m.marker
		m.mu.Unlock()

		if idx < 0 {
			// Identity no longer in the queue (removed): drop and continue.
			m.mu.Lock()
			m.dropCurrentLocked()
			m.mu.Unlock()
			continue
		}
		mk := marker
		if mk == nil {
			mk = noopMarker{}
		}
		err := m.PlayQueueEntry(entry, mk)
		if err == nil {
			m.commitCurrentIndex(idx)
			go m.prefetchNext()
			return nil
		}
		if !errors.Is(err, ErrMissingFile) {
			// The load failed for a reason other than a missing file: roll back
			// the provisional step so the identity is not consumed and a retry
			// re-attempts it.
			m.mu.Lock()
			m.restoreShuffleLocked(snap)
			m.mu.Unlock()
			return err
		}
		// Missing on disk: drop from history so it is not revisited, continue.
		m.mu.Lock()
		m.dropCurrentLocked()
		m.mu.Unlock()
	}
}

// advanceBackward walks the shuffle playback history backward and plays the
// previous playable identity. Missing or removed history entries are pruned in
// traversal order until a playable one is found or the history start is reached
// (ErrPlaybackFailed).
func (m *Manager) advanceBackward() error {
	m.clearPrefetch()
	for {
		m.mu.Lock()
		if m.shufflePos <= 0 {
			m.mu.Unlock()
			return fmt.Errorf("%w: no previous track in shuffle history", ErrPlaybackFailed)
		}
		snap := m.snapshotShuffleLocked()
		p := m.shufflePos - 1
		id := m.shuffleHistory[p]
		idx := identityIndex(m.queue, id)
		var entry QueueTrack
		if idx >= 0 {
			entry = m.queue[idx]
		}
		marker := m.marker
		m.shufflePos = p
		m.mu.Unlock()

		skip := idx < 0
		if !skip {
			mk := marker
			if mk == nil {
				mk = noopMarker{}
			}
			err := m.PlayQueueEntry(entry, mk)
			if err == nil {
				m.commitCurrentIndex(idx)
				go m.prefetchNext()
				return nil
			}
			if !errors.Is(err, ErrMissingFile) {
				// The load failed for a reason other than a missing file: roll
				// back the cursor move so the previous identity is not consumed
				// and a retry re-attempts it.
				m.mu.Lock()
				m.restoreShuffleLocked(snap)
				m.mu.Unlock()
				return err
			}
			skip = true
		}
		if skip {
			// Prune the unplayable entry at p and leave the cursor at p so
			// the next iteration steps to p-1 (the earlier history entry).
			m.mu.Lock()
			if p >= 0 && p < len(m.shuffleHistory) {
				m.shuffleHistory = append(m.shuffleHistory[:p], m.shuffleHistory[p+1:]...)
			}
			m.shufflePos = p
			m.mu.Unlock()
		}
	}
}

// advanceAfterCurrent advances playback after the current track ends. It is the
// internal traversal load used by auto-advance (EOF): it advances existing
// shuffle history without reanchoring the cycle, or plays the sequential next
// index when shuffle is off.
func (m *Manager) advanceAfterCurrent(seqNext int) error {
	m.mu.RLock()
	shuffle := m.shuffleEnabled
	m.mu.RUnlock()
	if shuffle {
		return m.advanceForward()
	}
	return m.playSequentialFrom(seqNext)
}

// RestartQueueCycle restarts playback of the queue from the beginning of a
// fresh cycle. With shuffle off it plays sequentially from index zero. With
// shuffle on it generates a new permutation across the whole queue (avoiding an
// immediate replay of the just-played track when at least two entries exist)
// and plays the first entry of that cycle.
func (m *Manager) RestartQueueCycle() error {
	m.mu.Lock()
	shuffle := m.shuffleEnabled
	var snap shuffleSnapshot
	if shuffle {
		snap = m.snapshotShuffleLocked()
		m.restartShuffleCycleLocked()
	}
	m.mu.Unlock()
	if !shuffle {
		return m.playSequentialFrom(0)
	}
	// advanceForward rolls back its own step on a failed load, but the fresh
	// permutation from restartShuffleCycleLocked is a separate mutation: if
	// nothing plays, restore the prior cycle so the failed restart leaves
	// traversal state unchanged and a retry regenerates from the same point.
	if err := m.advanceForward(); err != nil {
		m.mu.Lock()
		m.restoreShuffleLocked(snap)
		m.mu.Unlock()
		return err
	}
	return nil
}

// Next advances to the next track. With shuffle off it plays the next visible
// queue index. With shuffle on it advances existing shuffle history (forward
// history before unseen entries) without reanchoring the cycle. The Manager
// (not mpv's internal playlist) owns sequencing.
// Returns ErrNotConnected if Start has not been called.
func (m *Manager) Next() error {
	m.mu.RLock()
	shuffle := m.shuffleEnabled
	idx := m.currentIndex
	notStarted := m.client == nil && m.cmdQueue == nil
	m.mu.RUnlock()
	if notStarted {
		return ErrNotConnected
	}
	if shuffle {
		return m.advanceForward()
	}
	return m.playSequentialFrom(idx + 1)
}

// Previous returns to the previous track. With shuffle off it plays the
// previous visible queue index. With shuffle on it walks the exact playback
// history backward. Returns ErrNotConnected if Start has not been called.
func (m *Manager) Previous() error {
	m.mu.RLock()
	shuffle := m.shuffleEnabled
	idx := m.currentIndex
	notStarted := m.client == nil && m.cmdQueue == nil
	m.mu.RUnlock()
	if notStarted {
		return ErrNotConnected
	}
	if shuffle {
		return m.advanceBackward()
	}
	return m.playSequentialFrom(idx - 1)
}
