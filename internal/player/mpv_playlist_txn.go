package player

import (
	"errors"
	"fmt"
	"os"

	"github.com/sglavoie/muse/internal/safego"
)

// This file holds the playlist transaction layer: the single boundary through
// which every queue-owned mutation of mpv's internal playlist passes.
//
// A playlist transaction holds m.playlistMu for the whole operation, so mpv's
// playlist and the Go bookkeeping that mirrors it (currentIndex, shuffle
// traversal, prefetch tracking, playlistGen) are always updated together. That
// makes the two mutations that used to race — an explicit `loadfile ... replace`
// and a best-effort prefetch append — strictly serial, so Go can no longer
// retain a successor that a replacement already removed from mpv's playlist.
//
// Lock ordering: playlistMu is acquired BEFORE m.mu, never the other way round.
// Helpers whose name ends in Locked expect playlistMu to already be held and
// take m.mu themselves; they must not be called with m.mu held.
//
// Only isolated playback stays outside this layer: LoadFilePaused and loadFileAt
// (session restore and post-restart resume, which reinstate the already-current
// row rather than traverse the queue) and the preview player (a separate mpv
// process with no queue at all).

// prefetchState is the tracked "next track" as a single value so a transaction
// can snapshot it, clear it, and restore it without touching four fields by hand.
type prefetchState struct {
	index    int
	path     string
	identity string
	gen      uint64
}

// replacementState is the correlation the end-file/eof decision needs: it names
// the queue-owned playlist replacements that have begun but that mpv has not
// confirmed yet.
//
// playlistGen alone cannot supply that correlation. A replacement publishes the
// index (or the reanchored shuffle cycle) it is about to make current *before*
// the loadfile is sent, and advances playlistGen only after mpv accepts it, so
// throughout the send window every generation-stamped fact still describes the
// entry being discarded while currentIndex and the traversal already describe
// its replacement. An EOF decided from that mixture reads the new target with
// the old prefetch: it rejects the promotion and then queues a fallback advance
// measured from the row the user just selected, which lands playback one row
// past the selection — or, under shuffle, grafts the old successor onto the
// freshly reanchored cycle.
//
// depth is a nesting count rather than a flag because the marker is taken at two
// levels: a playlist transaction marks its whole body, since the traversal step
// inside it is where the new state first becomes visible, and replaceLocked
// marks its own load for the callers that reach it without a transaction wrapper
// (PlayQueueEntry, and the direct calls in tests).
type replacementState struct {
	depth int
	// path is the file the most recent replacement in this window is loading,
	// and gen the playlist generation it supersedes. Both are diagnostics only.
	path string
	gen  uint64
}

// active reports whether a queue-owned replacement is in flight.
func (r replacementState) active() bool { return r.depth > 0 }

// beginReplacementLocked opens a replacement window. Pass the file being loaded
// when it is known, or "" from a transaction wrapper that has not resolved a
// candidate yet.
//
// The caller must hold m.mu.
func (m *Manager) beginReplacementLocked(path string) {
	if m.replacing.depth == 0 {
		m.replacing.gen = m.playlistGen
	}
	if path != "" {
		m.replacing.path = path
	}
	m.replacing.depth++
}

// endReplacementLocked closes one replacement window opened by
// beginReplacementLocked.
//
// The caller must hold m.mu.
func (m *Manager) endReplacementLocked() {
	m.replacing.depth--
	if m.replacing.depth <= 0 {
		m.replacing = replacementState{}
	}
}

// beginReplacement is beginReplacementLocked for a caller that does not hold m.mu.
func (m *Manager) beginReplacement(path string) {
	m.mu.Lock()
	m.beginReplacementLocked(path)
	m.mu.Unlock()
}

// endReplacement is endReplacementLocked for a caller that does not hold m.mu.
func (m *Manager) endReplacement() {
	m.mu.Lock()
	m.endReplacementLocked()
	m.mu.Unlock()
}

// prefetchSnapshotLocked copies the current prefetch tracking.
//
// The caller must hold m.mu.
func (m *Manager) prefetchSnapshotLocked() prefetchState {
	return prefetchState{
		index:    m.prefetchedIndex,
		path:     m.prefetchedPath,
		identity: m.prefetchedIdentity,
		gen:      m.prefetchedGen,
	}
}

// dropPrefetchTrackingLocked forgets the tracked prefetch without touching mpv's
// playlist. Use it when mpv has already discarded the entry on its own (a
// successful `replace` swaps the whole playlist) — sending a playlist-remove in
// that case would delete a live row instead.
//
// The caller must hold m.mu.
func (m *Manager) dropPrefetchTrackingLocked() {
	m.prefetchedIndex = -1
	m.prefetchedPath = ""
	m.prefetchedIdentity = ""
	m.prefetchedGen = 0
}

