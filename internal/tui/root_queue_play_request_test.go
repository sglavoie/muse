package tui

import (
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/tui/playback"
)

// This file covers what happens when Enter outruns the player. Every play and
// replay the queue panel triggers is answered by a goroutine, so two presses can
// be in flight at once and their outcomes can arrive in either order. Row
// identity alone cannot separate them — repeated Enter aims every request at the
// same queue occurrence — so each request carries its own identity, and only the
// newest one is allowed to decide what the playback state is.
//
// The companion file root_queue_replay_test.go covers the settled case, where
// each press is answered before the next one is made.

// errRefusedPlay stands in for the player refusing a load or a replay.
var errRefusedPlay = errors.New("mpv refused the request")

// replayDisplayRow is where the playing row of replayModel([]int{3, 1, 4, 0, 2}, 2)
// is drawn: the panel follows the traversal, so the cursor position is the
// display row.
const replayDisplayRow = 2

// otherDisplayRow draws queue row 2 in that same model — a row that is not
// playing, so selecting it is a genuine new selection.
const otherDisplayRow = 4

// assertStillPlayingRow4 checks the model kept the confirmed row 4 state that a
// superseded failure must not be allowed to tear down.
func assertStillPlayingRow4(t *testing.T, m Model) {
	t.Helper()
	if m.playback.state != playback.StatePlaying {
		t.Errorf("state = %v, want playing — a superseded failure must not reset", m.playback.state)
	}
	if m.playback.currentIndex != 4 {
		t.Errorf("currentIndex = %d, want 4", m.playback.currentIndex)
	}
	if m.playback.currentHash != m.queue.Entries[4].Hash {
		t.Errorf("currentHash = %q, want row 4's %q", m.playback.currentHash, m.queue.Entries[4].Hash)
	}
	if msg := m.statusBar.TopNotification().Msg; strings.Contains(msg, "failed") {
		t.Errorf("notification = %q, want no failure surfaced for a superseded request", msg)
	}
}

