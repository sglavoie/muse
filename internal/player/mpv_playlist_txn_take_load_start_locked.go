package player

// takeLoadStartLocked consumes the generation and the queue occurrence of the
// next load-induced start-file. An empty queue falls back to the live generation
// and an unknown occurrence, which covers the loads that do not pass through the
// transaction layer (session restore, the post-restart resume) and an entry mpv
// started from an idle playlist.
//
// The caller must hold m.mu.
func (m *Manager) takeLoadStartLocked() (uint64, trackOccurrence) {
	if len(m.pendingLoadStarts) == 0 {
		return m.loadGen, trackOccurrence{}
	}
	pending := m.pendingLoadStarts[0]
	m.pendingLoadStarts = m.pendingLoadStarts[1:]
	if len(m.pendingLoadStarts) == 0 {
		// Drop the backing array instead of re-slicing further into it. A reader
		// that is keeping up empties the queue on every load, which is the
		// steady state, so this is where the storage a burst grew is returned.
		m.pendingLoadStarts = nil
	}
	return pending.gen, pending.occ
}

// takeGaplessSuccessorLocked reports whether mpv has an appended entry waiting
// behind the file it is playing, and consumes it: mpv starts that entry as the
// end-file is emitted, so the expectation is spent by the very next start-file.
//
// The caller must hold m.mu.
func (m *Manager) takeGaplessSuccessorLocked() bool {
	if m.gaplessSuccessors <= 0 {
		return false
	}
	m.gaplessSuccessors--
	return true
}

// resyncGaplessSuccessorsLocked recomputes the successor count for a newly
// correlated generation. mpv's playlist for that generation holds exactly the
// tracked prefetch, if it was appended into it, and nothing else — every
// earlier append belongs to a playlist the load just replaced.
//
// The caller must hold m.mu.
func (m *Manager) resyncGaplessSuccessorsLocked() {
	m.gaplessSuccessors = 0
	if m.startedLoadGen != 0 && m.prefetchedIndex >= 0 && m.prefetchedLoadGen == m.startedLoadGen {
		m.gaplessSuccessors = 1
	}
}

// adoptCurrentLoad re-stamps the entry mpv is playing for an event reader that
// is starting mid-playback: a reconnect or a subprocess restart emits its
// start-file to a socket this reader was not yet scanning. Without it the
// correlation would keep naming a generation this reader never saw a file start
// in, and would discard every end-file after it.
//
// obs is that reader's observation, taken by loadStartBoundary as soon as its
// connection existed; obs.answered is the last load mpv had answered then. Every
// start-file owed to one of those went to a socket this reader never had —
// activateConnection replays the queued commands before it spawns the reader, so
// a queued replacement's own start-file predates this connection too — and none
// of them will ever be answered here: an expectation left behind would be
// consumed by the first start-file this reader *can* read, handing that load a
// generation it did not emit and making its own end-file read as superseded from
// the moment it began.
//
// Everything mpv had not answered by then is the opposite case and must survive.
// The command gate is open by the time the reader is spawned, so a live
// replacement — several of them — can be admitted while it is still coming up,
// and each one emits a start-file this connection receives. So does the one
// whose command was already in flight when the reader dialled: it is unanswered
// at the boundary precisely because mpv has not executed it yet, and it will
// emit into this connection like the rest.
//
// The stamp follows from what is left. With the queue empty, the entry mpv is
// playing is the one the live generation loaded. With expectations preserved,
// the loads they name have not started as far as this reader is concerned, so
// what it will see ending first still belongs to the generation the oldest of
// them displaced. A reader that has never observed a file start leaves the
// correlation disarmed either way, instead of claiming a generation for an entry
// it knows nothing about.
//
// What it cannot decide is a load mpv had executed but not yet answered, whose
// start-file therefore also predates this connection. reconcileStartedLoad
// settles those, from the event stream itself.
//
// It reports whether such an undecidable expectation survived: one recorded
// before this connection existed and still unanswered when it did. The caller
// must then hold its event stream until mpv reports the file it is playing,
// because the first start-file it reads would otherwise consume that
// expectation — and if the expectation names an event this connection never
// received, the load that consumed it is stamped with a generation it did not
// emit and reads as superseded from the moment it begins.
func (m *Manager) adoptCurrentLoad(obs readerObservation) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.retireLoadStartsLocked(obs.answered)
	m.restampObservedLoadLocked()
	return len(m.pendingLoadStarts) > 0 && m.pendingLoadStarts[0].seq <= obs.recorded
}

// reconcileStartedLoad completes the adoption above with the one thing a command
// reply cannot say: which file mpv is actually playing.
//
// The answered boundary places an expectation in the past only once mpv has
// replied to its load, and mpv replies after it has executed it — so a load
// parked inside mpv across the reader's dial and answered just afterwards leaves
// an expectation for a start-file that was broadcast to sockets this reader
// never had. Preserved, it is consumed by the first start-file this reader *can*
// read, and that load reads as superseded from the moment it begins: its
// end-file is discarded and auto-advance stalls.
//
// playing is mpv's own report, delivered on the event stream as the initial
// property-change for `path`. That ordering is what makes it usable: it is a
// point in the same sequence the start-files arrive in, so it describes mpv at
// an instant this reader can place against its own events, rather than against
// when a command happened to return.
//
// delivered is how many start-files the reader read before that report, and it
// is the other half of the same ordering. mpv can execute a load admitted after
// the dial before it gets to the observe, so the report alone cannot say which
// of the loads it proves have started were broadcast to this connection; the
// events read ahead of it can. The caller must therefore have held every event
// it read until this point rather than dispatching them — an expectation a
// start-file has already consumed is gone from the queue, and matching mpv's
// file against what is left could then land on a later load of the same path
// whose event is still to come.
func (m *Manager) reconcileStartedLoad(playing string, delivered int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	unobserved := m.unobservedStartsLocked(playing, delivered)
	if unobserved == 0 {
		return
	}
	m.dropLoadStartsLocked(unobserved)
	m.restampObservedLoadLocked()
}