// restorePrefetchTracking reinstates a snapshot taken before a removal that
// could not be completed, so a later transaction retries the removal instead of
// leaking mpv's stale playlist slot.
func (m *Manager) restorePrefetchTracking(s prefetchState) {
	m.mu.Lock()
	m.prefetchedIndex = s.index
	m.prefetchedPath = s.path
	m.prefetchedIdentity = s.identity
	m.prefetchedGen = s.gen
	m.mu.Unlock()
}

// setCurrentIndexLocked records i as the current queue index, clamped against the
// live queue length. A concurrent SetQueue / RemoveFromQueue can shrink m.queue
// while a caller iterates over a snapshot, and an out-of-range currentIndex would
// leave Next/Previous and the event reader pointing past the end of the queue.
//
// The caller must hold m.mu.
func (m *Manager) setCurrentIndexLocked(i int) {
	switch {
	case len(m.queue) == 0:
		m.currentIndex = -1
	case i >= len(m.queue):
		m.currentIndex = len(m.queue) - 1
	default:
		m.currentIndex = i
	}
}

// commitIndexLocked publishes index as the current queue index and returns the
// undo that restores the previous one if the load that follows fails.
//
// The caller must hold m.mu.
func (m *Manager) commitIndexLocked(index int) func() {
	prev := m.currentIndex
	m.setCurrentIndexLocked(index)
	return func() { m.currentIndex = prev }
}

// playlistClient returns the live IPC client for a playlist mutation, or nil when
// the manager is disconnected or still replaying the reconnect backlog.
//
// Best-effort playlist work must not bypass the activation gate the way the
// ungated ipc() helper does: an append landing mid-restore would be wiped by the
// resume loadfile that activateConnection is about to send, leaving Go tracking a
// successor mpv no longer has. activateConnection rebuilds the prefetch once the
// drain completes.
func (m *Manager) playlistClient() *IPCClient {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.activating {
		return nil
	}
	return m.client
}

// replaceLocked runs one queue-owned playlist replacement inside the transaction
// the caller has already opened.
//
// commit, when non-nil, publishes the traversal state this replacement is about
// to make current. It runs under m.mu *before* the loadfile is sent, so a
// start-file event that arrives while the load is still in flight resolves the
// intended row instead of the one being replaced; it returns the undo applied
// when the synchronous load fails.
//
// A missing file is reported as ErrMissingFile after marking it, without touching
// mpv or the traversal state, so the caller can skip to the next candidate. Any
// other stat error is returned unchanged.
//
// The caller must hold m.playlistMu and must not hold m.mu.
func (m *Manager) replaceLocked(entry QueueTrack, marker MissingMarker, commit func() func()) error {
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

	// Publish the traversal state and open the replacement window in the same
	// critical section. From here until mpv confirms the load, an end-file/eof
	// belongs to the entry this replacement is discarding, so the EOF decision
	// must not read the state committed just above as if it described what
	// ended.
	var undo func()
	m.mu.Lock()
	if commit != nil {
		undo = commit()
	}
	m.beginReplacementLocked(entry.Path)
	m.mu.Unlock()

	err := m.admitPlaylist(func(c *IPCClient) error {
		if _, err := c.SendCommand(cmdLoadfile, entry.Path, "replace", 0, "pause=no"); err != nil {
			return err
		}
		m.bestEffortUnpauseWith(c)
		return nil
	})
	if err != nil {
		m.mu.Lock()
		if undo != nil {
			undo()
		}
		m.endReplacementLocked()
		m.mu.Unlock()
		return err
	}

	// mpv accepted the replacement, so its playlist now holds exactly this file:
	// a successful replace is authoritative prefetch invalidation. Forget the
	// tracking without a playlist-remove — slot 1 no longer holds the prefetched
	// entry, and removing it would delete a live row.
	m.mu.Lock()
	if commit != nil {
		// Re-assert the index. The EOF handler makes its promotion under m.mu
		// alone, by design — it must not wait on the playlist transaction — so
		// the promotion runs on a goroutine this transaction never joins. The
		// replacement window above is what stops it from committing over this
		// load; re-asserting here keeps the index correct even if a promotion
		// slipped through before the window opened.
		commit()
	}
	m.playlistGen++
	m.dropPrefetchTrackingLocked()
	// The replacement also swept away any completed entry a failed EOF cleanup
	// left behind, so slot addressing is meaningful again.
	m.completedSlotPending = false
	m.endReplacementLocked()
	m.mu.Unlock()
	return nil
}

