package player

import (
	"errors"
	"fmt"
	"log/slog"
)

// promoteShufflePrefetch commits the shuffle successor the prefetch resolved but
// deliberately did not consume, so history advances only once the transition to
// it is confirmed. The identity is re-resolved against the live queue, which
// lets a visible reorder survive while a removed row or a changed path is
// rejected.
func (m *Manager) promoteShufflePrefetch(state eofState) (eofTransition, []QueueTrack, bool) {
	p := state.prefetch
	if p.identity == "" {
		return eofTransition{}, nil, false
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	liveIdx := identityIndex(m.queue, p.identity)
	if !m.prefetchStillLiveLocked(p) ||
		liveIdx < 0 ||
		liveIdx >= len(m.queue) ||
		m.queue[liveIdx].Path != p.path {
		return eofTransition{}, nil, false
	}

	committed, skipped := m.commitShufflePrefetchLocked(p.identity)
	if committed < 0 {
		// The traversal consumed the skipped identities before discovering the
		// target had vanished; report them so they are still marked, and let the
		// caller fall back to a fresh advance.
		return eofTransition{}, skipped, false
	}

	m.currentIndex = committed
	m.dropPrefetchTrackingLocked()
	m.completedSlotPending = true
	// Same as the sequential promotion: mpv started the successor gaplessly, so
	// the pending end resolves back into a live traversal here rather than being
	// left for the maintenance that is about to append behind it.
	m.markPlaybackLiveLocked()
	return eofTransition{
		index:    committed,
		path:     p.path,
		identity: p.identity,
		gen:      m.playlistGen,
		marker:   state.marker,
	}, skipped, true
}

// prefetchStillLiveLocked reports whether the snapshotted prefetch is still the
// tracked one and still belongs to the live playlist generation.
//
// The generation check is what stops Manager state from advancing on stale
// metadata alone: a prefetch stamped with an older generation names an entry
// that a later `loadfile ... replace` already discarded, so mpv is playing the
// replacement — or nothing — rather than the successor the tracking describes.
//
// A replacement that opened its window between the snapshot and this re-check
// is rejected for the same reason: it has committed the row it is making
// current, so promoting the successor of the row that ended would commit over
// it — and under shuffle would advance the cycle the reanchor had just reset.
// The snapshot's own check cannot cover this; only the write lock the promotion
// commits under can.
//
// The caller must hold m.mu.
func (m *Manager) prefetchStillLiveLocked(p prefetchState) bool {
	if m.replacing.active() {
		return false
	}
	return m.prefetchedIndex == p.index &&
		m.prefetchedPath == p.path &&
		m.prefetchedIdentity == p.identity &&
		m.prefetchedGen == p.gen &&
		m.prefetchedGen == m.playlistGen
}

// runEOFMaintenance is the single ordered background operation that follows a
// committed gapless transition: durable missing-file markers, the unpause, the
// removal of the slot that just finished, and the refill, in that order.
//
// Removal comes before the refill because both address mpv's playlist by slot:
// only once the completed entry is gone does mpv hold exactly the current entry,
// which is what makes the appended successor slot 1. Running them as independent
// goroutines let the append land first, so the tracking named a slot that the
// removal then shifted out from under it.
//
// A removal that fails ends the maintenance. Prefetch tracking was already
// cleared when the transition was committed, so the next EOF finds nothing to
// promote and advances through the serialized replacement primitive, which
// resets mpv's playlist to a single entry. Refilling instead would append into a
// playlist whose slot numbering no longer matches the bookkeeping, and every
// later transition would inherit the mismatch.
func (m *Manager) runEOFMaintenance(transition eofTransition, skipped []QueueTrack) {
	m.markMissingEntries(skipped, transition.marker)

	m.playlistMu.Lock()
	defer m.playlistMu.Unlock()
	m.bestEffortUnpause()
	if err := m.removeCompletedLocked(transition.gen); err != nil {
		slog.Warn("mpv: eof: completed playlist slot not removed, skipping refill",
			transition.logAttrs("playlist-remove", err)...)
		return
	}
	m.prefetchNextLocked()
}

// advanceAfterEOF runs the fallback when no prefetch could be promoted: the
// traversal-aware advance, which loads through the serialized replacement
// primitive rather than assuming anything about mpv's playlist.
//
// The advance is queued from the event reader and then waits for the playlist
// transaction lock, so a replacement can win that lock and complete before it
// runs. nextIdx and the traversal the advance would step were both measured
// against the playlist generation the EOF belonged to; once that generation is
// gone they describe a playlist mpv no longer has, and advancing anyway lands
// playback one row past whatever the replacement selected. The generation is
// therefore re-checked inside the transaction, where a replacement can no longer
// interleave.
//
// Only a genuine traversal exhaustion ends playback. An IPC drop or an mpv
// rejection leaves the queue position untouched and is logged instead: reporting
// it as OnPlaybackEnded would tell the TUI the queue ran out when it did not,
// and would reset the playback toggle behind a queue that still has tracks. The
// restart supervisor owns recovery from a broken connection.
func (m *Manager) advanceAfterEOF(nextIdx int, state eofState) {
	_, err := m.runPlaylistTxnIndex(func() (int, error) { return m.advanceAfterEOFLocked(nextIdx, state) })
	switch {
	case err == nil:
		// The load landed, so replaceLocked has already marked the traversal live
		// again: the pending end resolved into a successor.
		return
	case errors.Is(err, errEOFSuperseded):
		// Nothing here will decide the end now. A queue-owned replacement that won
		// the race marked the traversal live for the row it selected, which
		// settlePlaybackEnding leaves alone; an mpv restart that opened the new
		// generation loaded nothing, and the pending end becomes the end.
		m.settlePlaybackEnding()
		slog.Debug("mpv: eof: dropped an auto-advance whose playlist generation was replaced",
			state.logAttrs("advance-after-current", err)...)
	case errors.Is(err, errStartedSuccessorUnsettled):
		// mpv may still be playing the successor that no longer belongs to the
		// traversal. Keep the EOF settlement pending and make the restart
		// supervisor own the old process. An exhausted traversal's report is
		// emitted only after that process has actually been reaped; a selection
		// queued during the outage can supersede it by marking playback live first.
		m.handoffPendingEOFSettlement(err)
		slog.Warn("mpv: eof: invalid started successor survived bounded discard; restarting mpv",
			state.logAttrs("discard-started-successor", err)...)
	case errors.Is(err, errTraversalExhausted):
		m.emitPlaybackEnded()
	default:
		// The queue was not exhausted, but no successor reached mpv either, so the
		// entry that finished is still the last thing it played. Settling keeps
		// best-effort work off an idle playlist; the queue position is untouched,
		// so the user's next explicit operation resumes from where it was.
		m.settlePlaybackEnding()
		slog.Warn("mpv: eof: auto-advance failed without exhausting the queue",
			state.logAttrs("advance-after-current", err)...)
	}
}

// advanceAfterEOFLocked is advanceAfterEOF's transaction body: the generation
// re-check, the advance itself, and the settlement of whatever mpv is left
// holding when the advance loads nothing.
//
// Every path out of this function that does not load leaves mpv's playlist as the
// single entry that finished. That is already true when the successor was still
// parked, and the discard is what makes it true in the one case where it is not:
// a mid-transition queue mutation invalidated the row mpv had started, so the
// promotion rejected it and the advance behind it has nothing to overwrite it
// with. Discarding before the end is recorded is the ordering FR-008 turns on —
// exhaustion may not be reported while mpv is still playing a row the traversal
// has declined.
//
// The caller must hold m.playlistMu and must not hold m.mu.
func (m *Manager) advanceAfterEOFLocked(nextIdx int, state eofState) (int, error) {
	m.mu.RLock()
	gen := m.playlistGen
	m.mu.RUnlock()
	if gen != state.playlistGen {
		return -1, errEOFSuperseded
	}
	idx, err := m.advanceAfterCurrentLocked(nextIdx)
	if err == nil {
		return idx, nil
	}
	exhausted := errors.Is(err, errTraversalExhausted)
	startedSuccessor, prepareErr := m.prepareStartedSuccessorSettlement(state.playlistGen, exhausted)
	if prepareErr != nil {
		return idx, prepareErr
	}
	if !startedSuccessor {
		// Isolated Managers with neither an IPC client nor a restart channel do
		// not own a process whose audio can still be running. Preserve their
		// ordinary exhaustion path; a production Manager always has the channel,
		// even when recovery has already detached its client.
		if exhausted {
			m.markPlaybackEnded()
		}
		return idx, err
	}
	if discardErr := m.discardStartedSuccessorWithRetryLocked(); discardErr != nil {
		return idx, fmt.Errorf("%w: %w", errStartedSuccessorUnsettled, discardErr)
	}
	if !m.resolvePendingEOFSettlementAfterDiscard(exhausted) {
		// Recovery already consumed the settlement after stopping this process,
		// or an explicit operation made the traversal live. In either case this
		// transaction has no end left to resolve or emit.
		return idx, errEOFSuperseded
	}
	return idx, err
}

// prepareStartedSuccessorSettlement re-checks the process generation after the
// traversal attempt and publishes the pending settlement before its first IPC.
// A transport failure can be observed by the health check as soon as
// SendCommand releases the client lock, so recovery must see this state even if
// it outruns the EOF goroutine after that return.
func (m *Manager) prepareStartedSuccessorSettlement(gen uint64, exhausted bool) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.playlistGen != gen {
		return false, errEOFSuperseded
	}
	started := m.startedSuccessorLocked(m.prefetchSnapshotLocked()) &&
		(m.client != nil || m.disconnectCh != nil)
	if started {
		m.pendingEOFSettlement = true
		m.pendingExhaustionReport = exhausted
	}
	return started, nil
}

