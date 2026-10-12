package player

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
	loadGen  uint64
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
	// prevLoadGen is the load generation the outermost window displaced and
	// loaded records whether a load inside it reached mpv. Together they let a
	// window that replaced nothing hand the generation back, so the entry mpv
	// never stopped playing keeps matching it.
	prevLoadGen uint64
	loaded      bool
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
		m.replacing.prevLoadGen = m.loadGen
		// Open the load generation here rather than when mpv answers: from this
		// point on, whatever mpv is still playing belongs to the generation
		// this window is superseding, and it keeps belonging to it after the
		// answer arrives and the window closes.
		m.loadGen++
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
	if m.replacing.depth > 0 {
		return
	}
	// A window that got no load accepted — an exhausted traversal, a rejected
	// loadfile, a transaction that only dropped a prefetch — leaves mpv on the
	// entry it was already playing, so the generation it opened describes
	// nothing. Hand it back, or that entry's own end-file would read as
	// superseded and be discarded, stalling the queue.
	//
	// Unless a file mpv reported starting inside the window claimed the
	// generation it opened. Only a start this window's own load cannot account
	// for does that — a restart resume, or an entry mpv began from an idle
	// playlist — and it makes that generation the live one, so it must stand.
	if !m.replacing.loaded && m.startedLoadGen != m.loadGen {
		m.loadGen = m.replacing.prevLoadGen
	}
	m.replacing = replacementState{}
}

// noteFileStarted records the load generation of the file mpv has begun
// playing. The event reader calls it for every start-file, which is what arms
// the end-file correlation: events arrive in order on one socket, so the
// end-file of an entry is always read before the start-file of whatever
// replaced it.
//
// gapless says the previous event was an end-file/eof for an entry that had a
// successor waiting behind it in mpv's playlist. mpv is then continuing inside
// the generation that entry belongs to, so the correlation does not move — and
// must not, because a replacement can complete while both events sit buffered
// on the socket, and stamping the successor with the generation live at that
// later instant is what would make its own end-file read as belonging to the
// row the user just selected.
//
// Every other start-file answers a load the Manager sent, and takes the
// generation of the oldest such load that has not been answered yet.
//
// It also reports the queue occurrence the start belongs to, which is the same
// question answered against the same two sources: gaplessOcc is the row the
// transition handed mpv over to, and a load-induced start takes the row recorded
// with the expectation it consumes. Either way the answer is state captured
// before this start-file was read, never the current row re-read now — the caller
// is about to block on IPC, and a selection racing it has already published its
// own row as current.
//
// An unresolved occurrence falls back to that current row, which is what the
// loads outside the transaction layer (session restore, the post-restart resume)
// have always reported.
func (m *Manager) noteFileStarted(gapless bool, gaplessOcc trackOccurrence) trackOccurrence {
	m.mu.Lock()
	defer m.mu.Unlock()
	if gapless && m.startedLoadGen != 0 {
		return m.startOccurrenceLocked(gaplessOcc)
	}
	gen, occ := m.takeLoadStartLocked()
	m.startedLoadGen = gen
	m.resyncGaplessSuccessorsLocked()
	if gapless && gaplessOcc.known {
		// mpv continued into an entry that was already in its playlist, so the
		// transition names the row even though the correlation fell through to a
		// load expectation above.
		return m.startOccurrenceLocked(gaplessOcc)
	}
	return m.startOccurrenceLocked(occ)
}

// pendingLoadStart is one unanswered load-start expectation: the load
// generation a `loadfile ... replace` was sent in, and the position of that
// load in the stream of loads this process has issued.
//
// seq exists for the event reader's observation boundary. mpv broadcasts an
// event to the connections that exist when it fires, so an expectation whose
// start-file was already emitted when a reader's connection came up names an
// event that reader can never be handed, while one whose load had not been
// executed yet names an event it will read. Nothing else separates the two: the
// generation moves for reasons unrelated to when the reader connected, and the
// queue's own length says nothing about when any particular entry joined it.
//
// The number is compared against sentLoadStartSeq rather than loadStartSeq, so
// what places an expectation against the connection is the moment mpv answered
// its load — the only observable that says the start-file has fired — and not
// the earlier moment the Manager decided to send it.
//
// That answer is still only an upper bound: it proves the event has fired, and
// nothing proves it has not. path is what closes the other side — mpv naming the
// file it is playing places every expectation up to the one that loaded it in
// the past, whatever the Manager has been told about the commands themselves.
// See reconcileStartedLoad.
//
// occ is the queue row the load is making current, captured in the same critical
// section that publishes it. It is what the start-file this expectation names is
// reported under: by the time that event is dispatched, a later replacement may
// already have published its own row, so the current row no longer identifies
// what mpv just started.
type pendingLoadStart struct {
	gen  uint64
	seq  uint64
	path string
	occ  trackOccurrence
}

