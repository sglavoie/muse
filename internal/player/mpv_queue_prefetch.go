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
	m.playlistMu.Lock()
	defer m.playlistMu.Unlock()
	m.prefetchNextLocked()
}

// prefetchNextLocked is prefetchNext's body inside an open playlist transaction.
// Resolving the successor, removing any superseded slot, sending the append, and
// recording the tracking are one serialized operation, so a concurrent
// replacement can neither observe half of it nor leave stale tracking behind.
//
// The caller must hold m.playlistMu.
func (m *Manager) prefetchNextLocked() {
	m.mu.RLock()
	idx := m.currentIndex
	queue := m.queue
	gapless := m.gaplessEnabled
	stop := m.stopAfterCurrent
	live := m.liveness.live()
	ending := m.liveness == livenessEnding
	shuffle := m.shuffleEnabled
	completedPending := m.completedSlotPending
	cur := m.prefetchSnapshotLocked()
	var upcoming []string
	var successors []int
	if shuffle {
		upcoming = m.upcomingShuffleLocked()
	} else {
		successors = m.sequentialSuccessorsLocked(idx + 1)
	}
	m.mu.RUnlock()

	if completedPending {
		// A gapless transition has been committed but its completed slot is
		// still in mpv's playlist, so position 1 does not name the successor
		// slot yet. Appending here would put the new entry at position 2 and
		// make every later slot-1 removal address a live row. The EOF
		// maintenance re-runs this once the removal succeeds; if it failed, the
		// next EOF replaces the playlist and clears the condition.
		return
	}

	if ending {
		// An end-file has been read and the decision that follows it has not
		// resolved. mpv's playlist is mid-transition in the same way: the entry
		// that finished is still at position 0, and position 1 is either the
		// successor mpv started gaplessly — the promotion has not consumed the
		// tracking naming it yet — or nothing at all. Suppress the append as for
		// any other traversal that is not live, but leave the tracking and the
		// slot alone, because removing position 1 here would delete the row mpv
		// has just moved onto. The EOF path re-runs this once it resolves: through
		// the maintenance after a promotion, through the replacement transaction
		// after an advance.
		return
	}

	// The append below uses `append-play`, which starts an idle mpv playlist, so
	// this best-effort path is capable of starting audio. It must therefore
	// refuse to run once playback has logically ended: after a stop-after-current
	// EOF the flag above is already cleared and currentIndex still names the row
	// that finished, so nothing else here distinguishes "the user stopped" from
	// "the next track is due" — and an unrelated queue mutation (AppendToQueue,
	// SetQueue, a shuffle toggle) schedules this call. Only an explicit
	// operation, ResumeAfterQueueExtension among them, may restart a traversal
	// that ended.
	if !gapless || stop || !live || idx < 0 {
		m.removePrefetchLocked()
		return
	}

	// Resolve the successor without consuming traversal state: under shuffle the
	// next track is the first playable identity the traversal would visit, not
	// idx+1, and with shuffle off it is the first playable row of the sequential
	// continuation, which is idx+1 only while the cursor is the frontier. The
	// resolved identity is committed to history only when the EOF transition to
	// it is confirmed (see the end-file handler).
	var nextIdx int
	var nextPath, nextID string
	if shuffle {
		nextIdx, nextPath, nextID = resolveNextShufflePrefetch(queue, upcoming)
	} else {
		nextIdx, nextPath = resolveNextSequentialPrefetch(queue, successors)
	}
	if nextIdx < 0 {
		m.removePrefetchLocked()
		return
	}

	if cur.index == nextIdx && cur.path == nextPath && cur.identity == nextID {
		return
	}

	// Prefetch is best effort and must never bypass the reconnect gate: an append
	// sent while activateConnection is still restoring would land in a playlist
	// the resume loadfile is about to replace. Activation reruns the prefetch
	// once its drain completes.
	c := m.playlistClient()
	if c == nil {
		return
	}

	if !m.removePrefetchLocked() {
		// Cannot safely append the new track: the stale entry is still in mpv's
		// playlist and the tracking has been restored so a later call retries.
		return
	}

	if err := m.appendPrefetchLocked(c, nextIdx, nextPath, nextID); err != nil {
		slog.Warn("mpv: prefetch next failed", "path", nextPath, "err", err)
	}
}

// resolveNextSequentialPrefetch returns the index and path of the first playable
// (present-on-disk) row among the sequential continuation's candidates, or
// (-1, "") when none is. Used by prefetchNext when shuffle is off.
//
// It takes the candidate order rather than a starting index because the
// continuation is not always "everything after the cursor": once a resume has
// stepped back onto a row inserted behind an exhausted traversal, the rows in
// between are ones that already played, and the successors are the remaining
// insertions followed by whatever lies past the frontier. See
// sequentialSuccessorsLocked.
func resolveNextSequentialPrefetch(queue []QueueTrack, candidates []int) (int, string) {
	for _, i := range candidates {
		if i < 0 || i >= len(queue) {
			continue
		}
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
//
// Use it for queue mutations that invalidate the successor while mpv's playlist
// is otherwise untouched. A playlist replacement must NOT go through here: mpv
// has already discarded the entry, so the playlist-remove would delete a live
// row (replaceLocked drops the tracking instead).
func (m *Manager) clearPrefetch() {
	m.playlistMu.Lock()
	defer m.playlistMu.Unlock()
	m.removePrefetchLocked()
}
