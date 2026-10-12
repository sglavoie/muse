package player

import (
	"fmt"
	"slices"
	"testing"
	"time"
)

// TestEOF_BufferedStartAndEOFAfterReplacementDoesNotAdvance is the deterministic
// regression for the ordering the test above deliberately excludes: it waits for
// the playing entry to be correlated *before* it holds the socket, so the
// start-file of the gapless successor is always dispatched while the generation
// it belongs to is still live.
//
// Here the hold goes on first, so the successor's start-file is buffered too and
// is only read once the replacement has been acknowledged. Stamping it with the
// generation live at that later instant hands the discarded entry the
// selection's generation, and the *second* buffered end-file — emitted by that
// same superseded entry — then reads as live. Nothing in the state it is decided
// against says otherwise: the promotion takes the selection's own prefetch as
// the successor that just started, moves the current row onto it, and runs the
// completed-slot cleanup against the selection's playlist, deleting the row mpv
// is actually playing.
//
// Both events belong to entries the replacement discarded, so both must decide
// nothing: the selected row stands and so does the successor prefetched for it.
func TestEOF_BufferedStartAndEOFAfterReplacementDoesNotAdvance(t *testing.T) {
	m, f := newFakePlaylistManager(t, "eof-buffered-start")
	queue := queueOf(t, 5)
	installQueue(m, queue, -1, true)
	runEventReader(t, m, f)

	if err := m.PlayFromIndex(0); err != nil {
		t.Fatalf("PlayFromIndex(0): %v", err)
	}
	f.waitForEntries(t, paths(queue, 0, 1))
	assertPrefetch(t, m, queue, 1)
	waitForCorrelatedLoad(t, m, f)

	// Freeze the event socket. Every event below is buffered behind it,
	// start-files included.
	release := f.holdEvents(t)

	// queue[0] plays out and mpv moves onto the prefetched queue[1] on its own:
	// end-file/eof plus start-file, both unread.
	f.finishCurrent()
	// queue[1] plays out too. Nothing was prefetched behind it — the reader has
	// not seen the transition yet, so no maintenance ran — and mpv goes idle
	// after a second unread end-file/eof.
	f.finishCurrent()

	// The user selects a different row while all three events are still in the
	// pipe. The selection runs end to end — playlist replaced, generation
	// bumped, window closed — and its own successor is prefetched behind it.
	if err := m.PlayFromIndex(3); err != nil {
		t.Fatalf("PlayFromIndex(3): %v", err)
	}
	f.waitForEntries(t, paths(queue, 3, 4))
	assertPrefetch(t, m, queue, 4)
	removalsBefore := f.count(isPlaylistRemove)
	replacesBefore := f.count(isLoadfileReplace)

	// Only now does the reader get to the discarded entries' events, in the
	// order mpv emitted them.
	drainHeldEvents(t, m, f, release)
	// The damage a promotion does lands in its background maintenance, after
	// the decision itself has returned.
	time.Sleep(150 * time.Millisecond)

	if got := currentIdx(m); got != 3 {
		t.Fatalf("currentIndex = %d, want 3: a buffered start-file re-armed the end-file that followed it", got)
	}
	if got := f.playingPath(); got != queue[3].Path {
		t.Fatalf("mpv is playing %q, want the selected %q", got, queue[3].Path)
	}
	if got := f.entriesSnapshot(); !slices.Equal(got, paths(queue, 3, 4)) {
		t.Fatalf("entries = %v, want %v: the selected row and its successor", got, paths(queue, 3, 4))
	}
	assertPrefetch(t, m, queue, 4)
	if got := f.count(isPlaylistRemove); got != removalsBefore {
		t.Fatalf("playlist-remove count = %d, want %d: the superseded EOF ran its completed-slot cleanup "+
			"against the selection's playlist", got, removalsBefore)
	}
	if got := f.count(isLoadfileReplace); got != replacesBefore {
		t.Fatalf("loadfile replace count = %d, want %d: the superseded EOF sent an advance of its own",
			got, replacesBefore)
	}
	// The correlation must end up on the generation the selection loaded, or
	// the next genuine end-file would be discarded and the queue would stall.
	waitForCorrelatedLoad(t, m, f)
}

