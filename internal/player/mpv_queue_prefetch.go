package player

import (
	"errors"
	"log/slog"
	"os"
)

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