// expectLoadStartLocked records that a load has been sent in the live
// generation, so the start-file answering it is correlated with the generation
// that emitted it however long the event reader takes to get there.
//
// Nothing is ever dropped to bound the queue's length. The correlation is
// positional: entry k names the generation of the k-th start-file still to be
// read, so discarding the front shifts every entry behind it onto an earlier
// load. Once enough loads have queued up, that shift hands a superseded entry
// the live generation, and the end-file it emitted before it was discarded then
// passes eofState.superseded and mutates the selection that displaced it.
//
// The queue is bounded by how far behind the event reader has fallen rather
// than by a constant: an entry is withdrawn by cancelLoadStartLocked when mpv
// rejects the load, consumed by the start-file that answers it, or retired by
// retireLoadStartsLocked when the reader that was owed it is gone.
//
// path is the file the load will make current, which is how a reader that comes
// up mid-playback recognises the expectation mpv has already answered with the
// entry it is playing.
//
// entry is the queue row being loaded, recorded alongside the position the
// caller's commit has just published for it. Together they are the occurrence the
// resulting start-file is reported under.
//
// It returns the sequence number it recorded, which the caller hands to
// noteLoadStartSent once mpv has answered the load.
//
// The caller must hold m.mu.
func (m *Manager) expectLoadStartLocked(entry QueueTrack) uint64 {
	m.loadStartSeq++
	m.pendingLoadStarts = append(m.pendingLoadStarts, pendingLoadStart{
		gen:  m.loadGen,
		seq:  m.loadStartSeq,
		path: entry.Path,
		occ:  trackOccurrence{track: entry, index: m.currentIndex, known: true},
	})
	return m.loadStartSeq
}

// noteLoadStartSent records that mpv has answered the load recorded under seq,
// which is what places that load's start-file in time: mpv emits the event while
// it executes the loadfile and replies afterwards, so the answer proves the
// event has already fired.
//
// It is the observation boundary an event reader captures on connecting, and it
// is why a load whose command spans that dial keeps its expectation: nothing has
// answered it, so its start-file is still to come and the new connection will be
// handed it.
//
// It never moves backwards. Loads are sent one at a time under playlistMu, and a
// send deferred to the reconnect queue is replayed under it too, so the
// answers arrive in the order the expectations were recorded — the guard is
// there so a future path that breaks that ordering cannot silently widen the
// boundary back over an unanswered load.
func (m *Manager) noteLoadStartSent(seq uint64) {
	m.mu.Lock()
	if seq > m.sentLoadStartSeq {
		m.sentLoadStartSeq = seq
	}
	m.mu.Unlock()
}

// cancelLoadStartLocked withdraws the expectation expectLoadStartLocked
// recorded for gen, for a load mpv rejected: it answers with an error rather
// than a start-file, so nothing will ever consume it. It withdraws only the
// most recent entry, since anything queued behind it belongs to another load.
//
// The caller must hold m.mu.
func (m *Manager) cancelLoadStartLocked(gen uint64) {
	n := len(m.pendingLoadStarts)
	if n > 0 && m.pendingLoadStarts[n-1].gen == gen {
		m.pendingLoadStarts = m.pendingLoadStarts[:n-1]
	}
}

// readerObservation is what the load-start queue looked like at the instant an
// event reader's connection came up: the last load mpv had answered by then and
// the last one the Manager had recorded.
//
// answered is the boundary adoptCurrentLoad retires against. recorded says which
// of the survivors were recorded before this connection existed, and those are
// the only ones that can still name a start-file it will never be handed —
// everything recorded afterwards belongs to a load this reader is on the
// receiving end of. An expectation of the first kind is what raises the event
// reader's adoption barrier; see reconcileStartedLoad.
type readerObservation struct {
	answered uint64
	recorded uint64
}

