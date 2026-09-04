package player

import (
	"errors"
	"fmt"
)

// errTraversalExhausted marks the traversal failures that mean "there is no next
// track to play": the sequential walk ran past the end of the queue, every
// remaining entry was missing on disk, or the shuffle cycle was fully consumed.
//
// It wraps ErrPlaybackFailed so existing callers keep matching, and it lets the
// EOF handler tell real exhaustion apart from a load that failed. That
// distinction is the difference between reporting the end of the queue and
// falsely reporting it: an IPC drop or an mpv rejection leaves playable tracks
// behind, and must not be translated into OnPlaybackEnded.
var errTraversalExhausted = fmt.Errorf("%w: traversal exhausted", ErrPlaybackFailed)

// errEOFSuperseded marks an auto-advance abandoned because the playlist
// generation the EOF was decided against no longer exists: a queue-owned
// replacement won the transaction lock first and mpv is already playing what it
// loaded.
//
// It deliberately does NOT wrap ErrPlaybackFailed. Nothing failed and nothing is
// exhausted — the transition the EOF was going to make has been made by someone
// else — so it must reach neither OnPlaybackEnded nor the failure log.
var errEOFSuperseded = errors.New("eof superseded by a playlist replacement")

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
	body := func() (int, error) { return m.playSequentialFromLocked(index) }
	if shuffle {
		body = func() (int, error) { return m.playShuffleAnchoredLocked(index) }
	}
	_, err := m.runPlaylistTxnIndex(body)
	return err
}

// playSequentialFrom plays the visible queue forward from index, skipping
// missing entries until a playable one is found. This is the plain
// (non-shuffle) traversal used by explicit selection, Next, and Previous.
func (m *Manager) playSequentialFrom(index int) error {
	_, err := m.runPlaylistTxnIndex(func() (int, error) { return m.playSequentialFromLocked(index) })
	return err
}

// playSequentialFromLocked is playSequentialFrom's body inside an open playlist
// transaction: index selection and the mpv replacement that acts on it are one
// serialized operation, so a concurrent prefetch cannot slip between them. It
// returns the queue index that actually started, which is index only when that
// entry was present on disk.
//
// The caller must hold m.playlistMu.
func (m *Manager) playSequentialFromLocked(index int) (int, error) {
	m.mu.RLock()
	queue := m.queue
	marker := m.marker
	m.mu.RUnlock()

	if index < 0 || index >= len(queue) {
		// Do not clear the prefetch here: returning an error means we did
		// not change the playing track, so the existing prefetched "next"
		// is still valid. Clearing would force a redundant playlist-remove
		// against mpv on every bad call.
		return -1, fmt.Errorf("%w: index %d out of range [0, %d)", errTraversalExhausted, index, len(queue))
	}

	for i := index; i < len(queue); i++ {
		err := m.replaceLocked(queue[i], marker, func() func() { return m.commitIndexLocked(i) })
		if err == nil {
			return i, nil
		}
		if !errors.Is(err, ErrMissingFile) {
			return -1, err
		}
	}
	return -1, fmt.Errorf("%w: no playable entry starting at index %d", errTraversalExhausted, index)
}

// continueSequentialLocked is the shuffle-off traversal's automatic
// continuation: the EOF advance and the positional tail of a resume.
//
// It differs from playSequentialFromLocked, which serves explicit selection,
// only in where the candidates come from. A selection means "play the queue
// forward from here"; a continuation means "play what the traversal has not
// reached yet", and once a resume has stepped back onto an insertion those two
// stop agreeing — from is behind rows that already played. The candidate order
// comes from sequentialSuccessorsLocked, and each load is committed as a step so
// the frontier survives it.
//
// The caller must hold m.playlistMu.
func (m *Manager) continueSequentialLocked(from int) (int, error) {
	m.mu.RLock()
	candidates := m.sequentialSuccessorsLocked(from)
	queue := m.queue
	marker := m.marker
	m.mu.RUnlock()

	for _, i := range candidates {
		err := m.replaceLocked(queue[i], marker, func() func() { return m.commitSequentialStepLocked(i) })
		if err == nil {
			return i, nil
		}
		if !errors.Is(err, ErrMissingFile) {
			return -1, err
		}
	}
	return -1, fmt.Errorf("%w: no playable entry left after index %d", errTraversalExhausted, from-1)
}

