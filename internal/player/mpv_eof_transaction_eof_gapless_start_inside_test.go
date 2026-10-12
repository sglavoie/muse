package player

import (
	"slices"
	"testing"
	"time"
)

// TestEOF_GaplessStartInsideARejectedReplacementStaysLive covers a window that
// loaded nothing while mpv moved into the prefetched successor by itself.
//
// That successor was appended behind the entry the window is superseding, so it
// belongs to the generation the window displaced rather than to the one the
// window opened — the load that would have justified the new generation never
// reached mpv. Handing the generation back on close is what keeps the two
// matched. Leaving it open would strand the correlation behind the live
// generation and discard the successor's own end-file, stalling the queue.
func TestEOF_GaplessStartInsideARejectedReplacementStaysLive(t *testing.T) {
	m, f := newFakePlaylistManager(t, "eof-gapless-in-window")
	queue := queueOf(t, 3)
	installQueue(m, queue, -1, true)
	runEventReader(t, m, f)

	if err := m.PlayFromIndex(0); err != nil {
		t.Fatalf("PlayFromIndex(0): %v", err)
	}
	f.waitForEntries(t, paths(queue, 0, 1))
	waitForCorrelatedLoad(t, m, f)

	// Park a selection so the window stays open, then let mpv reach the end of
	// the current entry inside it. Spending the successor expectation is what
	// proves the reader has dispatched that end-file from inside the window;
	// the start-file it read next cannot be waited on directly, because its
	// handler blocks on the command socket the parked loadfile is holding.
	gate, selected := parkReplacement(t, m, f, 2)
	f.finishCurrent()
	f.waitFor(t, "the superseded end-file to be dispatched inside the window", func() bool {
		return gaplessSuccessorsOf(m) == 0
	})

	// The selection is then refused, so the window closes having loaded nothing
	// — and the successor mpv is playing keeps matching the live generation.
	f.failCommand("loadfile:replace", "device error")
	gate.release()
	f.unblock("loadfile:replace")
	if err := <-selected; err == nil {
		t.Fatal("PlayFromIndex(2) succeeded; the fake was told to reject the load")
	}
	f.clearFailure("loadfile:replace")
	waitForCorrelatedLoad(t, m, f)

	// The successor now ends. Its end-file is the live one and must be acted on.
	f.finishCurrent()
	waitForCurrentIndex(t, m, 1)
	f.waitFor(t, "the queue to keep running past the successor", func() bool {
		return f.count(isAppendOf(queue[2].Path)) > 0
	})
}

// TestEOF_ReaderRestartAdoptsTheLiveLoadGeneration covers the correlation's blind
// spot: a load that lands while no event reader is scanning. Its start-file goes
// to a socket nobody is reading, so the stamp on the entry mpv is playing is left
// naming the generation the previous reader last saw a file start in — and every
// end-file from then on reads as superseded, stalling the queue for good. The
// reader that comes up next has to adopt what mpv is playing as its own.
//
// The adoption is driven directly rather than by starting a second reader: both
// readers would hold a live connection to the one fake socket and split the event
// stream between them. It is called at exactly this point in startEventReader,
// next to the TrackChangedMsg emitted for the same unobserved start-file.
func TestEOF_ReaderRestartAdoptsTheLiveLoadGeneration(t *testing.T) {
	m, f := newFakePlaylistManager(t, "eof-reader-restart")
	queue := queueOf(t, 3)
	installQueue(m, queue, -1, true)
	runEventReader(t, m, f)

	if err := m.PlayFromIndex(0); err != nil {
		t.Fatalf("PlayFromIndex(0): %v", err)
	}
	f.waitForEntries(t, paths(queue, 0, 1))
	assertPrefetch(t, m, queue, 1)
	waitForCorrelatedLoad(t, m, f)

	// A replacement opens and lands with the reader down — an mpv restart
	// reloading the current file, or a load deferred to the reconnect queue and
	// drained during activation. mpv is playing the new entry; nothing correlated
	// it, so the stamp is a generation behind.
	m.mu.Lock()
	m.loadGen++
	m.mu.Unlock()

	m.adoptCurrentLoad(m.loadStartBoundary())

	// The entry mpv is playing now ends. It is the live one, so the queue keeps
	// running: without the adoption its end-file looks superseded and is dropped.
	f.finishCurrent()
	waitForCurrentIndex(t, m, 1)
	if got := f.playingPath(); got != queue[1].Path {
		t.Fatalf("mpv is playing %q, want the promoted successor %q", got, queue[1].Path)
	}
	assertPrefetch(t, m, queue, 2)
}