// loadStartBoundary captures the observation boundary of an event reader whose
// connection has just been established.
//
// mpv broadcasts an event to the connections that exist at the instant it
// fires, so this instant separates the load-start expectations naming
// start-files that went to sockets this reader never had from the ones naming
// start-files it will read. adoptCurrentLoad retires exactly the former.
//
// The boundary is the last load mpv has *answered*, not the last one recorded.
// A recorded load says only that the Manager decided to send it: the command can
// still be blocked in the IPC send while this connection is being established,
// and mpv then executes it — and emits its start-file — with this reader already
// on the receiving end. Placing the boundary at the intent would retire that
// expectation, and the first event this reader does read would consume a
// generation belonging to a later load.
//
// It must be read before the reader blocks on anything, so the boundary
// describes the connection rather than how long the reader took to reach its
// scan loop: activateConnection opens the command gate before it spawns the
// reader, so live replacements can be admitted throughout that interval, and
// every one of them emits a start-file this connection receives.
//
// The answer is a lower bound on what has started, never an upper one: mpv emits
// the start-file while it executes the load and replies afterwards, so a load it
// has not answered may still have fired its event already. That gap is what
// reconcileStartedLoad closes, against mpv's own report of the file it is
// playing rather than against the reply.
func (m *Manager) loadStartBoundary() readerObservation {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return readerObservation{answered: m.sentLoadStartSeq, recorded: m.loadStartSeq}
}

// retireLoadStartsLocked drops the expectations whose loads mpv had already
// answered at boundary — their start-files were emitted to connections this
// reader did not have — and keeps every expectation still unanswered then,
// including one whose command was in flight across the reader's own dial.
//
// Retiring the whole queue instead would be wrong in the same way trimming it
// to a fixed length was: the surviving entries name start-files this reader is
// about to read, and dropping them makes the first of those fall back to the
// live generation. The entry it belongs to then reads as the one mpv is playing
// even though a later replacement has already discarded it, and the end-file it
// emitted before that is acted on against the selection that displaced it.
//
// The caller must hold m.mu.
func (m *Manager) retireLoadStartsLocked(boundary uint64) {
	retired := 0
	for retired < len(m.pendingLoadStarts) && m.pendingLoadStarts[retired].seq <= boundary {
		retired++
	}
	m.dropLoadStartsLocked(retired)
}

// dropLoadStartsLocked removes the first n expectations, which is how both
// retirement rules give up on start-files that will never be read.
//
// The caller must hold m.mu.
func (m *Manager) dropLoadStartsLocked(n int) {
	if n <= 0 {
		return
	}
	if n >= len(m.pendingLoadStarts) {
		// Drop the backing array rather than re-slice to an empty tail, the way
		// takeLoadStartLocked does once the reader has caught up.
		m.pendingLoadStarts = nil
		return
	}
	m.pendingLoadStarts = m.pendingLoadStarts[n:]
}

// unobservedStartsLocked counts the expectations at the front of the queue whose
// start-files a newly connected event reader can never be handed, given the file
// mpv reports playing and how many start-files that reader read before the
// report.
//
// Loads are sent one at a time under playlistMu and mpv executes them in the
// order it receives them, so the entry that loaded the current file places every
// expectation ahead of it in the past as well: their start-files have all fired.
// It is the *first* match that counts. A path can be loaded twice — a replay of
// the same row, a restart reloading it — and the later expectation is then a
// load whose event may still be coming; stopping at the first occurrence can
// only leave a stale entry behind, never discard a live one.
//
// Having fired is not the same as having been missed, though. The report is
// ordered in the reader's own event stream, so the start-files it read ahead of
// it are exactly the ones mpv broadcast with this connection already open —
// the last delivered of the loads that fired. Subtracting them leaves the prefix
// that fired before this connection existed, which is what may be retired.
//
// delivered is the reader's whole count, so an entry mpv started on its own
// behind a completed one inflates it. That only leaves an expectation behind,
// which is the direction this has to err in: retiring one whose start-file is
// still coming makes that event consume a generation it did not emit.
//
// The caller must hold m.mu.
func (m *Manager) unobservedStartsLocked(playing string, delivered int) int {
	if playing == "" {
		return 0
	}
	for i, pending := range m.pendingLoadStarts {
		if pending.path == playing {
			return max(i+1-delivered, 0)
		}
	}
	return 0
}
