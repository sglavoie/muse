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

// AppendToQueue appends a single entry to the in-memory queue used by
// PlayFromIndex. Call this after persisting the entry to the database so that
// the player and TUI queues stay in sync without a full SetQueue replacement.
func (m *Manager) AppendToQueue(entry QueueTrack) {
	m.mu.Lock()
	m.queue = append(m.queue, entry)
	m.mu.Unlock()
	go m.prefetchNext()
}

// RemoveFromQueue removes the queue entry at index from the in-memory queue
// and adjusts currentIndex atomically. If the removed index precedes
// currentIndex, currentIndex is decremented by one so it continues pointing
// at the same track. If the removed index equals currentIndex, currentIndex
// is left unchanged so that the next auto-advance (PlayFromIndex(currentIndex))
// resolves to what was previously index+1. Returns an error if index is
// negative or >= len(m.queue).
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
		// Clamp to the next valid entry, or last entry if removed was last
		if index >= len(m.queue) {
			m.currentIndex = len(m.queue) - 1
		}
	}
	if len(m.queue) == 0 {
		m.currentIndex = -1
	}
	m.mu.Unlock()

	// Clear the prefetch under prefetchMu so that the playlist-remove
	// cannot race with a concurrent prefetchNext appending a new entry.
	m.clearPrefetch()
	go m.prefetchNext()

	return nil
}

// PlayFromIndex starts playback from the queue entry at index. The queue must
// have been populated via SetQueue. Missing entries are skipped automatically:
// PlayFromIndex iterates forward from index until it finds a playable entry,
// calling missingFileFn for each skipped entry. Returns ErrPlaybackFailed if
// index is out of range or no playable entry exists from index onward.
func (m *Manager) PlayFromIndex(index int) error {
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
			// Clamp i against the live queue length: a concurrent
			// SetQueue / RemoveFromQueue can shrink m.queue while we
			// iterate over the local snapshot, and setting an out-of-
			// range currentIndex would leave Next/Previous and the
			// event reader pointing past the end of the live queue.
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
			go m.prefetchNext()
			return nil
		}
		if !errors.Is(err, ErrMissingFile) {
			return err
		}
	}
	return fmt.Errorf("%w: no playable entry starting at index %d", ErrPlaybackFailed, index)
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
	curPrefIdx := m.prefetchedIndex
	curPrefPath := m.prefetchedPath
	m.mu.RUnlock()

	if !gapless || stop || idx < 0 {
		m.doClearPrefetch()
		return
	}

	nextIdx := -1
	nextPath := ""
	for i := idx + 1; i < len(queue); i++ {
		if _, err := os.Stat(queue[i].Path); err == nil {
			nextIdx = i
			nextPath = queue[i].Path
			break
		}
	}
	if nextIdx < 0 {
		m.doClearPrefetch()
		return
	}

	if curPrefIdx == nextIdx && curPrefPath == nextPath {
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
		if stillPrefetched {
			m.prefetchedIndex = -1
			m.prefetchedPath = ""
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
	m.mu.Unlock()
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
	had := savedIdx >= 0
	if had {
		m.prefetchedIndex = -1
		m.prefetchedPath = ""
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
		m.mu.Unlock()
		return
	}
	if _, err := c.SendCommand(cmdPlaylistRemove, 1); err != nil {
		// Restore tracking state so a subsequent call can retry the remove.
		m.mu.Lock()
		m.prefetchedIndex = savedIdx
		m.prefetchedPath = savedPath
		m.mu.Unlock()
	}
}

// Next advances to the next track in the queue using the Go-side index so
// that the Manager (not mpv's internal playlist) owns sequencing.
// Returns ErrNotConnected if Start has not been called.
func (m *Manager) Next() error {
	m.mu.RLock()
	idx := m.currentIndex
	notStarted := m.client == nil && m.cmdQueue == nil
	m.mu.RUnlock()
	if notStarted {
		return ErrNotConnected
	}
	return m.PlayFromIndex(idx + 1)
}

// Previous returns to the previous track in the queue using the Go-side index.
// Returns ErrNotConnected if Start has not been called.
func (m *Manager) Previous() error {
	m.mu.RLock()
	idx := m.currentIndex
	notStarted := m.client == nil && m.cmdQueue == nil
	m.mu.RUnlock()
	if notStarted {
		return ErrNotConnected
	}
	return m.PlayFromIndex(idx - 1)
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
		return ErrMissingFile
	}

	c, err := m.ipcOrEnqueue(func() error { return m.PlayQueueEntry(entry, marker) })
	if err != nil || c == nil {
		return err
	}
	if _, err = c.SendCommand(cmdLoadfile, entry.Path, "replace", 0, "pause=no"); err != nil {
		return err
	}
	// loadfile with "pause=no" resets mpv's pause property; eagerly mirror
	// that state so an auto-restart firing before the property-change event
	// arrives does not resume the track paused.
	m.mu.Lock()
	m.lastPaused = false
	m.mu.Unlock()
	return nil
}
