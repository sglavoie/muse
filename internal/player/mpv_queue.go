package player

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
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
	go m.prefetchNext()
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
		go m.prefetchNext()
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
	go m.prefetchNext()
}

// AppendToQueue appends a single entry to the in-memory queue used by
// PlayFromIndex. Call this after persisting the entry to the database so that
// the player and TUI queues stay in sync without a full SetQueue replacement.
func (m *Manager) AppendToQueue(entry QueueTrack) {
	m.mu.Lock()
	m.queue = append(m.queue, entry)
	m.reconcileShuffleLocked()
	m.mu.Unlock()
	go m.prefetchNext()
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
	go m.prefetchNext()

	return nil
}

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

// prefetchNext appends the next playable queue entry to mpv's internal
// playlist so that gapless-audio can crossfade into it. The method is
// best-effort: errors are logged but not propagated. It is safe to call
// from any goroutine; calls are idempotent.
func (m *Manager) prefetchNext() {
	m.prefetchMu.Lock()
	defer m.prefetchMu.Unlock()

	m.mu.RLock()
	idx := m.currentIndex
	queue := m.queue
	gapless := m.gaplessEnabled
	stop := m.stopAfterCurrent
	shuffle := m.shuffleEnabled
	curPrefIdx := m.prefetchedIndex
	curPrefPath := m.prefetchedPath
	curPrefID := m.prefetchedIdentity
	var upcoming []string
	if shuffle {
		upcoming = m.upcomingShuffleLocked()
	}
	m.mu.RUnlock()

	if !gapless || stop || idx < 0 {
		m.doClearPrefetch()
		return
	}

	// Resolve the successor without consuming shuffle history: under shuffle the
	// next track is the first playable identity the traversal would visit, not
	// idx+1. The resolved identity is committed to history only when the EOF
	// transition to it is confirmed (see the end-file handler).
	var nextIdx int
	var nextPath, nextID string
	if shuffle {
		nextIdx, nextPath, nextID = resolveNextShufflePrefetch(queue, upcoming)
	} else {
		nextIdx, nextPath = resolveNextSequentialPrefetch(queue, idx)
	}
	if nextIdx < 0 {
		m.doClearPrefetch()
		return
	}

	if curPrefIdx == nextIdx && curPrefPath == nextPath && curPrefID == nextID {
		return
	}

	c, err := m.ipc()
	if err != nil {
		return
	}

	if curPrefIdx >= 0 {
		// Re-check under write lock: the end-file handler may have cleared
		// prefetchedIndex between our RLock snapshot and here, in which case
		// the playlist slot 1 no longer contains our prefetched track and we
		// must not send a spurious playlist-remove.
		m.mu.Lock()
		stillPrefetched := m.prefetchedIndex >= 0
		savedIdx := m.prefetchedIndex
		savedPath := m.prefetchedPath
		savedID := m.prefetchedIdentity
		if stillPrefetched {
			m.prefetchedIndex = -1
			m.prefetchedPath = ""
			m.prefetchedIdentity = ""
		}
		m.mu.Unlock()
		if stillPrefetched {
			if _, err := c.SendCommand(cmdPlaylistRemove, 1); err != nil {
				// Cannot safely append the new track: the stale entry is still in
				// mpv's playlist. Restore tracking state so the next call can
				// retry the remove once mpv recovers.
				m.mu.Lock()
				m.prefetchedIndex = savedIdx
				m.prefetchedPath = savedPath
				m.prefetchedIdentity = savedID
				m.mu.Unlock()
				return
			}
		}
	}

	if _, err := c.SendCommand(cmdLoadfile, nextPath, "append"); err != nil {
		slog.Warn("mpv: prefetch next failed", "path", nextPath, "err", err)
		return
	}

	m.mu.Lock()
	m.prefetchedIndex = nextIdx
	m.prefetchedPath = nextPath
	m.prefetchedIdentity = nextID
	m.mu.Unlock()
}

// resolveNextSequentialPrefetch returns the index and path of the first playable
// (present-on-disk) queue entry after idx, or (-1, "") when none exists. Used by
// prefetchNext when shuffle is off.
func resolveNextSequentialPrefetch(queue []QueueTrack, idx int) (int, string) {
	for i := idx + 1; i < len(queue); i++ {
		if _, err := os.Stat(queue[i].Path); err == nil {
			return i, queue[i].Path
		}
	}
	return -1, ""
}

// resolveNextShufflePrefetch walks the upcoming shuffle identities and returns
// the index, path, and identity of the first one that both resolves to a live
// queue row and is present on disk, or (-1, "", "") when none exists. Removed
// identities (identityIndex == -1) and files missing on disk (os.ErrNotExist)
// are skipped in traversal order, mirroring advanceForward's skip behaviour
// without consuming history; the skipped-missing identities are marked at EOF
// commit time (see commitShufflePrefetchLocked).
//
// A non-not-exist stat error (e.g. a permission failure) is NOT treated as a
// skippable missing file: resolution stops so the EOF fallback plays through
// advanceForward/PlayQueueEntry, which surfaces the error unchanged instead of
// falsely marking the entry missing. This keeps gapless traversal's missing-state
// side effects equivalent to non-gapless traversal's.
func resolveNextShufflePrefetch(queue []QueueTrack, upcoming []string) (int, string, string) {
	for _, id := range upcoming {
		i := identityIndex(queue, id)
		if i < 0 {
			continue
		}
		if _, err := os.Stat(queue[i].Path); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return -1, "", ""
		}
		return i, queue[i].Path, id
	}
	return -1, "", ""
}

