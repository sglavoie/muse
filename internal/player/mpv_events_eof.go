package player

import (
	"errors"
	"log/slog"

	"github.com/sglavoie/muse/internal/safego"
)

// This file holds the end-file/eof transition: the single place where a track
// finishing turns into the next one starting.
//
// The transition is split in two on purpose. The decision — is playback
// stopping here, can the tracked prefetch be promoted, or must the queue be
// traversed again — is a short critical section over Manager state with no IPC
// and no database work, so it runs on the event-reader goroutine and cannot
// delay the next event. Everything that can block runs afterwards as one
// ordered background operation, so mpv's playlist is never mutated by two
// independent goroutines racing over the same transition.

// eofState is the snapshot the event reader takes when mpv reports end-file/eof.
// It is a snapshot, not a claim: every field is re-validated under the write
// lock before anything is committed, because a queue mutation can land between
// the snapshot and the commit.
type eofState struct {
	currentIndex   int
	queueLen       int
	shuffle        bool
	stopAfter      bool
	prefetch       prefetchState
	playlistGen    uint64
	replacing      replacementState
	loadGen        uint64
	startedLoadGen uint64
	marker         MissingMarker
	// gaplessStart records that mpv had an appended successor waiting behind
	// the entry that ended, so the start-file the reader reads next is that
	// same entry's generation continuing rather than a newer load arriving.
	// It is decided here because only this goroutine is still ordered against
	// the socket; by the time the start-file is dispatched, a replacement may
	// have come and gone.
	gaplessStart bool
}

// superseded reports whether the entry that emitted this end-file had already
// been discarded by a queue-owned replacement.
//
// The two halves cover the same window from opposite sides. An active
// replacement has published the row (or the reanchored cycle) it is making
// current but is still waiting on mpv, so what ended is the entry it is
// discarding. A load generation that no longer matches the one mpv last
// reported a file starting in says the same thing after the fact: the
// replacement has been acknowledged and its window is closed, and this
// end-file was emitted by an entry from before it — buffered on the event
// socket while the replacement went through, and only read now.
//
// The generation half is disarmed while startedLoadGen is zero: nothing has
// ever identified which generation mpv is playing in, so no end-file can be
// attributed to a superseded one.
func (s eofState) superseded() bool {
	if s.replacing.active() {
		return true
	}
	return s.startedLoadGen != 0 && s.startedLoadGen != s.loadGen
}

// supersededAttrs returns the structured context for an EOF that was discarded
// because a queue-owned replacement had already claimed the transition.
func (s eofState) supersededAttrs() []any {
	return []any{
		"op", "superseded-eof",
		"playlistGen", s.replacing.gen,
		"prefetchedGen", s.prefetch.gen,
		"loadGen", s.loadGen,
		"startedLoadGen", s.startedLoadGen,
		"replacementInFlight", s.replacing.active(),
		"queueIndex", s.currentIndex,
		"queueLen", s.queueLen,
		"replacingPath", s.replacing.path,
	}
}

// logAttrs returns the structured context an EOF warning carries when no
// transition was committed: the live playlist generation, the queue position
// playback ended on, and the successor the prefetch had resolved.
func (s eofState) logAttrs(op string, err error) []any {
	return []any{
		"op", op,
		"playlistGen", s.playlistGen,
		"prefetchedGen", s.prefetch.gen,
		"queueIndex", s.currentIndex,
		"queueLen", s.queueLen,
		"identity", s.prefetch.identity,
		"path", s.prefetch.path,
		"err", err,
	}
}

// eofTransition describes a committed gapless transition: mpv already started
// the prefetched successor on its own, and the Manager bookkeeping has been
// moved onto it.
type eofTransition struct {
	index    int
	path     string
	identity string
	gen      uint64
	marker   MissingMarker
}

