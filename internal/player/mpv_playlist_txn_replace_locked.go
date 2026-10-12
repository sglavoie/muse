package player

import (
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/sglavoie/muse/internal/safego"
)

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
	// Claim the start-file this load will produce before sending it. mpv can
	// emit that event before the command's own reply comes back, so recording
	// the expectation afterwards would let the reader correlate it against
	// whatever generation is live by then.
	var undo func()
	m.mu.Lock()
	if commit != nil {
		undo = commit()
	}
	m.beginReplacementLocked(entry.Path)
	pendingSeq := m.expectLoadStartLocked(entry)
	pendingGen := m.loadGen
	m.mu.Unlock()

	err := m.admitPlaylist(func(c *IPCClient) error {
		if _, err := c.SendCommand(cmdLoadfile, entry.Path, "replace", 0, "pause=no"); err != nil {
			return err
		}
		// mpv has executed the load, so the start-file this expectation is
		// waiting for has already been emitted. Recording that here rather than
		// on the success path below is what makes the boundary describe the
		// event: a send deferred to the reconnect queue is marked when the drain
		// finally performs it, and a send still parked inside mpv is not marked
		// at all, so a reader connecting meanwhile keeps its expectation.
		m.noteLoadStartSent(pendingSeq)
		m.bestEffortUnpauseWith(c)
		return nil
	})
	if err != nil {
		m.mu.Lock()
		if undo != nil {
			undo()
		}
		m.cancelLoadStartLocked(pendingGen)
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
	// The load landed, so the generation this window opened is the one mpv is
	// entering: it stands even though nothing has reported the new file
	// starting yet, which is exactly the interval an old buffered end-file can
	// still be read in.
	m.replacing.loaded = true
	m.dropPrefetchTrackingLocked()
	// The replacement also swept away any completed entry a failed EOF cleanup
	// left behind, so slot addressing is meaningful again.
	m.completedSlotPending = false
	// Every queue-owned load an explicit operation performs passes through here,
	// so this is the one place a traversal that stop-after-current or exhaustion
	// ended becomes live again. Marking it only after mpv accepted the load
	// keeps the state describing mpv rather than the intent.
	m.markPlaybackLiveLocked()
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
	m.prefetchedLoadGen = m.loadGen
	if m.startedLoadGen != 0 && m.loadGen == m.startedLoadGen {
		// The successor went in behind the entry the event reader has
		// correlated, so mpv will start it on its own when that entry ends.
		// An append made in any other generation belongs to a load whose own
		// start-file has not been read yet; resyncGaplessSuccessorsLocked
		// picks it up when it is.
		m.gaplessSuccessors++
	}
	m.mu.Unlock()
	return nil
}

// startedSuccessorLocked reports whether the tracked prefetch p is the entry mpv
// has already entered rather than one still parked behind the current file.
//
// The two are the same slot and differ only in time. mpv owns its playlist: when
// the entry that was playing ends, it moves straight into the appended successor
// at position 1 and only then does the Manager get to decide what that means.
// livenessEnding is exactly that interval — an end-file has been read and the
// transition that promotes the successor onto the bookkeeping has not resolved —
// so while it stands, the tracking names the row that is playing and a
// playlist-remove aimed at position 1 would stop it.
//
// prefetchNextLocked reaches the same conclusion from the other side and leaves
// the slot alone for the same reason; this is what extends that rule to the
// direct invalidators, which reach the slot through clearPrefetch instead. The
// interval is bounded: every branch of the EOF dispatch resolves it, and the
// promotion and the fallback advance both re-run the prefetch afterwards, which
// is where an invalidation that arrived mid-transition is finally applied.
//
// The caller must hold m.mu.
func (m *Manager) startedSuccessorLocked(p prefetchState) bool {
	return p.index >= 0 && m.liveness == livenessEnding
}