// clearPrefetch removes the prefetched entry from mpv's internal playlist
// and resets the tracking state. Safe to call when nothing is prefetched.
func (m *Manager) clearPrefetch() {
	m.prefetchMu.Lock()
	defer m.prefetchMu.Unlock()
	m.doClearPrefetch()
}

// doClearPrefetch is the inner implementation of clearPrefetch. The caller
// must hold prefetchMu.
func (m *Manager) doClearPrefetch() {
	// Read and clear atomically under the write lock so that the end-file
	// handler cannot race between our read and the IPC send: if it already
	// cleared prefetchedIndex to -1 we must not send a spurious playlist-remove.
	m.mu.Lock()
	savedIdx := m.prefetchedIndex
	savedPath := m.prefetchedPath
	savedID := m.prefetchedIdentity
	had := savedIdx >= 0
	if had {
		m.prefetchedIndex = -1
		m.prefetchedPath = ""
		m.prefetchedIdentity = ""
	}
	m.mu.Unlock()

	if !had {
		return
	}
	c, err := m.ipc()
	if err != nil {
		// Restore tracking state so a subsequent call can retry.
		m.mu.Lock()
		m.prefetchedIndex = savedIdx
		m.prefetchedPath = savedPath
		m.prefetchedIdentity = savedID
		m.mu.Unlock()
		return
	}
	if _, err := c.SendCommand(cmdPlaylistRemove, 1); err != nil {
		// Restore tracking state so a subsequent call can retry the remove.
		m.mu.Lock()
		m.prefetchedIndex = savedIdx
		m.prefetchedPath = savedPath
		m.prefetchedIdentity = savedID
		m.mu.Unlock()
	}
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

// SetStopAfterCurrent enables or disables "stop after current track" mode.
// When enabled, the next end-file EOF event fires OnPlaybackEnded instead of
// calling PlayFromIndex(idx+1). The flag auto-resets after firing.
func (m *Manager) SetStopAfterCurrent(v bool) {
	m.mu.Lock()
	m.stopAfterCurrent = v
	m.mu.Unlock()
	if v {
		m.clearPrefetch()
	} else {
		go m.prefetchNext()
	}
}

// StopAfterCurrent returns whether "stop after current track" mode is enabled.
func (m *Manager) StopAfterCurrent() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.stopAfterCurrent
}

// PlayQueueEntry stats the file at entry.Path before loading it into mpv.
// If the file does not exist it marks the track and queue entry as missing via
// marker, emits a transient status message, and returns ErrMissingFile without
// touching mpv — the caller should skip to the next queue entry. Other stat
// failures are returned to the caller unchanged.
// If the file is present it sends a loadfile command to mpv (queued if not yet
// connected).
func (m *Manager) PlayQueueEntry(entry QueueTrack, marker MissingMarker) error {
	if marker == nil {
		marker = noopMarker{}
	}
	if _, err := os.Stat(entry.Path); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		m.markEntryMissing(entry, marker)
		return ErrMissingFile
	}

	return m.admit(func(c *IPCClient) error {
		if _, err := c.SendCommand(cmdLoadfile, entry.Path, "replace", 0, "pause=no"); err != nil {
			return err
		}
		m.bestEffortUnpauseWith(c)
		return nil
	})
}

// markEntryMissing marks entry's track and queue row as missing through marker
// and emits the missing-file status message and callback. It is the shared side
// effect that PlayQueueEntry (Next-driven traversal) and the gapless-shuffle EOF
// commit path both invoke, so a file missing on disk produces identical durable
// markers and callbacks whether it is skipped by explicit traversal or by an
// EOF-driven gapless commit. The caller must NOT hold m.mu.
func (m *Manager) markEntryMissing(entry QueueTrack, marker MissingMarker) {
	if marker == nil {
		marker = noopMarker{}
	}
	if err := marker.MarkTrackMissing(entry.Hash); err != nil {
		slog.Error("mpv: mark track missing", "hash", entry.Hash, "err", err)
	}
	if err := marker.MarkQueueEntryMissing(entry.QueueID); err != nil {
		slog.Error("mpv: mark queue entry missing", "queueID", entry.QueueID, "err", err)
	}

	m.mu.RLock()
	fn := m.callbacks.OnStatus
	mfn := m.callbacks.OnMissingFile
	m.mu.RUnlock()
	msg := "Missing file: " + entry.Path
	if fn != nil {
		fn(msg)
	}
	if mfn != nil {
		mfn(entry.Path)
	}
	if fn == nil && mfn == nil {
		slog.Warn(msg)
	}
}
