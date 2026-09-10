package player

import (
	"errors"
	"testing"
)

// This file is the refresh boundary seen from mpv's side: what a refresh is
// allowed to do to a playlist that is producing audio right now.
//
// The rule the tests below pin is narrow and absolute. A refresh may take back
// position 1 — the parked gapless successor — when the plan actually retired the
// row it holds, and it may do nothing else. Position 0 is the track the user is
// listening to; no refresh reloads it, seeks it, or removes it, and no refresh
// re-appends a successor that did not change just because the row moved in the
// queue.
//
// Every case is driven through the fake mpv rather than through timing: the
// playlist is modelled, so "mpv has already started the successor" and "the
// end-file has not been delivered yet" are states the test puts the fake into,
// not races it hopes to hit.

// playingRefreshManager starts a gapless traversal on the first row of an n-row
// queue and waits until mpv holds the current entry plus its parked successor.
func playingRefreshManager(t *testing.T, name string, n int) (*Manager, *fakePlaylist, []QueueTrack) {
	t.Helper()
	m, f := newFakePlaylistManager(t, name)
	queue := queueOf(t, n)
	installQueue(m, queue, -1, true)

	if err := m.PlayFromIndex(0); err != nil {
		t.Fatalf("PlayFromIndex(0): %v", err)
	}
	f.waitForEntries(t, paths(queue, 0, 1))
	assertPrefetch(t, m, queue, 1)
	return m, f, queue
}

// assertCurrentTrackUntouched is the invariant every case here shares: whatever
// the refresh did to the successor slot, the row that was playing is still
// playing and was never handed to mpv a second time.
func assertCurrentTrackUntouched(t *testing.T, f *fakePlaylist, want string) {
	t.Helper()
	if got := f.playingPath(); got != want {
		t.Fatalf("mpv is playing %q, want the uninterrupted current row %q", got, want)
	}
	if got := f.count(isLoadfileReplace); got != 1 {
		t.Fatalf("mpv received %d loadfile replace commands, want only the one that started playback: %v",
			got, f.commands())
	}
}

// TestApplyQueueRefresh_LeavesAnUnaffectedSuccessorInMpvsPlaylist pins the
// cheapest and most common case: the refresh swapped a row further down the
// queue and the parked successor is not one of them.
//
// Dropping the prefetch whenever anything changed would be defensible and is
// what the boundary did first, but it costs a playlist-remove and a re-append
// for a successor that is byte-for-byte the one already parked — a round trip
// mpv absorbs while a track is playing, and one more chance to address the wrong
// slot. Rebinding makes it cost nothing at all.
func TestApplyQueueRefresh_LeavesAnUnaffectedSuccessorInMpvsPlaylist(t *testing.T) {
	m, f, queue := playingRefreshManager(t, "refgap-keep", 3)
	fresh := refreshQueue(t, 10)

	snap := m.CaptureQueueRefresh()
	res, err := m.ApplyQueueRefresh(buildRefresh(snap, queue, []refreshSwap{swap(3, fresh[0])}))
	if err != nil {
		t.Fatalf("ApplyQueueRefresh: %v", err)
	}
	if res.PrefetchDropped || res.PrefetchDeferred {
		t.Fatalf("result = %+v, want the untouched successor neither dropped nor deferred", res)
	}

	// Apply schedules the reschedule on its own goroutine; running it here too
	// makes "it sends nothing" an assertion rather than a hope.
	m.prefetchNext()

	if got := f.count(isPlaylistRemove); got != 0 {
		t.Fatalf("the refresh sent %d playlist-remove commands for a successor it did not name: %v",
			got, f.commands())
	}
	if got := f.count(isLoadfileAppend); got != 1 {
		t.Fatalf("mpv received %d appends, want only the original prefetch: %v", got, f.commands())
	}
	assertCurrentTrackUntouched(t, f, queue[0].Path)
	f.waitForEntries(t, paths(queue, 0, 1))
	assertPrefetch(t, m, managerQueue(m), 1)
}

