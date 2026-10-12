package player

import (
	"slices"
	"testing"
	"time"
)

// readerAdoptionDialSpanScenario plays one load with no reader connected at all,
// parks the next one inside mpv and brings the reader up while it is still in
// flight, admits a third once that one has played out, and only then lets the
// reader adopt and read everything mpv emitted meanwhile.
//
// The manager it is given must still have its callbacks unwired: that park is
// what holds startEventReader between its dial and its adoption, so the spanning
// load can be released with the connection established and nothing yet adopted.
func readerAdoptionDialSpanScenario(t *testing.T, m *Manager, f *fakePlaylist, queue []QueueTrack) {
	t.Helper()

	// Model mpv's broadcast, so "this reader could not have seen it" is a fact
	// about the fake and not merely about timing.
	f.dropUnsubscribedEvents()

	// A load performed with no event reader anywhere. mpv answers it, so its
	// start-file has already fired by the time anything connects: this is the
	// expectation the adoption must retire.
	if err := m.PlayFromIndex(0); err != nil {
		t.Fatalf("PlayFromIndex(0): %v", err)
	}
	f.waitForPlayingWithSuccessor(t, queue[0].Path)
	appendsBefore := f.count(isLoadfileAppend)

	// The next replacement is parked inside mpv before it can execute, so its
	// expectation is recorded while its start-file is still to come.
	gate := f.block(t, "loadfile:replace")
	spanning := make(chan error, 1)
	go func() { spanning <- m.PlayFromIndex(2) }()
	gate.waitArrived(t, "the replacement that spans the reader's dial")

	// The reader connects on top of that in-flight command. The fake relaying
	// its stream proves the dial returned — the boundary is captured between the
	// two — and the unwired callbacks then park the reader before it adopts.
	go m.startEventReader(m.ctx, nil)
	f.waitForEventStream(t)

	// Freeze the stream at the socket. Everything mpv emits from here is
	// observable to this reader and none of it is read yet, which is the state
	// the adoption below has to reason about.
	release := f.holdEvents(t)

	// Let the parked load through. Its refill is refused so the entry it starts
	// ends alone: the end-file that follows is then the only event between the
	// two start-files, and each start-file answers exactly one load.
	f.failCommandOnce("loadfile:append-play", "mpv rejected the append")
	f.unblock("loadfile:replace")
	if err := <-spanning; err != nil {
		t.Fatalf("PlayFromIndex(2): %v", err)
	}
	f.waitFor(t, "the spanning replacement to attempt its refill", func() bool {
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

	// The last replacement supersedes all of it and prefetches its own successor
	// behind itself, still before the reader has adopted anything.
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
		t.Fatalf("pendingLoadStarts = %v, want the unobserved load plus the spanning one and the selection", got)
	}

	// Wiring the callbacks releases the parked reader into its adoption. The
	// stream is still frozen, so nothing consumes the queue behind it and the
	// length it settles on is the adoption's own decision.
	m.SetEventCallbacks(noopEventCallbacks())
	f.waitFor(t, "the reader to adopt the current load", func() bool {
		return len(pendingLoadStartsOf(m)) != 3
	})
	if got := pendingLoadStartsOf(m); !slices.Equal(got, []uint64{supersededGen, selectedGen}) {
		t.Fatalf("pendingLoadStarts = %v, want %v: adoption retired the load whose command was still "+
			"in flight when this reader dialled, whose start-file it will read",
			got, []uint64{supersededGen, selectedGen})
	}

	// Only now does the reader reach the buffered events, in the order mpv
	// emitted them.
	drainHeldEvents(t, m, f, release)
	// The damage a promotion does lands in its background maintenance, after the
	// decision itself has returned.
	time.Sleep(150 * time.Millisecond)

	if got := currentIdx(m); got != 4 {
		t.Fatalf("currentIndex = %d, want 4: the spanning load's start-file took the newer load's generation", got)
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

// TestEOF_ReaderAdoptionRetiresLoadWhoseEventPredatesDial is the deterministic
// regression for the other end of that boundary: an expectation whose
// start-file was emitted before the reader connected, but whose loadfile was
// answered after.
//
// mpv emits the start-file while it executes a load and replies once it is
// done, so the answer proves the event has fired and nothing proves it has not.
// A load already executing when the reader dials is therefore unanswered at the
// boundary — indistinguishable, by the reply alone, from one still parked
// inside mpv — and adoption keeps its expectation for an event that went to
// sockets this reader never had.
//
// Kept, it is consumed by the first start-file this reader *can* read. That
// load is stamped with the generation of the one before it, so it reads as
// superseded from the instant it begins: the end-file it emits at the end of
// the track is discarded as belonging to a replaced entry, no advance is made,
// and playback stops with mpv idle.
//
// The reply gate is what makes the window a fact rather than a timing: mpv has
// loaded the file and emitted its start-file to nobody, and the Manager learns
// the command returned only after the reader's connection exists.
func TestEOF_ReaderAdoptionRetiresLoadWhoseEventPredatesDial(t *testing.T) {
	t.Run("sequential", func(t *testing.T) {
		m, f := newUnwiredFakePlaylistManager(t, "eof-predates-dial-seq")
		queue := queueOf(t, 5)
		installQueue(m, queue, -1, true)
		readerAdoptionEventPredatesDialScenario(t, m, f, queue)
	})

	// The shuffle half runs the same window with the traversal driving the
	// advance, so the correlation is proved live against a promotion that
	// consumes a shuffle identity rather than the next row.
	t.Run("shuffled", func(t *testing.T) {
		m, f := newUnwiredFakePlaylistManager(t, "eof-predates-dial-shuf")
		queue := queueOf(t, 5)
		m.mu.Lock()
		m.gaplessEnabled = true
		m.mu.Unlock()
		buildShuffleState(t, m, queue, 0, reversePermute)
		readerAdoptionEventPredatesDialScenario(t, m, f, queue)
	})
}

// readerAdoptionEventPredatesDialScenario parks a load's reply inside mpv once
// it has already started playing the file, brings the reader up in that
// interval, and only then lets the command return — so the expectation is
// recorded before the dial, answered after it, and names a start-file that was
// broadcast while no reader existed.
//
// The manager it is given must still have its callbacks unwired: that park is
// what holds startEventReader between its dial and its adoption.
func readerAdoptionEventPredatesDialScenario(t *testing.T, m *Manager, f *fakePlaylist, queue []QueueTrack) {
	t.Helper()

	// Model mpv's broadcast, so "this reader could not have seen it" is a fact
	// about the fake and not merely about timing.
	f.dropUnsubscribedEvents()

	// The load runs inside mpv — its file is current and its start-file is
	// emitted to the sockets that exist, which is none — and then parks on its
	// way back to the Manager.
	gate := f.blockReply(t, "loadfile:replace")
	predating := make(chan error, 1)
	go func() { predating <- m.PlayFromIndex(0) }()
	gate.waitArrived(t, "the reply to the load whose start-file predates the dial")
	if got := f.playingPath(); got != queue[0].Path {
		t.Fatalf("mpv is playing %q, want %q: the load must have executed before its reply parks",
			got, queue[0].Path)
	}

	// The reader connects on top of that unanswered command. Waiting for the
	// stream proves the dial returned and mpv answered the path subscription, so
	// what that subscription put on the stream names the file above.
	go m.startEventReader(m.ctx, nil)
	f.waitForEventStream(t)

	// Only now does the load return, so the Manager marks it sent after the
	// reader's boundary was captured: the reply says nothing about which side of
	// the connection the event fell on.
	f.unblockReply("loadfile:replace")
	if err := <-predating; err != nil {
		t.Fatalf("PlayFromIndex(0): %v", err)
	}
	f.waitForPlayingWithSuccessor(t, queue[0].Path)
	if got := pendingLoadStartsOf(m); len(got) != 1 {
		t.Fatalf("pendingLoadStarts = %v, want the one load recorded before the dial", got)
	}

	// Wiring the callbacks releases the parked reader into its adoption, and the
	// path value already on the stream settles what the reply could not.
	m.SetEventCallbacks(noopEventCallbacks())
	f.waitFor(t, "the reader to retire the expectation whose start-file predates its dial", func() bool {
		return len(pendingLoadStartsOf(m)) == 0
	})

	// A load performed now is the first one this reader can correlate, and it
	// must be stamped with its own generation rather than with the retired one.
	if err := m.PlayFromIndex(2); err != nil {
		t.Fatalf("PlayFromIndex(2): %v", err)
	}
	f.waitForPlayingWithSuccessor(t, queue[2].Path)
	waitForCorrelatedLoad(t, m, f)
	// Wait for the successor to be tracked and counted rather than merely present
	// in mpv's playlist: appendPrefetchLocked records the tracking only after the
	// command returns, and a promotion decided before that lands would take the
	// fallback advance instead.
	f.waitFor(t, "the selection's successor to be tracked as a gapless one", func() bool {
		got, gen := prefetchTracking(m)
		return got.index >= 0 && got.gen == gen && gaplessSuccessorsOf(m) == 1
	})
	successor, _ := prefetchTracking(m)

	// Its end-file is therefore live: mpv moves into the prefetched successor and
	// the queue keeps running instead of stopping at the end of the track.
	f.finishCurrent()
	waitForCurrentIndex(t, m, successor.index)
	if got := f.playingPath(); got != successor.path {
		t.Fatalf("mpv is playing %q, want the promoted successor %q", got, successor.path)
	}
	f.waitFor(t, "the promoted transition to refill the prefetch", func() bool {
		got, gen := prefetchTracking(m)
		return got.index >= 0 && got.index != successor.index && got.gen == gen
	})
}

// TestEOF_ReaderAdoptionOrdersPostDialStartAfterPredialReconciliation is the
// deterministic regression for the two windows overlapping: an expectation whose
// start-file predates the dial, and a *second* load whose start-file arrives
// ahead of the snapshot that was supposed to retire the first.
//
// The snapshot is mpv's answer to the path subscription, and mpv answers it when
// it gets round to the command — behind any loadfile queued in front of it. So a
// load admitted after the reader connects can execute, emit its start-file to
// that reader, and be answered, all before the reader is told which file mpv is
// playing.
//
// Dispatched in arrival order, that start-file consumes the pre-dial expectation
// positionally: the second load is stamped with the generation the first one
// displaced and reads as superseded from the instant it begins, its end-file is
// discarded, and playback stops with mpv idle. Holding the stream until the
// snapshot lands is what keeps each load on its own generation.
func TestEOF_ReaderAdoptionOrdersPostDialStartAfterPredialReconciliation(t *testing.T) {
	t.Run("sequential", func(t *testing.T) {
		m, f := newUnwiredFakePlaylistManager(t, "eof-path-late-seq")
		queue := queueOf(t, 5)
		installQueue(m, queue, -1, true)
		readerAdoptionStartBeforePathScenario(t, m, f, queue)
	})

	// The shuffle half runs the same window with the traversal driving the
	// advance, so the correlation is proved live against a promotion that
	// consumes a shuffle identity rather than the next row.
	t.Run("shuffled", func(t *testing.T) {
		m, f := newUnwiredFakePlaylistManager(t, "eof-path-late-shuf")
		queue := queueOf(t, 5)
		m.mu.Lock()
		m.gaplessEnabled = true
		m.mu.Unlock()
		buildShuffleState(t, m, queue, 0, reversePermute)
		readerAdoptionStartBeforePathScenario(t, m, f, queue)
	})
}

// readerAdoptionStartBeforePathScenario stages the pre-dial load exactly as
// readerAdoptionEventPredatesDialScenario does — executed inside mpv, answered
// only once the reader's connection exists — and then holds mpv's answer to the
// path subscription while a second load runs to completion, so its start-file is
// delivered to the reader ahead of the snapshot.
//
// The manager it is given must still have its callbacks unwired: that park is
// what holds startEventReader between its dial and its adoption, so both loads
// are settled before a single event is dispatched.
func readerAdoptionStartBeforePathScenario(t *testing.T, m *Manager, f *fakePlaylist, queue []QueueTrack) {
	t.Helper()

	// Model mpv's broadcast, so "this reader could not have seen it" is a fact
	// about the fake and not merely about timing.
	f.dropUnsubscribedEvents()

	// The first load runs inside mpv — its file is current and its start-file is
	// emitted to the sockets that exist, which is none — and then parks on its way
	// back to the Manager.
	gate := f.blockReply(t, "loadfile:replace")
	predating := make(chan error, 1)
	go func() { predating <- m.PlayFromIndex(0) }()
	gate.waitArrived(t, "the reply to the load whose start-file predates the dial")
	if got := f.playingPath(); got != queue[0].Path {
		t.Fatalf("mpv is playing %q, want %q: the load must have executed before its reply parks",
			got, queue[0].Path)
	}

	// mpv will not get to the path subscription until this is released, which is
	// the whole window: the reader is connected and has no idea what is playing.
	releasePath := f.holdPathSnapshot(t)

	// The reader connects on top of that unanswered command. Waiting for the
	// subscription to reach the fake proves the dial returned — the boundary is
	// captured between the two — and the unwired callbacks then park the reader
	// before it adopts.
	go m.startEventReader(m.ctx, nil)
	f.waitForEventSubscriber(t)

	// Only now does the first load return, so the Manager marks it sent after the
	// reader's boundary was captured: the reply says nothing about which side of
	// the connection the event fell on.
	f.unblockReply("loadfile:replace")
	if err := <-predating; err != nil {
		t.Fatalf("PlayFromIndex(0): %v", err)
	}
	f.waitForPlayingWithSuccessor(t, queue[0].Path)
	predatingGen := loadGenOf(m)
	if got := pendingLoadStartsOf(m); !slices.Equal(got, []uint64{predatingGen}) {
		t.Fatalf("pendingLoadStarts = %v, want %v: the one load recorded before the dial",
			got, []uint64{predatingGen})
	}

	// The second load runs end to end with the reader connected: mpv executes it,
	// broadcasts its start-file onto the stream, and answers it — all ahead of the
	// still-held snapshot. This is the event the reader reads first.
	if err := m.PlayFromIndex(2); err != nil {
		t.Fatalf("PlayFromIndex(2): %v", err)
	}
	f.waitForPlayingWithSuccessor(t, queue[2].Path)
	selectedGen := loadGenOf(m)
	if got := pendingLoadStartsOf(m); !slices.Equal(got, []uint64{predatingGen, selectedGen}) {
		t.Fatalf("pendingLoadStarts = %v, want %v: the pre-dial load and the one whose start-file "+
			"is already on the stream", got, []uint64{predatingGen, selectedGen})
	}

	// Wiring the callbacks releases the parked reader into its adoption. The reply
	// boundary cannot place the pre-dial expectation, so the reader holds the
	// start-file it can already read instead of letting it consume that
	// expectation.
	m.SetEventCallbacks(noopEventCallbacks())

	// The snapshot names the file the second load made current, and settles the
	// first load's expectation against the start-files read ahead of it. Only then
	// is the held start-file dispatched — onto its own generation.
	releasePath()
	f.waitFor(t, "the reader to settle both loads: one retired, one consumed by its own start-file",
		func() bool { return len(pendingLoadStartsOf(m)) == 0 })
	waitForCorrelatedLoad(t, m, f)
	if got := startedLoadGenOf(m); got != selectedGen {
		t.Fatalf("startedLoadGen = %d, want %d: the post-dial start-file consumed the pre-dial "+
			"expectation and took the generation it displaced", got, selectedGen)
	}
	if got := loadGenOf(m); got != selectedGen {
		t.Fatalf("loadGen = %d, want %d: the selection's own generation must survive the adoption",
			got, selectedGen)
	}

	// Wait for the successor to be tracked and counted rather than merely present
	// in mpv's playlist: appendPrefetchLocked records the tracking only after the
	// command returns, and a promotion decided before that lands would take the
	// fallback advance instead.
	f.waitFor(t, "the selection's successor to be tracked as a gapless one", func() bool {
		got, gen := prefetchTracking(m)
		return got.index >= 0 && got.gen == gen && gaplessSuccessorsOf(m) == 1
	})
	successor, _ := prefetchTracking(m)

	// The second load's end-file is therefore live: mpv moves into the prefetched
	// successor and the queue keeps running instead of stopping at the end of the
	// track.
	f.finishCurrent()
	waitForCurrentIndex(t, m, successor.index)
	if got := f.playingPath(); got != successor.path {
		t.Fatalf("mpv is playing %q, want the promoted successor %q", got, successor.path)
	}
	f.waitFor(t, "the promoted transition to refill the prefetch", func() bool {
		got, gen := prefetchTracking(m)
		return got.index >= 0 && got.index != successor.index && got.gen == gen
	})
}

// TestEOF_GaplessTransitionRemovesCompletedSlotBeforeAppending covers the happy
// path and the ordering inside it: mpv advances to the prefetched entry on its
// own, and the maintenance that follows removes the finished entry before
// appending the next one, so mpv is never asked to hold three entries and slot 1
// keeps naming the successor.
func TestEOF_GaplessTransitionRemovesCompletedSlotBeforeAppending(t *testing.T) {
	m, f := newFakePlaylistManager(t, "eof-order")
	queue := queueOf(t, 4)
	installQueue(m, queue, -1, true)
	runEventReader(t, m, f)

	if err := m.PlayFromIndex(0); err != nil {
		t.Fatalf("PlayFromIndex(0): %v", err)
	}
	f.waitForEntries(t, paths(queue, 0, 1))

	f.finishCurrent()

	waitForCurrentIndex(t, m, 1)
	f.waitForEntries(t, paths(queue, 1, 2))
	if got := f.playingPath(); got != queue[1].Path {
		t.Fatalf("mpv is playing %q, want the promoted successor %q", got, queue[1].Path)
	}
	assertPrefetch(t, m, queue, 2)

	removed := f.commandOrder(isPlaylistRemove)
	appended := f.commandOrder(isAppendOf(queue[2].Path))
	if removed < 0 {
		t.Fatalf("the completed slot was never removed; commands = %v", f.commands())
	}
	if appended < 0 {
		t.Fatalf("the successor was never appended; commands = %v", f.commands())
	}
	if removed > appended {
		t.Fatalf("playlist-remove at %d ran after the append at %d; the completed slot must go first",
			removed, appended)
	}
}