// playShuffleAnchoredLocked reanchors the shuffle cycle around index and plays
// it, returning the queue index that actually started. The selected identity
// becomes the sole history entry; every other queue identity is re-permuted into
// a fresh unseen pool. If the anchor file is missing it is marked and dropped,
// and traversal advances into the shuffled remainder. A load that fails for any
// other reason rolls the reanchor back, and replaceLocked restores the prior
// current index, so the failed selection leaves no traversal state behind.
//
// The caller must hold m.playlistMu.
func (m *Manager) playShuffleAnchoredLocked(index int) (int, error) {
	m.mu.Lock()
	if index < 0 || index >= len(m.queue) {
		m.mu.Unlock()
		return -1, fmt.Errorf("%w: index %d out of range [0, %d)", ErrPlaybackFailed, index, len(m.queue))
	}
	snap := m.snapshotShuffleLocked()
	m.anchorShuffleLocked(index)
	entry := m.queue[index]
	marker := m.marker
	m.mu.Unlock()

	err := m.replaceLocked(entry, marker, func() func() { return m.commitIndexLocked(index) })
	if err == nil {
		return index, nil
	}
	if !errors.Is(err, ErrMissingFile) {
		// The anchor load failed for a reason other than a missing file: roll
		// back the reanchor so the prior cycle is preserved and a retry
		// re-attempts the same anchor.
		m.mu.Lock()
		m.restoreShuffleLocked(snap)
		m.mu.Unlock()
		return -1, err
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
	_, err := m.runPlaylistTxnIndex(m.advanceForwardLocked)
	return err
}

// advanceForwardLocked is advanceForward's body inside an open playlist
// transaction: each provisional traversal step and the replacement that either
// confirms or rolls it back happen without another playlist mutation in between.
// It returns the queue index the traversal landed on.
//
// The caller must hold m.playlistMu.
func (m *Manager) advanceForwardLocked() (int, error) {
	for {
		m.mu.Lock()
		snap := m.snapshotShuffleLocked()
		id, ok := m.stepForwardLocked()
		if !ok {
			m.mu.Unlock()
			return -1, fmt.Errorf("%w: shuffle cycle exhausted", errTraversalExhausted)
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
			return idx, nil
		}
		if !errors.Is(err, ErrMissingFile) {
			// The load failed for a reason other than a missing file: roll back
			// the provisional step so the identity is not consumed and a retry
			// re-attempts it.
			m.mu.Lock()
			m.restoreShuffleLocked(snap)
			m.mu.Unlock()
			return -1, err
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
	_, err := m.runPlaylistTxnIndex(m.advanceBackwardLocked)
	return err
}

// advanceBackwardLocked is advanceBackward's body inside an open playlist
// transaction. It returns the queue index the traversal landed on.
//
// The caller must hold m.playlistMu.
func (m *Manager) advanceBackwardLocked() (int, error) {
	for {
		m.mu.Lock()
		if m.shufflePos <= 0 {
			m.mu.Unlock()
			return -1, fmt.Errorf("%w: no previous track in shuffle history", errTraversalExhausted)
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
				return idx, nil
			}
			if !errors.Is(err, ErrMissingFile) {
				// The load failed for a reason other than a missing file: roll
				// back the cursor move so the previous identity is not consumed
				// and a retry re-attempts it.
				m.mu.Lock()
				m.restoreShuffleLocked(snap)
				m.mu.Unlock()
				return -1, err
			}
			skip = true
		}
		if skip {
			// Prune the unplayable entry at p and leave the cursor at p so
			// the next iteration steps to p-1 (the earlier history entry).
			m.mu.Lock()
			m.excludeIdentityLocked(id)
			if p >= 0 && p < len(m.shuffleHistory) {
				m.shuffleHistory = append(m.shuffleHistory[:p], m.shuffleHistory[p+1:]...)
			}
			m.shufflePos = p
			m.mu.Unlock()
		}
	}
}

// advanceAfterCurrentLocked advances playback after the current track ends. It
// is the internal traversal load used by auto-advance (EOF): it advances
// existing shuffle history without reanchoring the cycle, or continues the
// shuffle-off traversal past the row that finished. It returns the queue index
// the traversal landed on.
//
// It has no unlocked counterpart on purpose: its only caller runs it inside a
// transaction it opened itself, so that the playlist generation the EOF was
// decided against can be re-validated in the same serialized step as the load.
//
// The caller must hold m.playlistMu.
func (m *Manager) advanceAfterCurrentLocked(seqNext int) (int, error) {
	m.mu.RLock()
	shuffle := m.shuffleEnabled
	m.mu.RUnlock()
	if shuffle {
		return m.advanceForwardLocked()
	}
	return m.continueSequentialLocked(seqNext)
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
		_, err := m.playSequentialFromLocked(0)
		return err
	}
	// advanceForwardLocked rolls back its own step on a failed load, but the
	// fresh permutation from restartShuffleCycleLocked is a separate mutation: if
	// nothing plays, restore the prior cycle so the failed restart leaves
	// traversal state unchanged and a retry regenerates from the same point.
	if _, err := m.advanceForwardLocked(); err != nil {
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