// TestRapidSameTargetPlaybackResultsUseLatestRequest covers repeated Enter on
// one row: two requests naming the same queue occurrence, resolving differently,
// arriving in either order. Only the newest request's outcome may act. Index and
// Hash cannot tell these apart — both requests carry the same pair — so the
// request identity is the only thing standing between an abandoned failure and
// the state the newer request just established.
//
// Each subtest lets the first press reach the player before the second is
// dispatched, which is the only way an abandoned request can carry a real
// outcome now that the gate supersedes one still waiting to run. What the two
// presses race over here is the delivery of their results, not their execution;
// TestRapidDifferentTargetPlaybackKeepsLatestRequest covers the execution order.
func TestRapidSameTargetPlaybackResultsUseLatestRequest(t *testing.T) {
	t.Run("older failure cannot undo a newer success", func(t *testing.T) {
		m, p := replayModel([]int{3, 1, 4, 0, 2}, 2) // playing queue row 4

		// The first Enter on the playing row is refused by the player.
		m, first := dispatchSelect(t, m, replayDisplayRow)
		p.replayErr = errRefusedPlay
		firstResult := runPlayCmd(t, first)
		// The second Enter supersedes it and succeeds.
		p.replayErr = nil
		m, second := dispatchSelect(t, m, replayDisplayRow)
		secondResult := runPlayCmd(t, second)

		if p.replayCalls != 2 {
			t.Fatalf("ReplayCurrent calls = %d, want 2 — both presses reached the player", p.replayCalls)
		}
		if firstResult.Request == secondResult.Request {
			t.Fatalf("both requests share identity %d; they must be distinguishable", firstResult.Request)
		}
		if firstResult.Index != secondResult.Index || firstResult.Hash != secondResult.Hash {
			t.Fatal("test setup: the two requests must name the same queue occurrence")
		}

		m = m.handlePlayFromIndexResultMsg(secondResult)
		m = m.handlePlayFromIndexResultMsg(firstResult)

		assertStillPlayingRow4(t, m)
	})

	t.Run("older failure arriving first is dropped", func(t *testing.T) {
		m, p := replayModel([]int{3, 1, 4, 0, 2}, 2)

		m, first := dispatchSelect(t, m, replayDisplayRow)
		p.replayErr = errRefusedPlay
		firstResult := runPlayCmd(t, first)
		p.replayErr = nil
		m, second := dispatchSelect(t, m, replayDisplayRow)
		secondResult := runPlayCmd(t, second)

		// The abandoned failure lands before the newer success this time.
		m = m.handlePlayFromIndexResultMsg(firstResult)
		m = m.handlePlayFromIndexResultMsg(secondResult)

		assertStillPlayingRow4(t, m)
	})

	t.Run("newest failure still resets and notifies", func(t *testing.T) {
		m, p := replayModel([]int{3, 1, 4, 0, 2}, 2)

		m, first := dispatchSelect(t, m, replayDisplayRow)
		firstResult := runPlayCmd(t, first) // succeeds
		p.replayErr = errRefusedPlay
		m, second := dispatchSelect(t, m, replayDisplayRow)
		secondResult := runPlayCmd(t, second) // fails

		m = m.handlePlayFromIndexResultMsg(firstResult)
		m = m.handlePlayFromIndexResultMsg(secondResult)

		if m.playback.state != playback.StateIdle {
			t.Errorf("state = %v, want idle — the newest request failed", m.playback.state)
		}
		if m.playback.currentIndex != -1 {
			t.Errorf("currentIndex = %d, want -1", m.playback.currentIndex)
		}
		if msg := m.statusBar.TopNotification().Msg; !strings.HasPrefix(msg, "Replay failed: ") {
			t.Errorf("notification = %q, want the newest failure surfaced", msg)
		}
	})

	t.Run("selections of the same row are separated too", func(t *testing.T) {
		m, p := replayModel([]int{3, 1, 4, 0, 2}, 2) // playing queue row 4
		// The first selection of row 2 fails; the second one succeeds.
		p.playFromIndexErrs = []error{errRefusedPlay, nil}

		m, first := dispatchSelect(t, m, otherDisplayRow)
		firstResult := runPlayCmd(t, first)
		m, second := dispatchSelect(t, m, otherDisplayRow)
		secondResult := runPlayCmd(t, second)

		if !slices.Equal(p.playFromIndexCalls, []int{2, 2}) {
			t.Fatalf("PlayFromIndex calls = %v, want [2 2]", p.playFromIndexCalls)
		}
		if firstResult.Err == nil || secondResult.Err != nil {
			t.Fatalf("test setup: want the first request to fail and the second to succeed, got %v / %v",
				firstResult.Err, secondResult.Err)
		}

		m = m.handlePlayFromIndexResultMsg(secondResult)
		m = m.handlePlayFromIndexResultMsg(firstResult)

		if m.playback.state != playback.StatePlaying {
			t.Errorf("state = %v, want playing", m.playback.state)
		}
		if m.playback.currentIndex != 2 {
			t.Errorf("currentIndex = %d, want the selected row 2", m.playback.currentIndex)
		}
		if msg := m.statusBar.TopNotification().Msg; strings.Contains(msg, "failed") {
			t.Errorf("notification = %q, want the abandoned selection to stay silent", msg)
		}
	})
}

// playResult type-asserts the outcome of a player command that was executed on
// another goroutine. It runs on the test goroutine, which is the only one
// allowed to call t.Fatalf.
func playResult(t *testing.T, msg tea.Msg) playback.PlayFromIndexResultMsg {
	t.Helper()
	result, ok := msg.(playback.PlayFromIndexResultMsg)
	if !ok {
		t.Fatalf("expected PlayFromIndexResultMsg, got %T", msg)
	}
	return result
}

