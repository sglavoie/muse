package player

import (
	"errors"
	"log/slog"

	"github.com/sglavoie/muse/internal/safego"
)

// errStartedSuccessorUnsettled marks an EOF whose invalid gapless successor
// could not be removed. It is distinct from the traversal error underneath it:
// mpv may still be producing audio, so liveness — and, for exhaustion,
// OnPlaybackEnded — may not settle until a retry succeeds or recovery stops the
// old process.
var errStartedSuccessorUnsettled = errors.New("started successor remains unsettled")

const startedSuccessorDiscardAttempts = 2

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
	// gaplessOcc is the queue row that successor is, resolved under the same lock
	// for the same reason: it is the row the next start-file must be reported
	// under, and a replacement completing before that event is dispatched would
	// otherwise leave the current row naming the selection instead.
	gaplessOcc trackOccurrence
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
// playlist — which is what tells the reader how to correlate the start-file it
// reads next — and which queue row that successor is. Both answers hold whatever
// the decision below turns out to be: they describe mpv's playlist at the moment
// the entry ended, not what the Manager chooses to do about it.
func (m *Manager) handleEndOfFile() (bool, trackOccurrence) {
	state := m.snapshotEOF()
	m.dispatchEOF(state)
	return state.gaplessStart, state.gaplessOcc
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
	// exactly what it is going to find out. Until it reports back the traversal
	// must not read as live: mpv has played the entry out and no successor has
	// been loaded. That pending end was already recorded by snapshotEOF, in the
	// same critical section as the snapshot, so nothing — least of all a recovery
	// snapshot racing this dispatch — can observe the completed entry as live.
	nextIdx := state.currentIndex + 1
	safego.Go("player.advanceAfterEOF", func() {
		m.markMissingEntries(skipped, state.marker)
		m.advanceAfterEOF(nextIdx, state)
	})
}

// snapshotEOF captures the state the EOF decision is made from, consumes the
// stop-after-current flag, which must fire exactly once, and publishes the end
// the entry that finished has already reached.
//
// Recording liveness here rather than after the decision is what makes the end
// atomic with the state it was decided from. Every branch below this lock leaves
// the traversal not-live for at least as long as it takes to answer "is there a
// successor": stop-after-current answers it immediately, the gapless promotion
// answers it under the next acquisition of this same lock, and the fallback
// advance answers it a background goroutine and a playlist transaction later. A
// recovery snapshot taken in any of those intervals must not read the completed
// entry as resumable, and only a mark taken in this critical section is ordered
// ahead of all of them — lastTrackPath still names the row that just finished,
// so a live-looking snapshot is a reload of a completed track.
//
// A superseded EOF records nothing and consumes nothing: stop-after-current is
// armed against the track the user is listening to, and the entry a replacement
// is discarding is no longer that track, so spending the flag — or claiming an
// end — here would fire silently against a row that never finished while the
// replacement it belongs to is making its own row current.
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
	if state.gaplessStart {
		// Name that successor now. This is the last point at which the row mpv is
		// about to start is unambiguous: the promotion below commits it as the
		// current row, but a replacement racing this EOF can publish its own row
		// first, and the start-file would then be reported under the selection
		// rather than under the entry mpv actually began.
		state.gaplessOcc = m.gaplessOccurrenceLocked(state)
	}
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
		return state
	}
	// A natural end, whose outcome nothing here knows yet. The traversal holds
	// the pending state until whichever branch of the dispatch resolves it: the
	// gapless promotion marks it live again for the successor mpv already
	// started, replaceLocked does the same for a successor the advance loads, and
	// an exhausted or superseded advance settles it into a confirmed end.
	m.markPlaybackEndingLocked()
	return state
}

// gaplessOccurrenceLocked resolves the queue row mpv is starting on its own: the
// tracked prefetch, located the way the promotion locates it — by identity under
// shuffle, by position otherwise — and confirmed against the file that was
// appended.
//
// It resolves nothing when the tracking no longer describes a live row. mpv still
// starts the entry (it owns the playlist), but nothing here can say which queue
// occurrence that is, so the report falls back to the current row exactly as it
// did before.
//
// The caller must hold m.mu.
func (m *Manager) gaplessOccurrenceLocked(state eofState) trackOccurrence {
	p := state.prefetch
	idx := p.index
	if state.shuffle && p.identity != "" {
		idx = identityIndex(m.queue, p.identity)
	}
	if idx < 0 || idx >= len(m.queue) || m.queue[idx].Path != p.path {
		return trackOccurrence{}
	}
	return m.occurrenceAtLocked(idx)
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

	// Commit as a continuation step rather than by assignment: the prefetch may
	// be one of the insertions a resume left pending, and consuming it here is
	// what stops the next transition from resolving it a second time. A step also
	// leaves the frontier standing, so the transition after this one still knows
	// which rows are behind it.
	m.commitSequentialStepLocked(p.index)
	m.dropPrefetchTrackingLocked()
	m.completedSlotPending = true
	// The successor question the EOF snapshot left pending is answered: mpv is
	// already playing the next entry, so the traversal is running again. This has
	// to happen under the same lock as the commit — runEOFMaintenance's own refill
	// is a best-effort append, which is suppressed while the traversal is not
	// live, so a promotion that left the pending end standing would starve the
	// gapless transition it just committed.
	m.markPlaybackLiveLocked()
	return eofTransition{
		index:  p.index,
		path:   p.path,
		gen:    m.playlistGen,
		marker: state.marker,
	}, true
}