// removePrefetchLocked drops the tracked prefetch from mpv's playlist (position
// 1) and from the Go bookkeeping in the same serialized step. It reports whether
// mpv's playlist is now known to be free of the prefetched entry: false means the
// removal could not be completed and the tracking was restored — or was never
// given up — so a later transaction retries it. Removing nothing succeeds.
//
// The caller must hold m.playlistMu and must not hold m.mu.
func (m *Manager) removePrefetchLocked() bool {
	// Read and clear atomically under the write lock so the end-file handler
	// cannot race between the read and the IPC send: if it already cleared
	// prefetchedIndex to -1 we must not send a spurious playlist-remove.
	m.mu.Lock()
	saved := m.prefetchSnapshotLocked()
	had := saved.index >= 0
	started := m.startedSuccessorLocked(saved)
	if had && !started {
		m.dropPrefetchTrackingLocked()
	}
	m.mu.Unlock()

	if started {
		// Position 1 is not a parked successor: mpv has already moved into it.
		// Report the removal as incomplete rather than performing it — the entry
		// is still in the playlist, and the EOF transition that owns it re-runs
		// the prefetch once it resolves, which is where a genuine invalidation
		// takes effect.
		return false
	}
	if !had {
		return true
	}
	return m.sendPrefetchRemovalLocked(saved) == nil
}

// discardStartedSuccessorLocked takes position 1 away from mpv even though mpv is
// playing it, and returns an error when the slot is not known to be gone. It is a
// no-op unless the tracked prefetch is a successor mpv has already entered.
//
// This is the opposite half of removePrefetchLocked's guard, and only one caller
// is entitled to it: the EOF fallback advance, once it has established that the
// transition resolves into no successor at all. Usually the transition adopts the
// started row (the promotion) or overwrites it (the advance's own replacement),
// and both leave mpv playing something the bookkeeping agrees with. What is left
// is the case a mid-transition queue mutation opens — the tracked row was removed
// or replaced between the EOF snapshot and the promotion, the promotion rejected
// it, and the advance behind it found nothing to load. mpv is then playing a row
// the traversal has just declined, and recording the end while it plays would
// report an exhausted queue over audible playback, with the tracking still naming
// the slot so the next best-effort cleanup would stop it as an afterthought.
//
// Removing it settles that: mpv is left holding only the entry that finished,
// which is the same shape an ordinary exhaustion leaves behind.
//
// The caller must hold m.playlistMu and must not hold m.mu.
func (m *Manager) discardStartedSuccessorLocked() error {
	m.mu.Lock()
	saved := m.prefetchSnapshotLocked()
	started := m.startedSuccessorLocked(saved)
	if started {
		m.dropPrefetchTrackingLocked()
	}
	m.mu.Unlock()

	if !started {
		return nil
	}
	if err := m.sendPrefetchRemovalLocked(saved); err == nil {
		return nil
	} else {
		// The send restored the tracking, which keeps this end recoverable: a
		// bounded retry can identify the same slot, and process recovery can drop
		// it wholesale without losing the pending end report.
		slog.Warn("mpv: eof: the successor mpv started could not be discarded",
			"path", saved.path, "index", saved.index, "err", err)
		return err
	}
}

// sendPrefetchRemovalLocked performs the removal half shared by the prefetch
// invalidators: it drops playlist position 1 in mpv and reconciles the gapless
// accounting, restoring the caller's saved tracking if the command does not land.
//
// The caller must hold m.playlistMu, must not hold m.mu, and must already have
// given up the tracking that saved describes.
func (m *Manager) sendPrefetchRemovalLocked(saved prefetchState) error {
	c := m.playlistClient()
	if c == nil {
		m.restorePrefetchTracking(saved)
		return fmt.Errorf("%w: no playlist client for prefetch removal", ErrNotConnected)
	}
	if _, err := c.SendCommand(cmdPlaylistRemove, 1); err != nil {
		m.restorePrefetchTracking(saved)
		return err
	}
	// mpv no longer has the entry, so it can no longer start it on its own. A
	// prefetch from another generation was never counted against the entry the
	// reader has correlated, so there is nothing to give back for it.
	m.mu.Lock()
	if m.startedLoadGen != 0 && saved.loadGen == m.startedLoadGen && m.gaplessSuccessors > 0 {
		m.gaplessSuccessors--
	}
	m.mu.Unlock()
	return nil
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