// TestRapidDifferentTargetPlaybackKeepsLatestRequest covers the race the request
// identity alone could not settle: two Enters on two *different* rows whose
// goroutines reach the player in the reverse of the order they were dispatched
// in.
//
// Nothing about a tea.Cmd preserves dispatch order — the runtime schedules the
// goroutines freely, and the player's own playlist mutex then serializes
// whichever one arrives first — so the older selection could replace mpv with
// the row the user had already moved away from, and go on to report that row as
// the current track through a TrackChangedMsg that carries no request identity
// at all. Neither guard on the result message helps: an older *successful*
// command produces nothing to reject, and by then the damage is in the player.
//
// The two requests are released here from separate goroutines in reverse
// execution order, and the newest one must be what the player is left holding
// and what the TUI shows.
func TestRapidDifferentTargetPlaybackKeepsLatestRequest(t *testing.T) {
	m, p := replayModel([]int{3, 1, 4, 0, 2}, 2) // playing queue row 4

	// Two different rows, neither of them the one playing, so both presses are
	// genuine selections rather than replays.
	const olderTarget, newerTarget = 0, 2
	m, older := dispatchSelect(t, m, m.queue.FindEntryFilteredPosition(olderTarget))
	m, newer := dispatchSelect(t, m, m.queue.FindEntryFilteredPosition(newerTarget))
	if !m.playback.pendingSelection {
		t.Fatal("a dispatched selection must be marked unconfirmed")
	}

	// Reverse execution order, made deterministic by releasing the older request
	// only once the newer one has been to the player and back.
	var (
		wg               sync.WaitGroup
		olderMsg, newMsg tea.Msg
	)
	newerDone := make(chan struct{})
	wg.Add(2)
	go func() {
		defer wg.Done()
		defer close(newerDone)
		newMsg = newer()
	}()
	go func() {
		defer wg.Done()
		<-newerDone
		olderMsg = older()
	}()
	wg.Wait()
	newerResult := playResult(t, newMsg)
	olderResult := playResult(t, olderMsg)

	if olderResult.Request >= newerResult.Request {
		t.Fatalf("request identities = %d then %d, want the second dispatch to be newer",
			olderResult.Request, newerResult.Request)
	}
	// The older request was superseded before it ran, so the player was never
	// told to go back to the abandoned row.
	if !slices.Equal(p.playFromIndexCalls, []int{newerTarget}) {
		t.Errorf("PlayFromIndex calls = %v, want only the newer request's row %d",
			p.playFromIndexCalls, newerTarget)
	}
	if slices.Contains(p.loaded, olderTarget) {
		t.Errorf("loaded = %v, want the abandoned row %d never loaded", p.loaded, olderTarget)
	}
	// The player's actual target: the cursor sits on the newer selection.
	tr := p.QueueTraversal()
	if got := tr.Order[tr.CurrentPosition]; got != newerTarget {
		t.Errorf("player is on queue row %d, want the newer request's row %d", got, newerTarget)
	}
	if olderResult.Err != nil {
		t.Errorf("superseded request reported %v, want no failure surfaced for a call never made",
			olderResult.Err)
	}

	// The other half of the finding: the abandoned row's own start-file event.
	// It carries no request identity, so only the unresolved newer selection can
	// tell the TUI not to believe it.
	stale := m.queue.Entries[olderTarget]
	m, _ = m.handleTrackChangedMsg(playback.TrackChangedMsg{
		Hash: stale.Hash, Index: olderTarget, Artist: stale.Artist, Title: stale.Title,
	})
	assertCurrentRow(t, m, newerTarget)

	// Results arrive in the same reverse order the commands ran in.
	m = m.handlePlayFromIndexResultMsg(newerResult)
	m = m.handlePlayFromIndexResultMsg(olderResult)
	assertCurrentRow(t, m, newerTarget)
	if msg := m.statusBar.TopNotification().Msg; strings.Contains(msg, "failed") {
		t.Errorf("notification = %q, want the superseded selection to stay silent", msg)
	}

	// The player confirms the row it actually started, which is the newer one.
	confirmed := m.queue.Entries[newerTarget]
	m, _ = m.handleTrackChangedMsg(playback.TrackChangedMsg{
		Hash: confirmed.Hash, Index: newerTarget, Artist: confirmed.Artist, Title: confirmed.Title,
	})
	assertCurrentRow(t, m, newerTarget)
	if m.playback.pendingSelection {
		t.Error("the player's start-file report must confirm the newer selection")
	}
}