// logAttrs returns the structured context an EOF warning carries once the
// transition is committed: the playlist generation it belongs to, the queue
// identity and index it promoted, the file mpv is now playing, and the
// operation that failed.
func (t eofTransition) logAttrs(op string, err error) []any {
	return []any{
		"op", op,
		"playlistGen", t.gen,
		"queueIndex", t.index,
		"identity", t.identity,
		"path", t.path,
		"err", err,
	}
}

// handleEndOfFile runs the end-file/eof decision on the event-reader goroutine
// and hands every operation that can block to the background.
//
// It reports whether mpv is moving into a successor that was already in its
// playlist, which is what tells the reader how to correlate the start-file it
// reads next. That answer holds whatever the decision below turns out to be:
// it describes mpv's playlist at the moment the entry ended, not what the
// Manager chooses to do about it.
func (m *Manager) handleEndOfFile() bool {
	state := m.snapshotEOF()
	m.dispatchEOF(state)
	return state.gaplessStart
}

// dispatchEOF acts on the snapshot handleEndOfFile took.
func (m *Manager) dispatchEOF(state eofState) {
	if state.superseded() {
		// A queue-owned replacement has claimed this transition: it has already
		// published the row (or the reanchored cycle) it is making current, so
		// what just ended is the entry it is discarding — whether the loadfile
		// is still in flight or mpv answered it while this event waited on the
		// socket. Deciding anything from here would measure the successor from
		// the new selection instead of from the row that finished. The
		// replacement itself starts playback and its transaction refills the
		// prefetch, so there is nothing left for this EOF to do.
		slog.Debug("mpv: eof: discarded an end-file from a superseded playlist entry",
			state.supersededAttrs()...)
		return
	}
	if state.stopAfter {
		m.emitPlaybackEnded()
		return
	}

	transition, skipped, advanced := m.promotePrefetch(state)
	if advanced {
		safego.Go("player.eofMaintenance", func() {
			m.runEOFMaintenance(transition, skipped)
		})
		return
	}

	// No promotable prefetch: fall back to the traversal-aware advance, which
	// reaches mpv through the serialized replacement primitive.
	//
	// The advance owns the decision, but it runs on a background goroutine behind
	// the playlist transaction lock, and whether the queue has a successor left is
	// exactly what it is going to find out. Record the pending end here, on the
	// reader, before anything can block: from this instant mpv has played the
	// entry out and no successor has been loaded, so until the transaction reports
	// back the traversal must not read as live. An event-socket failure landing in
	// that window is the case this closes — the restart supervisor snapshots
	// liveness the moment recovery begins, and a snapshot taken while the
	// exhaustion is still queued would resume the track that just finished.
	nextIdx := state.currentIndex + 1
	m.markPlaybackEnding(state.playlistGen)
	safego.Go("player.advanceAfterEOF", func() {
		m.markMissingEntries(skipped, state.marker)
		m.advanceAfterEOF(nextIdx, state)
	})
}

// snapshotEOF captures the state the EOF decision is made from and consumes the
// stop-after-current flag, which must fire exactly once.
//
// A superseded EOF consumes nothing: stop-after-current is armed against the
// track the user is listening to, and the entry a replacement is discarding is
// no longer that track, so spending the flag here would make it fire silently
// against a row that never finished.
func (m *Manager) snapshotEOF() eofState {
	m.mu.Lock()
	defer m.mu.Unlock()
	state := eofState{
		currentIndex:   m.currentIndex,
		queueLen:       len(m.queue),
		shuffle:        m.shuffleEnabled,
		stopAfter:      m.stopAfterCurrent,
		prefetch:       m.prefetchSnapshotLocked(),
		playlistGen:    m.playlistGen,
		replacing:      m.replacing,
		loadGen:        m.loadGen,
		startedLoadGen: m.startedLoadGen,
		marker:         m.marker,
	}
	// Spend the successor expectation whatever the decision below is: mpv
	// starts the appended entry as it emits this end-file, so the next
	// start-file is accounted for either way. A superseded EOF is no exception
	// — the entry it belongs to was still leading mpv's playlist when it ended,
	// and the successor behind it starts all the same.
	state.gaplessStart = m.takeGaplessSuccessorLocked()
	if state.superseded() {
		return state
	}
	if state.stopAfter {
		// The flag fires exactly once, and the idle mpv is left in is
		// intentional. Both are recorded in this one critical section so a
		// best-effort prefetch can never observe the cleared flag without also
		// observing that playback has ended — the window in which it would see
		// a stop-after-current EOF as an ordinary "next track is due".
		m.stopAfterCurrent = false
		m.markPlaybackEndedLocked()
	}
	return state
}