// TestEOF_BufferedLoadStartAcrossAReplacementKeepsItsOwnGeneration is the other
// face of the same finding. Above it is a *gapless* start-file that survives a
// replacement; here it is the start-file of a load, buffered while a second load
// supersedes the first.
//
// mpv answers loads in the order they were sent, so the generation of a
// start-file is fixed when the loadfile goes out, not when the reader gets to
// it. Reading the live generation instead hands the first load's entry the
// second load's stamp, and the end-file that entry emitted before it was
// discarded then reads as live against the row the user just selected.
func TestEOF_BufferedLoadStartAcrossAReplacementKeepsItsOwnGeneration(t *testing.T) {
	m, f := newFakePlaylistManager(t, "eof-buffered-load-start")
	queue := queueOf(t, 5)
	installQueue(m, queue, -1, true)
	runEventReader(t, m, f)

	if err := m.PlayFromIndex(0); err != nil {
		t.Fatalf("PlayFromIndex(0): %v", err)
	}
	f.waitForEntries(t, paths(queue, 0, 1))
	waitForCorrelatedLoad(t, m, f)

	release := f.holdEvents(t)

	// A first selection lands in mpv, but its start-file stays on the socket.
	if err := m.PlayFromIndex(1); err != nil {
		t.Fatalf("PlayFromIndex(1): %v", err)
	}
	f.waitForEntries(t, paths(queue, 1, 2))

	// It then plays out, so its end-file is buffered behind its own start-file.
	f.finishCurrent()

	// A second selection supersedes all of it and prefetches its own successor.
	if err := m.PlayFromIndex(3); err != nil {
		t.Fatalf("PlayFromIndex(3): %v", err)
	}
	f.waitForEntries(t, paths(queue, 3, 4))
	assertPrefetch(t, m, queue, 4)
	removalsBefore := f.count(isPlaylistRemove)
	replacesBefore := f.count(isLoadfileReplace)

	drainHeldEvents(t, m, f, release)
	time.Sleep(150 * time.Millisecond)

	if got := currentIdx(m); got != 3 {
		t.Fatalf("currentIndex = %d, want 3: a buffered load start-file took the newer load's generation", got)
	}
	if got := f.playingPath(); got != queue[3].Path {
		t.Fatalf("mpv is playing %q, want the selected %q", got, queue[3].Path)
	}
	if got := f.entriesSnapshot(); !slices.Equal(got, paths(queue, 3, 4)) {
		t.Fatalf("entries = %v, want %v: the selected row and its successor", got, paths(queue, 3, 4))
	}
	assertPrefetch(t, m, queue, 4)
	if got := f.count(isPlaylistRemove); got != removalsBefore {
		t.Fatalf("playlist-remove count = %d, want %d: the superseded EOF ran its completed-slot cleanup "+
			"against the selection's playlist", got, removalsBefore)
	}
	if got := f.count(isLoadfileReplace); got != replacesBefore {
		t.Fatalf("loadfile replace count = %d, want %d: the superseded EOF sent an advance of its own",
			got, replacesBefore)
	}
	waitForCorrelatedLoad(t, m, f)
}

// formerPendingLoadStartLimit is the fixed length the pending-load queue used to
// be trimmed to. The scenario below has to reproduce the exact arithmetic that
// trimming produced, so it names the historical number: there is no live
// constant left to read, which is the point of the repair.
const formerPendingLoadStartLimit = 8

// waitForPlayingWithSuccessor blocks until mpv holds exactly path plus one
// appended successor behind it, without naming which successor. Under shuffle
// the traversal picks that row, and this scenario only needs the two-entry shape
// a gapless transition depends on.
func (f *fakePlaylist) waitForPlayingWithSuccessor(t *testing.T, path string) {
	t.Helper()
	f.waitFor(t, fmt.Sprintf("playlist %s plus its successor", path), func() bool {
		got := f.entriesSnapshot()
		return len(got) == 2 && got[0] == path
	})
}