// TestEOF_InitialActivationDropsUnobservedLoadStartExpectation covers the
// adoption path on a reader whose correlation has never been armed.
//
// activateConnection replays the commands captured during the outage and only
// then spawns the event reader, so a queued playlist replacement reaches mpv
// while nothing is scanning the event socket: the start-file it produces is
// broadcast to connections that do not exist yet, and the expectation recorded
// for it can never be answered. Retiring those expectations only when
// startedLoadGen is nonzero leaves this one in place — an initial reader's
// correlation is deliberately disarmed — and the next load's start-file then
// consumes the dead generation instead of its own. That load reads as
// superseded from the instant it starts, its end-file is discarded, and
// auto-advance stalls with mpv idle at the end of the track.
func TestEOF_InitialActivationDropsUnobservedLoadStartExpectation(t *testing.T) {
	m, f := newFakePlaylistManager(t, "eof-initial-reader-adopt")
	queue := queueOf(t, 5)
	installQueue(m, queue, -1, true)
	// Model mpv's broadcast: the replayed load's start-file must be genuinely
	// unobservable to the reader that comes up after it, not merely early.
	f.dropUnsubscribedEvents()

	// The manager is mid-activation — a client is installed but the gate is
	// still closed — so the selection is captured for replay rather than sent,
	// and no event reader exists to observe what replaying it will produce.
	m.mu.Lock()
	client := m.client
	m.activating = true
	m.cmdQueue = []func() error{}
	m.mu.Unlock()

	if err := m.PlayFromIndex(0); err != nil {
		t.Fatalf("PlayFromIndex(0): %v", err)
	}
	if got := f.count(isLoadfileReplace); got != 0 {
		t.Fatalf("loadfile replace count = %d, want 0: the load was meant to be queued for replay", got)
	}

	// Activation replays it against mpv, rebuilds the prefetch, and only then
	// starts the reader.
	m.activateConnection(m.ctx, client, nil)
	f.waitForEntries(t, paths(queue, 0, 1))
	f.waitForEventStream(t)

	// The expectation that replay left behind names a start-file this reader
	// cannot ever read, so the adoption has to retire it.
	f.waitFor(t, "the initial reader to retire the unobservable load expectation", func() bool {
		return len(pendingLoadStartsOf(m)) == 0
	})

	// A load performed now is the first one this reader can correlate, and it
	// must be stamped with its own generation rather than with the retired one.
	if err := m.PlayFromIndex(2); err != nil {
		t.Fatalf("PlayFromIndex(2): %v", err)
	}
	f.waitForEntries(t, paths(queue, 2, 3))
	waitForCorrelatedLoad(t, m, f)
	// Wait for the successor to be tracked and counted rather than merely
	// present in mpv's playlist: appendPrefetchLocked records the tracking only
	// after the command returns, and a promotion decided before that lands
	// would take the fallback advance instead.
	f.waitFor(t, "the selection's successor to be tracked as a gapless one", func() bool {
		got, gen := prefetchTracking(m)
		return got.index == 3 && got.gen == gen && gaplessSuccessorsOf(m) == 1
	})

	// Its end-file is therefore live: mpv moves into the prefetched successor
	// and the queue keeps running instead of stopping at the end of the track.
	f.finishCurrent()
	waitForCurrentIndex(t, m, 3)
	if got := f.playingPath(); got != queue[3].Path {
		t.Fatalf("mpv is playing %q, want the promoted successor %q", got, queue[3].Path)
	}
	f.waitForEntries(t, paths(queue, 3, 4))
	f.waitFor(t, "the promoted transition to refill the prefetch", func() bool {
		got, gen := prefetchTracking(m)
		return got.index == 4 && got.path == queue[4].Path && got.gen == gen
	})
}

// TestEOF_ReaderAdoptionPreservesObservableLoadExpectations is the deterministic
// regression for the other side of that adoption: the expectations it must not
// retire.
//
// activateConnection opens the command gate when its drain completes and only
// then spawns the event reader, so from the instant that reader's connection
// exists until the instant it adopts, the manager keeps accepting explicit
// selections — and every one of them emits a start-file to a socket this reader
// is already on the receiving end of. Retiring the whole pending queue discards
// those generations together with the prefix that really was unobservable.
//
// Two replacements in that window are what makes the loss visible. The first
// buffered start-file then finds the queue empty and falls back to the live
// generation, so the entry the second replacement discarded reads as the one mpv
// is playing: the end-file it emitted before it was discarded passes
// eofState.superseded, promotes the selection's own prefetch as if it were the
// successor that had just started, and runs the completed-slot cleanup against
// the selection's playlist — deleting the row mpv is actually playing.
//
// Both events belong to a superseded entry, so both must decide nothing: the
// last selection stands, and so do its prefetch, mpv's playlist, and the
// traversal position.
func TestEOF_ReaderAdoptionPreservesObservableLoadExpectations(t *testing.T) {
	t.Run("sequential", func(t *testing.T) {
		m, f := newUnwiredFakePlaylistManager(t, "eof-adopt-boundary-seq")
		queue := queueOf(t, 6)
		installQueue(m, queue, -1, true)
		readerAdoptionBoundaryScenario(t, m, f, queue)
	})

	// The shuffle half moves the traversal rather than the visible index: a
	// promotion here consumes an identity onto the cycle the last selection
	// reanchored, leaving a Previous entry that was never heard.
	t.Run("shuffled", func(t *testing.T) {
		m, f := newUnwiredFakePlaylistManager(t, "eof-adopt-boundary-shuf")
		queue := queueOf(t, 6)
		m.mu.Lock()
		m.gaplessEnabled = true
		m.mu.Unlock()
		buildShuffleState(t, m, queue, 0, reversePermute)
		readerAdoptionBoundaryScenario(t, m, f, queue)
	})
}