// promotePrefetch commits the prefetched successor as the current track when it
// still describes both the live queue and the live playlist. mpv has already
// started that entry gaplessly, so this only moves the Go bookkeeping onto it —
// no replacement is sent, which is what keeps the transition gapless.
//
// Committing also records that mpv's playlist still leads with the entry that
// just finished (completedSlotPending), which suppresses every append until the
// maintenance below removes it — that is what makes "remove before append" hold
// against the independent safety-net prefetch the start-file event schedules.
//
// It reports whether the promotion happened. The returned entries are queue rows
// a shuffle prefetch peek stepped over because their files are missing on disk;
// they must be marked outside the lock whether or not the promotion succeeded,
// since the traversal has already consumed them.
func (m *Manager) promotePrefetch(state eofState) (eofTransition, []QueueTrack, bool) {
	if state.prefetch.index < 0 {
		return eofTransition{}, nil, false
	}
	if state.shuffle {
		return m.promoteShufflePrefetch(state)
	}
	transition, ok := m.promoteSequentialPrefetch(state)
	return transition, nil, ok
}

// promoteSequentialPrefetch advances the visible-order index onto the prefetched
// entry. The prefetched index may be more than one step ahead: prefetchNext
// skips entries missing on disk.
func (m *Manager) promoteSequentialPrefetch(state eofState) (eofTransition, bool) {
	p := state.prefetch
	if p.index <= state.currentIndex || p.index >= state.queueLen {
		return eofTransition{}, false
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	// Re-check under the write lock: a SetQueue between the snapshot and here
	// can shrink the queue or replace the row, and committing the snapshot
	// anyway would leave currentIndex naming an entry mpv never played.
	if !m.prefetchStillLiveLocked(p) ||
		p.index <= m.currentIndex ||
		p.index >= len(m.queue) ||
		m.queue[p.index].Path != p.path {
		return eofTransition{}, false
	}

	m.currentIndex = p.index
	m.dropPrefetchTrackingLocked()
	m.completedSlotPending = true
	return eofTransition{
		index:  p.index,
		path:   p.path,
		gen:    m.playlistGen,
		marker: state.marker,
	}, true
}

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
	_, err := m.runPlaylistTxnIndex(func() (int, error) {
		m.mu.RLock()
		gen := m.playlistGen
		m.mu.RUnlock()
		if gen != state.playlistGen {
			return -1, errEOFSuperseded
		}
		idx, err := m.advanceAfterCurrentLocked(nextIdx)
		if errors.Is(err, errTraversalExhausted) {
			// The traversal ran out while mpv's playlist was already draining,
			// so the idle that follows is the end of the queue rather than a
			// stall. Recording it inside the transaction is what makes it safe:
			// a replacement cannot interleave here, so the mark can never land
			// on top of a load that has just started playing.
			m.markPlaybackEnded()
		}
		return idx, err
	})
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

// markMissingEntries marks each entry missing on disk. The caller must not hold
// m.mu: marking writes to the database and fires TUI callbacks.
func (m *Manager) markMissingEntries(entries []QueueTrack, marker MissingMarker) {
	for _, entry := range entries {
		m.markEntryMissing(entry, marker)
	}
}