// TestEOF_BufferedLoadStartsBeyondCorrelationLimitKeepTheirGenerations is the
// deterministic regression for a pending-load queue trimmed to a fixed length.
//
// The correlation between a start-file and the load that emitted it is
// positional: the k-th entry of the queue names the generation of the k-th
// start-file still to be read. Dropping the oldest entry to bound the queue
// therefore does not lose one correlation, it shifts every remaining one — and
// accepted replacements are bounded by nothing the event reader does, so a user
// clicking through a queue faster than the socket drains overflows it.
//
// Once it does, the shift walks the live generation backwards onto an entry a
// replacement already discarded: with the old cap of eight, the eighth buffered
// start-file takes the ninth load's stamp. The end-file that entry emitted
// before it was discarded then passes eofState.superseded and is acted on in
// full — it promotes the selection's own prefetch as if it were the successor
// that had just started, moves the current row onto it, and runs the
// completed-slot cleanup against the selection's playlist, deleting the row mpv
// is actually playing.
//
// Every one of those events belongs to a superseded entry, so all of them must
// decide nothing: the last selection stands, and so do its prefetch, mpv's
// playlist, and the traversal position.
func TestEOF_BufferedLoadStartsBeyondCorrelationLimitKeepTheirGenerations(t *testing.T) {
	t.Run("sequential", func(t *testing.T) {
		m, f := newFakePlaylistManager(t, "eof-load-overflow-seq")
		queue := queueOf(t, formerPendingLoadStartLimit+4)
		installQueue(m, queue, -1, true)
		runEventReader(t, m, f)
		bufferedLoadStartsOverflowScenario(t, m, f, queue)
	})

	// The shuffle half moves the traversal rather than the visible index: a
	// promotion here consumes an identity onto the cycle the last selection
	// reanchored, leaving a Previous entry that was never heard.
	t.Run("shuffled", func(t *testing.T) {
		m, f := newFakePlaylistManager(t, "eof-load-overflow-shuf")
		queue := queueOf(t, formerPendingLoadStartLimit+4)
		m.mu.Lock()
		m.gaplessEnabled = true
		m.mu.Unlock()
		buildShuffleState(t, m, queue, 0, reversePermute)
		runEventReader(t, m, f)
		bufferedLoadStartsOverflowScenario(t, m, f, queue)
	})
}

