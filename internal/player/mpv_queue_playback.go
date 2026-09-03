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
		return m.runPlaylistTxn(func() error { return m.playShuffleAnchoredLocked(index) })
	}
	return m.runPlaylistTxn(func() error { return m.playSequentialFromLocked(index) })
}

// playSequentialFrom plays the visible queue forward from index, skipping
// missing entries until a playable one is found. This is the plain
// (non-shuffle) traversal used by explicit selection, Next, and Previous.
func (m *Manager) playSequentialFrom(index int) error {
	return m.runPlaylistTxn(func() error { return m.playSequentialFromLocked(index) })
}

// playSequentialFromLocked is playSequentialFrom's body inside an open playlist
// transaction: index selection and the mpv replacement that acts on it are one
// serialized operation, so a concurrent prefetch cannot slip between them.
//
// The caller must hold m.playlistMu.
func (m *Manager) playSequentialFromLocked(index int) error {
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

	for i := index; i < len(queue); i++ {
		err := m.replaceLocked(queue[i], marker, func() func() { return m.commitIndexLocked(i) })
		if err == nil {
			return nil
		}
		if !errors.Is(err, ErrMissingFile) {
			return err
		}
	}
	return fmt.Errorf("%w: no playable entry starting at index %d", ErrPlaybackFailed, index)
}

// playShuffleAnchoredLocked reanchors the shuffle cycle around index and plays
// it. The selected identity becomes the sole history entry; every other queue
// identity is re-permuted into a fresh unseen pool. If the anchor file is missing
// it is marked and dropped, and traversal advances into the shuffled remainder. A
// load that fails for any other reason rolls the reanchor back, and
// replaceLocked restores the prior current index, so the failed selection leaves
// no traversal state behind.
//
// The caller must hold m.playlistMu.
func (m *Manager) playShuffleAnchoredLocked(index int) error {
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

	err := m.replaceLocked(entry, marker, func() func() { return m.commitIndexLocked(index) })
	if err == nil {
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
	return m.advanceForwardLocked()
}

// advanceForward moves one step forward through the shuffle traversal (forward
// history first, then the unseen pool) and plays the next playable identity.
// Missing or removed candidates are skipped in traversal order until a playable
// identity is found or the cycle is exhausted (ErrPlaybackFailed).
func (m *Manager) advanceForward() error {
	return m.runPlaylistTxn(m.advanceForwardLocked)
}

// advanceForwardLocked is advanceForward's body inside an open playlist
// transaction: each provisional traversal step and the replacement that either
// confirms or rolls it back happen without another playlist mutation in between.
//
// The caller must hold m.playlistMu.
func (m *Manager) advanceForwardLocked() error {
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
		err := m.replaceLocked(entry, marker, func() func() { return m.commitIndexLocked(idx) })
		if err == nil {
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
	return m.runPlaylistTxn(m.advanceBackwardLocked)
}

// advanceBackwardLocked is advanceBackward's body inside an open playlist
// transaction.
//
// The caller must hold m.playlistMu.
func (m *Manager) advanceBackwardLocked() error {
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
			err := m.replaceLocked(entry, marker, func() func() { return m.commitIndexLocked(idx) })
			if err == nil {
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
	return m.runPlaylistTxn(m.restartQueueCycleLocked)
}

// restartQueueCycleLocked regenerates the cycle and plays its first entry inside
// one playlist transaction, so the fresh permutation and the mpv replacement that
// acts on it cannot be observed apart.
//
// The caller must hold m.playlistMu.
func (m *Manager) restartQueueCycleLocked() error {
	m.mu.Lock()
	shuffle := m.shuffleEnabled
	var snap shuffleSnapshot
	if shuffle {
		snap = m.snapshotShuffleLocked()
		m.restartShuffleCycleLocked()
	}
	m.mu.Unlock()
	if !shuffle {
		return m.playSequentialFromLocked(0)
	}
	// advanceForwardLocked rolls back its own step on a failed load, but the
	// fresh permutation from restartShuffleCycleLocked is a separate mutation: if
	// nothing plays, restore the prior cycle so the failed restart leaves
	// traversal state unchanged and a retry regenerates from the same point.
	if err := m.advanceForwardLocked(); err != nil {
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