// TestLateStaleTrackChangeCannotOverwriteNewestSelection covers the delivery
// order the request identity cannot reach at all: a start-file event.
//
// The player answers a load on the IPC socket, but it reports a started file
// from the event reader, and that reader syncs the position before it dispatches
// the report. So the two channels can invert: mpv confirms the newest load while
// the previous row's start-file is still sitting in the reader. That report
// carries no request identity, and by the time it lands the newest request has
// already been answered — so if a successful answer is taken as proof that
// nothing older is still in flight, the stale report walks straight through and
// puts the TUI on a row the player left.
//
// Only the selected row's own start-file proves the reader is caught up, which
// is why success alone must not retire the barrier.
func TestLateStaleTrackChangeCannotOverwriteNewestSelection(t *testing.T) {
	// Two different rows, neither of them the one playing, so both presses are
	// genuine selections rather than replays.
	const olderTarget, newerTarget = 0, 1

	t.Run("a delayed report from the abandoned row is refused", func(t *testing.T) {
		m, p := replayModel([]int{3, 1, 4, 0, 2}, 2) // playing queue row 4

		// The older selection reaches the player and is accepted: mpv loads the
		// abandoned row and emits its start-file, which the reader is still
		// holding while it syncs the position.
		m, older := dispatchSelect(t, m, m.queue.FindEntryFilteredPosition(olderTarget))
		olderResult := runPlayCmd(t, older)
		// The user moves on before that report is delivered.
		m, newer := dispatchSelect(t, m, m.queue.FindEntryFilteredPosition(newerTarget))
		newerResult := runPlayCmd(t, newer)

		if olderResult.Err != nil || newerResult.Err != nil {
			t.Fatalf("test setup: both loads must be accepted, got %v / %v",
				olderResult.Err, newerResult.Err)
		}
		if !slices.Equal(p.playFromIndexCalls, []int{olderTarget, newerTarget}) {
			t.Fatalf("PlayFromIndex calls = %v, want both requests to reach the player in order",
				p.playFromIndexCalls)
		}

		// The newest request is answered, which confirms the row for replay
		// purposes but says nothing about what the event reader still holds.
		m = m.handlePlayFromIndexResultMsg(newerResult)
		m = m.handlePlayFromIndexResultMsg(olderResult) // superseded, discarded

		// The abandoned row's start-file finally arrives.
		m, _ = m.handleTrackChangedMsg(trackChangeFor(m, olderTarget))
		assertCurrentRow(t, m, newerTarget)
		if tr := p.QueueTraversal(); tr.Order[tr.CurrentPosition] != newerTarget {
			t.Errorf("player is on queue row %d, want the newest request's row %d",
				tr.Order[tr.CurrentPosition], newerTarget)
		}

		// The newest selection's own report still lands and retires the barrier.
		m, _ = m.handleTrackChangedMsg(trackChangeFor(m, newerTarget))
		assertCurrentRow(t, m, newerTarget)
		if m.playback.awaitingSelectionStart {
			t.Error("the selected row's own start-file must retire the barrier")
		}
	})

	t.Run("gapless advance applies once the selection has started", func(t *testing.T) {
		m, p := replayModel([]int{3, 1, 4, 0, 2}, 2)

		m, cmd := dispatchSelect(t, m, m.queue.FindEntryFilteredPosition(newerTarget))
		m = m.handlePlayFromIndexResultMsg(runPlayCmd(t, cmd))
		m, _ = m.handleTrackChangedMsg(trackChangeFor(m, newerTarget))

		// The traversal plays that row out and mpv starts the successor it had
		// already prefetched. Nothing is superseded here, so the barrier must not
		// be standing in the way of an ordinary auto-advance.
		next, ok := p.finishCurrent()
		if !ok {
			t.Fatal("test setup: the traversal must have a gapless successor")
		}
		m, _ = m.handleTrackChangedMsg(trackChangeFor(m, next))
		assertCurrentRow(t, m, next)
	})
}