// readerAdoptionBoundaryScenario replays a load with no reader connected, admits
// two more once the reader's connection exists but before it has adopted, and
// then lets that reader read everything mpv emitted while it was parked.
//
// The manager it is given must still have its callbacks unwired: that is what
// holds startEventReader between its dial and its adoption, which is the window
// under test.
func readerAdoptionBoundaryScenario(t *testing.T, m *Manager, f *fakePlaylist, queue []QueueTrack) {
	t.Helper()

	// Model mpv's broadcast: the replayed load's start-file must be genuinely
	// unobservable to the reader that comes up after it, not merely early.
	f.dropUnsubscribedEvents()

	// The manager is mid-activation — a client is installed but the gate is
	// still closed — so this selection is captured for replay rather than sent.
	m.mu.Lock()
	client := m.client
	m.activating = true
	m.cmdQueue = []func() error{}
	m.mu.Unlock()

	if err := m.PlayFromIndex(0); err != nil {
		t.Fatalf("PlayFromIndex(0): %v", err)
	}
	if got := f.count(isLoadfileReplace); got != 0 {
		t.Fatalf("loadfile replace count = %d, want 0: the load was meant to be queued for replay", got)
	}

	// Activation replays it against mpv, rebuilds the prefetch, and spawns the
	// reader. With the callbacks unwired the reader connects and then parks, so
	// its event connection exists and it has adopted nothing.
	m.activateConnection(m.ctx, client, nil)
	f.waitForPlayingWithSuccessor(t, queue[0].Path)
	f.waitForEventStream(t)

	// Freeze the stream at the socket. Everything mpv emits from here is
	// observable to this reader — it is connected — and none of it is read yet,
	// which is exactly the state the adoption below has to reason about.
	release := f.holdEvents(t)

	// The first live replacement of the window. Its refill is refused so the
	// entry it starts ends alone: the end-file that follows is then the only
	// event between the two start-files, and each start-file answers exactly one
	// load.
	appendsBefore := f.count(isLoadfileAppend)
	f.failCommandOnce("loadfile:append-play", "mpv rejected the append")
	if err := m.PlayFromIndex(2); err != nil {
		t.Fatalf("PlayFromIndex(2): %v", err)
	}
	f.waitFor(t, "the first admitted replacement to attempt its refill", func() bool {
		return f.count(isLoadfileAppend) > appendsBefore
	})
	f.clearFailure("loadfile:append-play")
	if got := f.entriesSnapshot(); !slices.Equal(got, paths(queue, 2)) {
		t.Fatalf("entries = %v, want %v: the superseded entry must end without a successor",
			got, paths(queue, 2))
	}
	supersededGen := loadGenOf(m)

	// It plays out, so the end-file of an entry the next replacement is about to
	// discard now sits between the two unread start-files.
	f.finishCurrent()

	// The second live replacement supersedes all of it and prefetches its own
	// successor behind itself.
	if err := m.PlayFromIndex(4); err != nil {
		t.Fatalf("PlayFromIndex(4): %v", err)
	}
	f.waitForPlayingWithSuccessor(t, queue[4].Path)
	// The append reaches mpv before the tracking that describes it is recorded,
	// so the playlist holding two entries is not yet proof that the Manager has a
	// promotable successor. Wait for the tracking itself.
	f.waitFor(t, "the selection's own successor to be tracked", func() bool {
		got, gen := prefetchTracking(m)
		return got.index >= 0 && got.gen == gen
	})
	selectedGen := loadGenOf(m)

	wantEntries := f.entriesSnapshot()
	wantPrefetch, _ := prefetchTracking(m)
	removalsBefore := f.count(isPlaylistRemove)
	replacesBefore := f.count(isLoadfileReplace)
	if got := pendingLoadStartsOf(m); len(got) != 3 {
		t.Fatalf("pendingLoadStarts = %v, want the replayed load plus the two admitted since", got)
	}

	// Wiring the callbacks releases the parked reader into its adoption. The
	// stream is still frozen, so nothing consumes the queue behind it and the
	// length it settles on is the adoption's own decision.
	m.SetEventCallbacks(noopEventCallbacks())
	f.waitFor(t, "the reader to adopt the current load", func() bool {
		return len(pendingLoadStartsOf(m)) != 3
	})
	if got := pendingLoadStartsOf(m); !slices.Equal(got, []uint64{supersededGen, selectedGen}) {
		t.Fatalf("pendingLoadStarts = %v, want %v: adoption retired a generation whose start-file "+
			"this reader can still read", got, []uint64{supersededGen, selectedGen})
	}

	// Only now does the reader reach the buffered events, in the order mpv
	// emitted them.
	drainHeldEvents(t, m, f, release)
	// The damage a promotion does lands in its background maintenance, after the
	// decision itself has returned.
	time.Sleep(150 * time.Millisecond)

	if got := currentIdx(m); got != 4 {
		t.Fatalf("currentIndex = %d, want 4: a buffered load start-file took the newer load's generation", got)
	}
	if got := f.playingPath(); got != queue[4].Path {
		t.Fatalf("mpv is playing %q, want the selected %q", got, queue[4].Path)
	}
	if got := f.entriesSnapshot(); !slices.Equal(got, wantEntries) {
		t.Fatalf("entries = %v, want %v: the selected row and its successor", got, wantEntries)
	}
	if got, _ := prefetchTracking(m); got != wantPrefetch {
		t.Fatalf("prefetch = %+v, want %+v: a superseded EOF consumed the selection's successor",
			got, wantPrefetch)
	}
	if got := f.count(isPlaylistRemove); got != removalsBefore {
		t.Fatalf("playlist-remove count = %d, want %d: the superseded EOF ran its completed-slot cleanup "+
			"against the selection's playlist", got, removalsBefore)
	}
	if got := f.count(isLoadfileReplace); got != replacesBefore {
		t.Fatalf("loadfile replace count = %d, want %d: the superseded EOF sent an advance of its own",
			got, replacesBefore)
	}
	if history, pos, _ := shuffleSnapshotForTest(m); len(history) > 1 || pos > 0 {
		t.Fatalf("shuffleHistory = %v pos = %d, want the reanchored selection alone: "+
			"a superseded EOF committed onto the new cycle", history, pos)
	}
	// The correlation must land on the generation the last selection loaded, or
	// the next genuine end-file would be discarded and the queue would stall.
	waitForCorrelatedLoad(t, m, f)
}