// appendPrefetchLocked appends path to mpv's playlist as the gapless successor
// and records the tracking only once mpv has accepted the command, stamped with
// the playlist generation it landed in.
//
// The append uses `append-play` rather than plain `append` so that a playlist
// which has gone idle (mpv reached the end of its list before Go noticed) starts
// playing the appended entry instead of silently holding it, which is the stall
// the plain append leaves behind.
//
// The caller must hold m.playlistMu and must not hold m.mu.
func (m *Manager) appendPrefetchLocked(c *IPCClient, index int, path, identity string) error {
	if _, err := c.SendCommand(cmdLoadfile, path, "append-play"); err != nil {
		return err
	}
	m.mu.Lock()
	m.prefetchedIndex = index
	m.prefetchedPath = path
	m.prefetchedIdentity = identity
	m.prefetchedGen = m.playlistGen
	m.mu.Unlock()
	return nil
}

// removePrefetchLocked drops the tracked prefetch from mpv's playlist (position
// 1) and from the Go bookkeeping in the same serialized step. It reports whether
// mpv's playlist is now known to be free of the prefetched entry: false means the
// removal could not be completed and the tracking was restored so a later
// transaction retries it. Removing nothing succeeds.
//
// The caller must hold m.playlistMu and must not hold m.mu.
func (m *Manager) removePrefetchLocked() bool {
	// Read and clear atomically under the write lock so the end-file handler
	// cannot race between the read and the IPC send: if it already cleared
	// prefetchedIndex to -1 we must not send a spurious playlist-remove.
	m.mu.Lock()
	saved := m.prefetchSnapshotLocked()
	had := saved.index >= 0
	if had {
		m.dropPrefetchTrackingLocked()
	}
	m.mu.Unlock()

	if !had {
		return true
	}

	c := m.playlistClient()
	if c == nil {
		m.restorePrefetchTracking(saved)
		return false
	}
	if _, err := c.SendCommand(cmdPlaylistRemove, 1); err != nil {
		m.restorePrefetchTracking(saved)
		return false
	}
	return true
}

// removeCompletedLocked drops the track that just finished (playlist position 0)
// after a gapless transition, so mpv's playlist stays at two entries instead of
// growing across transitions.
//
// gen is the playlist generation the transition being cleaned up belongs to.
// Removing position 0 is only meaningful while that generation is still live: a
// replacement that landed in between swept the completed entry away and put a
// live row at position 0, so the removal would stop playback. A superseded
// generation, or a slot another cleanup already removed, is reported as success
// — there is nothing left to remove.
//
// The error is reported rather than swallowed because the caller's next step
// depends on it: while the completed entry is still there, mpv's slot numbering
// no longer matches the bookkeeping, so appending a successor on top would
// compound the mismatch instead of correcting it.
//
// The caller must hold m.playlistMu and must not hold m.mu.
func (m *Manager) removeCompletedLocked(gen uint64) error {
	m.mu.RLock()
	stale := m.playlistGen != gen || !m.completedSlotPending
	m.mu.RUnlock()
	if stale {
		return nil
	}

	c := m.playlistClient()
	if c == nil {
		return fmt.Errorf("%w: no playlist client for the completed-slot removal", ErrNotConnected)
	}
	if _, err := c.SendCommand(cmdPlaylistRemove, 0); err != nil {
		return err
	}
	m.mu.Lock()
	m.completedSlotPending = false
	m.mu.Unlock()
	return nil
}

// runPlaylistTxn executes fn as a playlist transaction and, when fn succeeds,
// schedules a fresh prefetch once the lock has been released — the successor is
// resolved against the traversal state fn just committed.
func (m *Manager) runPlaylistTxn(fn func() error) error {
	_, err := m.runPlaylistTxnIndex(func() (int, error) { return -1, fn() })
	return err
}

// runPlaylistTxnIndex is runPlaylistTxn for a transaction body that reports the
// queue index it played. The index is the row playback actually landed on, which
// is not derivable from the request: sequential traversal skips entries missing
// on disk and shuffle traversal follows identities rather than positions. A
// failed transaction reports -1.
//
// The whole body runs inside a replacement window, not just the loadfile it
// eventually sends. Every body routed through here replaces mpv's playlist, and
// each one mutates the traversal before it reaches replaceLocked — the shuffle
// reanchor, the provisional forward step, the history rewind. That mutation is
// already visible to an EOF landing on the event reader, so the window has to
// start with the body rather than with the load.
func (m *Manager) runPlaylistTxnIndex(fn func() (int, error)) (int, error) {
	m.playlistMu.Lock()
	m.beginReplacement("")
	idx, err := fn()
	m.endReplacement()
	m.playlistMu.Unlock()
	if err != nil {
		return -1, err
	}
	safego.Go("player.prefetchNext", m.prefetchNext)
	return idx, nil
}