// TestApplyQueueRefresh_RebindsAShiftedSuccessorInsteadOfReappending is the same
// guarantee where it is not free: the successor survived the refresh but its
// queue position did not.
//
// The prefetch tracking is positional, so retiring rows ahead of the parked
// successor renames the slot it points at. Under shuffle the successor is a
// traversal identity that can sit anywhere, which is what makes the shift
// observable. Left alone the tracking would name a different row — or fall off
// the end of the queue — and the next prefetch would remove and re-append a file
// mpv is already holding.
func TestApplyQueueRefresh_RebindsAShiftedSuccessorInsteadOfReappending(t *testing.T) {
	m, f := newFakePlaylistManager(t, "refgap-shift")
	queue := queueOf(t, 5)
	installQueue(m, queue, -1, true)
	m.SetMissingMarker(&countingMarker{})
	m.SetPermuteFunc(reversePermute)
	if err := m.SetShuffle(true); err != nil {
		t.Fatalf("SetShuffle(true): %v", err)
	}
	if err := m.PlayFromIndex(0); err != nil {
		t.Fatalf("PlayFromIndex(0): %v", err)
	}
	assertEligible(t, m, 5, 4, 3, 2)
	f.waitForEntries(t, paths(queue, 0, 4))
	assertPrefetch(t, m, queue, 4)

	// Retiring rows 4 and 3 moves row 5 from queue index 4 to index 2 without
	// touching its turn in the cycle.
	snap := m.CaptureQueueRefresh()
	res, err := m.ApplyQueueRefresh(buildRefresh(snap, queue, []refreshSwap{retire(4), retire(3)}))
	if err != nil {
		t.Fatalf("ApplyQueueRefresh: %v", err)
	}
	if res.Retired != 2 || res.PrefetchDropped || res.PrefetchDeferred {
		t.Fatalf("result = %+v, want 2 retired slots and an untouched successor", res)
	}
	assertEligible(t, m, 5, 2)

	m.prefetchNext()

	if got := f.count(isPlaylistRemove); got != 0 {
		t.Fatalf("the refresh sent %d playlist-remove commands for a successor that only moved: %v",
			got, f.commands())
	}
	if got := f.count(isLoadfileAppend); got != 1 {
		t.Fatalf("mpv received %d appends, want only the original prefetch: %v", got, f.commands())
	}
	assertCurrentTrackUntouched(t, f, queue[0].Path)
	f.waitForEntries(t, paths(queue, 0, 4))
	// Same file, same mpv slot, new queue index.
	assertPrefetch(t, m, managerQueue(m), 2)
}

// TestApplyQueueRefresh_DropsAnObsoleteSuccessorWithoutInterruptingPlayback is
// the other half: the refresh did name the parked row, so mpv must not be left
// holding it.
func TestApplyQueueRefresh_DropsAnObsoleteSuccessorWithoutInterruptingPlayback(t *testing.T) {
	m, f, queue := playingRefreshManager(t, "refgap-drop", 3)
	fresh := refreshQueue(t, 10)

	snap := m.CaptureQueueRefresh()
	res, err := m.ApplyQueueRefresh(buildRefresh(snap, queue, []refreshSwap{swap(2, fresh[0])}))
	if err != nil {
		t.Fatalf("ApplyQueueRefresh: %v", err)
	}
	if !res.PrefetchDropped || res.PrefetchDeferred {
		t.Fatalf("result = %+v, want the retired successor dropped from mpv's playlist", res)
	}

	next := managerQueue(m)
	// The refreshed queue is [1, 3, 10]: the successor is now row 3.
	f.waitForEntries(t, []string{queue[0].Path, next[1].Path})
	assertCurrentTrackUntouched(t, f, queue[0].Path)
	assertPrefetch(t, m, next, 1)
}

// TestApplyQueueRefresh_RetainsAnUnremovableSuccessorForALaterRetry covers the
// post-commit failure the staged design accepts: the queue is published, and the
// one piece of maintenance left cannot reach mpv.
//
// Nothing is rolled back. The refreshed queue is what the database holds and
// what the traversal reads, so it stays authoritative; mpv is simply behind it
// by one stale playlist entry, and the tracking is kept so the next transaction
// removes it. Reporting the removal as done and forgetting the entry is the
// failure this pins — that leaks a slot mpv would eventually play.
func TestApplyQueueRefresh_RetainsAnUnremovableSuccessorForALaterRetry(t *testing.T) {
	m, f, queue := playingRefreshManager(t, "refgap-fail", 3)
	fresh := refreshQueue(t, 10)
	f.failCommand("playlist-remove", "mpv rejected the removal")

	snap := m.CaptureQueueRefresh()
	res, err := m.ApplyQueueRefresh(buildRefresh(snap, queue, []refreshSwap{swap(2, fresh[0])}))
	if err != nil {
		t.Fatalf("ApplyQueueRefresh: %v", err)
	}
	if !res.PrefetchDeferred || res.PrefetchDropped {
		t.Fatalf("result = %+v, want the removal reported as deferred", res)
	}

	next := managerQueue(m)
	if len(next) != 3 || next[0].QueueID != 1 || next[1].QueueID != 3 || next[2].QueueID != 10 {
		t.Fatalf("queue = %v, want the refreshed rows to stay authoritative", rowsOf(next))
	}
	assertCurrentTrackUntouched(t, f, queue[0].Path)

	// The stale entry is still mpv's, and the tracking still names it, which is
	// the only thing that will make a later transaction try again. The
	// assertion settles rather than samples: a removal drops the tracking
	// before the send and restores it on failure, so the reschedule Apply
	// spawned is briefly holding it.
	f.waitForEntries(t, paths(queue, 0, 1))
	assertPrefetch(t, m, queue, 1)

	// The later retry: mpv accepts the removal and the successor is replaced by
	// the row the refresh put in its place.
	f.clearFailure("playlist-remove")
	m.prefetchNext()
	f.waitForEntries(t, []string{queue[0].Path, next[1].Path})
	assertPrefetch(t, m, next, 1)
	assertCurrentTrackUntouched(t, f, queue[0].Path)
}