// restampObservedLoadLocked re-derives the correlation from what the pending
// queue holds after a retirement. A reader that has never observed a file start
// leaves it disarmed rather than claiming a generation for an entry it knows
// nothing about.
//
// The caller must hold m.mu.
func (m *Manager) restampObservedLoadLocked() {
	if m.startedLoadGen != 0 {
		m.startedLoadGen = m.observedLoadGenLocked()
	}
	m.resyncGaplessSuccessorsLocked()
}

// observedLoadGenLocked names the generation of the entry mpv is playing as far
// as a newly connected reader can tell.
//
// A surviving expectation is what makes this differ from the live generation:
// beginReplacementLocked opens a generation one above the one it displaces, so
// the oldest unanswered load names the entry it superseded exactly. Stamping the
// live generation over that would make an entry a replacement has already
// discarded read as the live one, and the end-file it emitted before being
// discarded would then advance the queue past the selection that replaced it.
//
// The caller must hold m.mu.
func (m *Manager) observedLoadGenLocked() uint64 {
	if len(m.pendingLoadStarts) == 0 {
		return m.loadGen
	}
	return m.pendingLoadStarts[0].gen - 1
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
		loadGen:  m.prefetchedLoadGen,
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
	m.prefetchedLoadGen = 0
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
	m.prefetchedLoadGen = s.loadGen
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
// This is the commit for a selection: an explicit one the user made, or a
// shuffle step, both of which reanchor plain visible-order continuation on the
// row they land. Automatic shuffle-off continuation goes through
// commitSequentialStepLocked instead, which is the only thing that may keep a
// frontier standing.
//
// The caller must hold m.mu.
func (m *Manager) commitIndexLocked(index int) func() {
	prev := m.currentIndex
	prevAdditions := m.unreachedAdditions
	prevCompleted := m.sequentialCompleted
	m.setCurrentIndexLocked(index)
	// A committed selection establishes a new sequential position, and positional
	// traversal never goes back: whatever had been inserted behind the old cursor
	// is either the row that just started or is now behind the new one. Keeping
	// the set would let a later resume jump backwards onto a row the user has
	// already moved past, and keeping a frontier would keep routing continuation
	// around rows the selection has made reachable again.
	m.unreachedAdditions = nil
	m.sequentialCompleted = nil
	return func() {
		m.currentIndex = prev
		m.unreachedAdditions = prevAdditions
		m.sequentialCompleted = prevCompleted
	}
}

// commitSequentialStepLocked publishes index for one automatic shuffle-off
// continuation step: the gapless promotion at EOF, the fallback advance behind
// it, and the positional tail of a resume.
//
// It differs from commitIndexLocked in what it does not throw away. A step is
// not a selection: it consumes the insertions it has moved past and leaves the
// ones still ahead of it, and it leaves the frontier where it is. That is what
// lets a traversal that stepped back onto an insertion visit the rest of them on
// the following EOFs instead of resolving currentIndex+1 into a row it already
// played. With no frontier standing every recorded insertion sits behind the
// cursor and a forward step drops all of them, which is exactly what
// commitIndexLocked would have done.
//
// Where a frontier does stand the step also extends it: the row being started is
// about to be a row that played, and inside a boundary that is recorded by
// identity rather than inferred from the cursor.
//
// The caller must hold m.mu.
func (m *Manager) commitSequentialStepLocked(index int) func() {
	prev := m.currentIndex
	dropped := m.forgetAdditionsThroughLocked(index)
	undoCompleted := m.completeSequentialStepLocked(index)
	m.setCurrentIndexLocked(index)
	return func() {
		m.currentIndex = prev
		undoCompleted()
		m.restoreAdditionsLocked(dropped)
	}
}

// commitResumedAdditionLocked publishes index for a resume that stepped
// backwards onto a row inserted behind an exhausted cursor.
//
// Moving the cursor back is what makes the rows the traversal already played
// numeric successors again, so the prefix it had actually played is pinned as
// the completion boundary first. Every continuation from here consults the
// frontier that prefix resolves to rather than the cursor, and only an explicit
// selection retires it.
//
// The commit runs twice for one load — once before the loadfile and once after
// mpv accepts it — so establishing the boundary is conditional on the step still
// being a step backwards; the second call finds the cursor already moved and
// leaves the boundary alone.
//
// The caller must hold m.mu.
func (m *Manager) commitResumedAdditionLocked(index int) func() {
	prevCompleted := m.sequentialCompleted
	if len(m.sequentialCompleted) == 0 && index < m.currentIndex {
		m.pinSequentialFrontierLocked()
	}
	undoStep := m.commitSequentialStepLocked(index)
	return func() {
		undoStep()
		m.sequentialCompleted = prevCompleted
	}
}

// commitReplayedIndexLocked publishes index for a replay of the row that is
// already current. A replay restarts a row without moving the timeline, so
// unlike every other commit it touches nothing else: the insertions still
// pending and the frontier the traversal reached both describe what comes after
// the replayed row just as they did before it.
//
// The caller must hold m.mu.
func (m *Manager) commitReplayedIndexLocked(index int) func() {
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