// bufferedLoadStartsOverflowScenario buffers more accepted loads than the old
// pending-load cap allowed, lets one of the superseded entries end while they
// are all still unread, and then drains the socket against the selection that
// displaced them.
func bufferedLoadStartsOverflowScenario(t *testing.T, m *Manager, f *fakePlaylist, queue []QueueTrack) {
	t.Helper()

	if err := m.PlayFromIndex(0); err != nil {
		t.Fatalf("PlayFromIndex(0): %v", err)
	}
	f.waitForPlayingWithSuccessor(t, queue[0].Path)
	waitForCorrelatedLoad(t, m, f)
	// The successor is only counted against the correlated entry once both the
	// append and the start-file have landed, in either order. Waiting for the
	// count settles that race, so the buffered events below are dispatched from
	// one known starting state.
	f.waitFor(t, "the prefetched successor to be counted against the correlated entry", func() bool {
		return gaplessSuccessorsOf(m) == 1
	})

	// Freeze the event socket: from here mpv keeps accepting loads and keeps
	// emitting, and the reader sees none of it.
	release := f.holdEvents(t)

	// Exactly as many selections as the old cap. Their start-files fill the
	// pending queue without overflowing it, and not one of them is read.
	for i := 1; i <= formerPendingLoadStartLimit; i++ {
		if err := m.PlayFromIndex(i); err != nil {
			t.Fatalf("PlayFromIndex(%d): %v", i, err)
		}
		f.waitForPlayingWithSuccessor(t, queue[i].Path)
	}

	// The last of them plays out, so the end-file of a superseded entry now sits
	// behind every one of those unread start-files.
	f.finishCurrent()

	// One more selection overflows the old cap and supersedes all of it. It runs
	// end to end — playlist replaced, generation bumped, window closed — and its
	// own successor is prefetched behind it.
	last := formerPendingLoadStartLimit + 1
	if err := m.PlayFromIndex(last); err != nil {
		t.Fatalf("PlayFromIndex(%d): %v", last, err)
	}
	f.waitForPlayingWithSuccessor(t, queue[last].Path)
	// The append reaches mpv before the tracking that describes it is recorded,
	// so the playlist holding two entries is not yet proof that the Manager has
	// a promotable successor. Wait for the tracking itself before reading the
	// state this scenario must find unchanged.
	f.waitFor(t, "the selection's own successor to be tracked", func() bool {
		got, gen := prefetchTracking(m)
		return got.index >= 0 && got.gen == gen
	})

	wantEntries := f.entriesSnapshot()
	wantPrefetch, _ := prefetchTracking(m)
	removalsBefore := f.count(isPlaylistRemove)
	replacesBefore := f.count(isLoadfileReplace)

	// Only now does the reader reach the discarded entries' events, in the order
	// mpv emitted them.
	drainHeldEvents(t, m, f, release)
	// The damage a promotion does lands in its background maintenance, after the
	// decision itself has returned.
	time.Sleep(150 * time.Millisecond)

	if got := currentIdx(m); got != last {
		t.Fatalf("currentIndex = %d, want %d: a buffered load start-file beyond the old cap "+
			"took a newer load's generation", got, last)
	}
	if got := f.playingPath(); got != queue[last].Path {
		t.Fatalf("mpv is playing %q, want the selected %q", got, queue[last].Path)
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

// TestEOF_BufferedOldEventAfterReplacementDoesNotMutateShuffle is the shuffle
// half of the same ordering. The traversal, not the visible index, is what a
// promotion moves here: committing the successor the discarded entry had
// resolved grafts an unplayed identity onto the cycle the selection reanchored,
// leaving a Previous entry that was never heard.
func TestEOF_BufferedOldEventAfterReplacementDoesNotMutateShuffle(t *testing.T) {
	m, f := newFakePlaylistManager(t, "eof-buffered-shuffle")
	queue := queueOf(t, 4)
	m.mu.Lock()
	m.gaplessEnabled = true
	m.mu.Unlock()
	buildShuffleState(t, m, queue, 0, reversePermute)
	runEventReader(t, m, f)

	// reversePermute leaves index 3 first in the unseen pool, so the anchor's
	// prefetched successor is queue[3].
	if err := m.PlayFromIndex(0); err != nil {
		t.Fatalf("PlayFromIndex(0): %v", err)
	}
	f.waitForEntries(t, paths(queue, 0, 3))
	waitForCorrelatedLoad(t, m, f)

	release := f.holdEvents(t)
	f.finishCurrent()

	// Reanchoring around index 1 resets the cycle; the identity the old entry
	// was on its way to must not be consumed by it.
	if err := m.PlayFromIndex(1); err != nil {
		t.Fatalf("PlayFromIndex(1): %v", err)
	}
	f.waitForEntries(t, paths(queue, 1, 3))

	drainHeldEvents(t, m, f, release)
	time.Sleep(150 * time.Millisecond)

	history, pos, _ := shuffleSnapshotForTest(m)
	if len(history) != 1 || pos != 0 {
		t.Fatalf("shuffleHistory = %v pos = %d, want the reanchored selection alone: "+
			"a buffered EOF committed onto the new cycle", history, pos)
	}
	if got := currentIdx(m); got != 1 {
		t.Fatalf("currentIndex = %d, want 1", got)
	}
	if got := f.playingPath(); got != queue[1].Path {
		t.Fatalf("mpv is playing %q, want the selected %q", got, queue[1].Path)
	}
	if got := f.entriesSnapshot(); !slices.Equal(got, paths(queue, 1, 3)) {
		t.Fatalf("entries = %v, want %v: the selected row and the successor its cycle resolves",
			got, paths(queue, 1, 3))
	}
}

// TestEOF_RejectedReplacementKeepsTheNextEOFLive is the other side of the
// correlation: a window that opens is not yet a replacement. mpv rejected the
// loadfile, so it never stopped playing the entry it had — that entry is still
// the live one, and its own end-file must still be acted on. A correlation that
// only ever moved forward would strand it one generation behind and turn a
// failed selection into a queue that stops at the end of the current track.
func TestEOF_RejectedReplacementKeepsTheNextEOFLive(t *testing.T) {
	m, f := newFakePlaylistManager(t, "eof-rejected-replace")
	queue := queueOf(t, 3)
	installQueue(m, queue, -1, true)
	runEventReader(t, m, f)

	if err := m.PlayFromIndex(0); err != nil {
		t.Fatalf("PlayFromIndex(0): %v", err)
	}
	f.waitForEntries(t, paths(queue, 0, 1))
	assertPrefetch(t, m, queue, 1)
	waitForCorrelatedLoad(t, m, f)

	// The selection is refused by mpv, which leaves its playlist — and so the
	// entry the next end-file will belong to — exactly as it was.
	f.failCommand("loadfile:replace", "device error")
	if err := m.PlayFromIndex(2); err == nil {
		t.Fatal("PlayFromIndex(2) succeeded; the fake was told to reject the load")
	}
	f.clearFailure("loadfile:replace")
	if got := currentIdx(m); got != 0 {
		t.Fatalf("currentIndex = %d, want 0: a rejected load must not move the queue position", got)
	}
	if got := f.playingPath(); got != queue[0].Path {
		t.Fatalf("mpv is playing %q, want the unchanged %q", got, queue[0].Path)
	}

	// The track it never stopped playing now ends, and the gapless successor
	// mpv already holds must be promoted as usual.
	f.finishCurrent()

	waitForCurrentIndex(t, m, 1)
	if got := f.playingPath(); got != queue[1].Path {
		t.Fatalf("mpv is playing %q, want the promoted successor %q", got, queue[1].Path)
	}
	f.waitForEntries(t, paths(queue, 1, 2))
	assertPrefetch(t, m, queue, 2)
}