// TestApplyQueueRefresh_PublishesAcrossADisconnectAndConvergesOnRestart is the
// same deferral where the transport itself is gone.
//
// This is the case that decides whether the durable queue or mpv's playlist is
// the authority. It has to be the queue: the batch is committed, a restart
// reloads it, and refusing to publish it in memory because a playlist-remove
// could not be delivered would leave the traversal playing rows the database no
// longer has. So the publication succeeds, and the recovery that reconnects is
// what brings mpv back in line.
func TestApplyQueueRefresh_PublishesAcrossADisconnectAndConvergesOnRestart(t *testing.T) {
	m, f, queue := playingRefreshManager(t, "refgap-disc", 3)
	fresh := refreshQueue(t, 10)

	m.mu.RLock()
	client := m.client
	m.mu.RUnlock()
	if client == nil {
		t.Fatal("manager has no command client to disconnect")
	}
	if err := client.Close(); err != nil {
		t.Fatalf("close command transport: %v", err)
	}

	snap := m.CaptureQueueRefresh()
	res, err := m.ApplyQueueRefresh(buildRefresh(snap, queue, []refreshSwap{swap(2, fresh[0])}))
	if err != nil {
		t.Fatalf("ApplyQueueRefresh across a closed transport: %v", err)
	}
	if !res.PrefetchDeferred {
		t.Fatalf("result = %+v, want the undeliverable removal reported as deferred", res)
	}
	next := managerQueue(m)
	if len(next) != 3 || next[1].QueueID != 3 || next[2].QueueID != 10 {
		t.Fatalf("queue = %v, want the refreshed rows published despite the disconnect", rowsOf(next))
	}
	if got := currentQueueID(m); got != 1 {
		t.Fatalf("current row = %d, want 1: a disconnect must not move playback", got)
	}
	if got := f.count(isPlaylistRemove); got != 0 {
		t.Fatalf("closed transport delivered %d playlist-remove commands, want 0", got)
	}

	// attemptRestart calls the settlement hook immediately after reaping the old
	// process; this fake has no subprocess, so model that result directly.
	f.mu.Lock()
	f.current = -1
	f.entries = nil
	f.mu.Unlock()

	// The TUI persists the position on a timer, so the row that was playing is
	// what the supervisor finds to resume.
	m.SetCurrentPosition(queue[0].Path, 7.5)
	restore := m.snapshotRestore()
	if restore.path != queue[0].Path || !restore.liveness.live() {
		t.Fatalf("restore = %+v, want a live resume of %q", restore, queue[0].Path)
	}
	restart := restartOnto(t, m, restore)

	// The fresh process is handed the row that never stopped playing and the
	// successor the refresh chose — never the row it retired.
	restart.waitForEntries(t, []string{queue[0].Path, next[1].Path})
	assertPrefetch(t, m, next, 1)
}