// TestEOF_ReaderAdoptionKeepsLoadWhoseCommandSpansDial is the deterministic
// regression for the boundary itself: what places a load-start expectation on
// one side or the other of a reader's connection.
//
// The expectation is recorded before the loadfile is sent, because mpv can emit
// the start-file before the command's own reply comes back. Placing it against
// the connection by *when it was recorded* therefore measures the Manager's
// intent to load rather than anything about the event that load produces: a
// replacement admitted just before the reader dials can still be parked inside
// mpv while the connection is established, and mpv then executes it — and
// broadcasts its start-file — with this reader already on the receiving end.
//
// Retiring that expectation does not lose one correlation, it shifts every
// other. The spanning load's buffered start-file finds only the later
// replacement's generation queued and takes it, so the entry that replacement
// discarded reads as the one mpv is playing: the end-file it emitted before
// being discarded passes eofState.superseded, promotes the selection's own
// successor as if it had just started, and runs the completed-slot cleanup
// against the selection's playlist — deleting the row mpv is actually playing.
//
// Both buffered events belong to a superseded entry, so both must decide
// nothing: the last selection stands, and so do its prefetch, mpv's playlist,
// and the traversal position.
func TestEOF_ReaderAdoptionKeepsLoadWhoseCommandSpansDial(t *testing.T) {
	t.Run("sequential", func(t *testing.T) {
		m, f := newUnwiredFakePlaylistManager(t, "eof-dial-span-seq")
		queue := queueOf(t, 6)
		installQueue(m, queue, -1, true)
		readerAdoptionDialSpanScenario(t, m, f, queue)
	})

	// The shuffle half moves the traversal rather than the visible index: a
	// promotion here consumes an identity onto the cycle the last selection
	// reanchored, leaving a Previous entry that was never heard.
	t.Run("shuffled", func(t *testing.T) {
		m, f := newUnwiredFakePlaylistManager(t, "eof-dial-span-shuf")
		queue := queueOf(t, 6)
		m.mu.Lock()
		m.gaplessEnabled = true
		m.mu.Unlock()
		buildShuffleState(t, m, queue, 0, reversePermute)
		readerAdoptionDialSpanScenario(t, m, f, queue)
	})
}