// discardStartedSuccessorWithRetryLocked gives a transient mpv rejection one
// bounded retry before escalating to process recovery. Each failed attempt
// restores the same tracking, so the next attempt can identify the slot again;
// no attempt resolves the pending EOF unless the removal actually lands.
//
// The caller must hold m.playlistMu and must not hold m.mu.
func (m *Manager) discardStartedSuccessorWithRetryLocked() error {
	var err error
	for range startedSuccessorDiscardAttempts {
		err = m.discardStartedSuccessorLocked()
		if err == nil {
			return nil
		}
	}
	return err
}

// handoffPendingEOFSettlement routes the installed generation through the same
// guarded disconnect transition used by the health check and event reader. The
// pending state was published before the failed IPC attempt, so recovery cannot
// reap the process ahead of the settlement it must consume.
func (m *Manager) handoffPendingEOFSettlement(err error) {
	m.mu.Lock()
	if !m.pendingEOFSettlement || m.liveness != livenessEnding {
		// A replacement or an already-running recovery settled the successor
		// before this goroutine reached the handoff. Retire any stale flags and
		// leave the live/new generation alone.
		m.pendingEOFSettlement = false
		m.pendingExhaustionReport = false
		m.mu.Unlock()
		return
	}
	ctx := m.ctx
	client := m.client
	m.mu.Unlock()
	if client != nil && ctx != nil {
		m.handleConnectionFailure(ctx, client, err)
	}
}

// markMissingEntries marks each entry missing on disk. The caller must not hold
// m.mu: marking writes to the database and fires TUI callbacks.
func (m *Manager) markMissingEntries(entries []QueueTrack, marker MissingMarker) {
	for _, entry := range entries {
		m.markEntryMissing(entry, marker)
	}
}