// TestApplyQueueRefresh_DoesNotRemoveASuccessorMpvHasSilentlyStarted is the
// delayed-event case, and the one that cannot be reasoned about from the
// Manager's state at all.
//
// mpv owns its playlist. When the current file ends it moves into the appended
// successor immediately; the end-file that says so travels over a socket and is
// read whenever the reader gets to it. In between, every liveness flag still
// reads "live" and currentIndex still names the row that finished — so a refresh
// that consulted them would conclude position 1 is a parked successor and send a
// playlist-remove that stops the audio.
//
// startedSuccessorLocked closes this window once the end-file has been read.
// What closes it before that is not knowing better: a refresh that did not name
// the tracked row never addresses the slot in the first place, so the delayed
// event changes nothing, and the promotion it eventually triggers finds its
// successor exactly where it left it.
func TestApplyQueueRefresh_DoesNotRemoveASuccessorMpvHasSilentlyStarted(t *testing.T) {
	// The event reader is the point of this case, so it runs for real: the hold
	// delays delivery rather than standing in for it.
	m, f := newFakePlaylistManager(t, "refgap-delay")
	runEventReader(t, m, f)
	queue := queueOf(t, 3)
	installQueue(m, queue, -1, true)
	if err := m.PlayFromIndex(0); err != nil {
		t.Fatalf("PlayFromIndex(0): %v", err)
	}
	f.waitForEntries(t, paths(queue, 0, 1))
	assertPrefetch(t, m, queue, 1)

	release := f.holdEvents(t)

	// mpv finishes the current entry and starts the successor. The end-file is
	// on the wire; the Manager has not seen it.
	f.finishCurrent()
	f.waitFor(t, "mpv to start the appended successor", func() bool {
		return f.playingPath() == queue[1].Path
	})
	if got := livenessOf(m); got != livenessLive {
		t.Fatalf("liveness = %v, want live: the end-file has not been delivered yet", got)
	}

	fresh := refreshQueue(t, 10)
	snap := m.CaptureQueueRefresh()
	res, err := m.ApplyQueueRefresh(buildRefresh(snap, queue, []refreshSwap{swap(3, fresh[0])}))
	if err != nil {
		t.Fatalf("ApplyQueueRefresh: %v", err)
	}
	if res.PrefetchDropped || res.PrefetchDeferred {
		t.Fatalf("result = %+v, want the untouched successor left in mpv's playlist", res)
	}
	m.prefetchNext()

	if got := f.count(isPlaylistRemove); got != 0 {
		t.Fatalf("the refresh sent %d playlist-remove commands while mpv was already playing "+
			"position 1: %v", got, f.commands())
	}
	if got := f.playingPath(); got != queue[1].Path {
		t.Fatalf("mpv is playing %q, want the successor %q it had started gaplessly", got, queue[1].Path)
	}

	// The held end-file is delivered at last: the transition finds the tracking
	// the refresh rebound and promotes the row mpv really started.
	release()
	f.waitFor(t, "the promotion to remove the completed slot", func() bool {
		return f.currentPos() == 0 && f.playingPath() == queue[1].Path
	})
	if got := currentIdx(m); got != 1 {
		t.Fatalf("currentIndex = %d, want 1: the delayed transition was not promoted", got)
	}
	next := managerQueue(m)
	f.waitForEntries(t, []string{queue[1].Path, next[2].Path})
	assertPrefetch(t, m, next, 2)
}

// TestApplyQueueRefresh_RefusesAPlanThatNamesThePromotedRow is the successor
// promotion seen from the refresh side.
//
// A capture taken while an end-file is unresolved describes the row that
// finished as the one playing, and the row mpv has already moved into as an
// ordinary upcoming slot — so a plan built from it would hand over the entry
// coming out of the speakers. Both refusals below are the same guarantee at
// different moments: during the transition because nothing can be checked
// against a timeline still in motion, and after it because the snapshot the plan
// was built from no longer describes anything.
func TestApplyQueueRefresh_RefusesAPlanThatNamesThePromotedRow(t *testing.T) {
	w := openEOFPendingWindow(t, "refgap-promo")
	fresh := refreshQueue(t, 10)

	snap := w.m.CaptureQueueRefresh()
	if got := snap.Current.QueueID; got != 1 {
		t.Fatalf("captured current row = %d, want the finished row 1: the pending transition "+
			"has not moved the cursor yet", got)
	}
	// Row 2 is the entry mpv is already playing, whatever this capture says.
	plan := buildRefresh(snap, w.queue, []refreshSwap{swap(2, fresh[0])})

	if _, err := w.m.ApplyQueueRefresh(plan); !errors.Is(err, ErrRefreshTransitionPending) {
		t.Fatalf("ApplyQueueRefresh during the pending end = %v, want ErrRefreshTransitionPending", err)
	}
	w.assertSuccessorIntact(t)

	// The transition resolves onto that same row.
	w.m.dispatchEOF(w.state)
	w.f.waitFor(t, "the promotion to remove the completed slot", func() bool {
		return w.f.currentPos() == 0 && w.f.playingPath() == w.queue[1].Path
	})
	if got := currentIdx(w.m); got != 1 {
		t.Fatalf("currentIndex = %d, want the promoted row", got)
	}

	// The plan is now a description of a timeline that no longer exists, and the
	// slot it names is playing.
	if _, err := w.m.ApplyQueueRefresh(plan); !errors.Is(err, ErrRefreshStale) {
		t.Fatalf("ApplyQueueRefresh after the promotion = %v, want ErrRefreshStale", err)
	}
	if got := currentQueueID(w.m); got != 2 {
		t.Fatalf("current row = %d, want the promoted row 2 still in the queue", got)
	}
	if got := w.f.playingPath(); got != w.queue[1].Path {
		t.Fatalf("mpv is playing %q, want the promoted row %q", got, w.queue[1].Path)
	}
	if got := w.f.count(isLoadfileReplace); got != 1 {
		t.Fatalf("mpv received %d loadfile replace commands, want only the one that started "+
			"playback: %v", got, w.f.commands())
	}
}

// rowsOf names a queue by its row IDs, for assertion messages.
func rowsOf(queue []QueueTrack) []int64 {
	out := make([]int64, len(queue))
	for i, row := range queue {
		out[i] = row.QueueID
	}
	return out
}
