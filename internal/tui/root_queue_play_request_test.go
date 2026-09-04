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

// assertCurrentRow checks the TUI is showing queue row idx as the current track.
func assertCurrentRow(t *testing.T, m Model, idx int) {
	t.Helper()
	if m.playback.currentIndex != idx {
		t.Errorf("currentIndex = %d, want %d", m.playback.currentIndex, idx)
	}
	if want := m.queue.Entries[idx].Hash; m.playback.currentHash != want {
		t.Errorf("currentHash = %q, want row %d's %q", m.playback.currentHash, idx, want)
	}
	if m.playback.state != playback.StatePlaying {
		t.Errorf("state = %v, want playing", m.playback.state)
	}
}

// TestPendingSelectionIsNotMisclassifiedAsReplay covers the other half of the
// race: Enter on a row the player has not been told about yet. The optimistic
// update writes the selected row into currentIndex/currentHash immediately, so a
// second Enter arriving before the player answers would match it and go to
// ReplayCurrent — which restarts the entry the player is still holding, i.e. the
// previous track, not the one the user pointed at twice.
func TestPendingSelectionIsNotMisclassifiedAsReplay(t *testing.T) {
	m, p := replayModel([]int{3, 1, 4, 0, 2}, 2) // playing queue row 4

	// Two Enters on row 2. Neither has reached the player: the first request's
	// goroutine has not run, so the player is still sitting on row 4.
	m, first := dispatchSelect(t, m, otherDisplayRow)
	if !m.playback.pendingSelection {
		t.Fatal("a dispatched selection must be marked unconfirmed")
	}
	m, second := dispatchSelect(t, m, otherDisplayRow)

	// The second request reaches the player first, which supersedes the first
	// one: it is answered without ever being sent.
	secondResult := runPlayCmd(t, second)
	firstResult := runPlayCmd(t, first)

	if p.replayCalls != 0 {
		t.Errorf("ReplayCurrent calls = %d, want 0 — the selected row was never confirmed", p.replayCalls)
	}
	if secondResult.Replay || firstResult.Replay {
		t.Error("neither request may report itself as a replay")
	}
	if !slices.Equal(p.playFromIndexCalls, []int{2}) {
		t.Errorf("PlayFromIndex calls = %v, want only the newest press to reach the player",
			p.playFromIndexCalls)
	}
	// The point of the whole guard: row 4 must never be restarted. A misclassified
	// replay would have loaded it, because that is where the player still was.
	if slices.Contains(p.loaded, 4) {
		t.Errorf("loaded = %v, want the previously playing row 4 left alone", p.loaded)
	}

	// Once the newest request is answered the selection is confirmed, and Enter on
	// the row is a replay again — the guard covers the unconfirmed window only.
	m = m.handlePlayFromIndexResultMsg(secondResult)
	if m.playback.pendingSelection {
		t.Fatal("the latest request's result must confirm the selection")
	}
	m, third := dispatchSelect(t, m, m.queue.FindEntryFilteredPosition(2))
	thirdResult := runPlayCmd(t, third)

	if !thirdResult.Replay || p.replayCalls != 1 {
		t.Errorf("replay = %v after %d ReplayCurrent calls, want a replay of the confirmed row",
			thirdResult.Replay, p.replayCalls)
	}
	if !slices.Equal(p.playFromIndexCalls, []int{2}) {
		t.Errorf("PlayFromIndex calls = %v, want the confirmed row replayed instead of reanchored",
			p.playFromIndexCalls)
	}

	// The abandoned first request finally reports back. It names the same row as
	// everything since, so only its identity marks it as spent.
	m = m.handlePlayFromIndexResultMsg(firstResult)
	if m.playback.currentIndex != 2 || m.playback.state != playback.StatePlaying {
		t.Errorf("after the abandoned result: currentIndex = %d state = %v; want 2 playing",
			m.playback.currentIndex, m.playback.state)
	}
}

// TestTrackChangedConfirmsPendingSelection verifies the player's own report of
// the row it started also closes the unconfirmed window. It is the confirmation
// that arrives in normal playback — the start-file event — and without it a
// selection whose result message is still in flight would leave every later
// Enter on that row reanchoring the shuffle cycle instead of replaying.
func TestTrackChangedConfirmsPendingSelection(t *testing.T) {
	m, p := replayModel([]int{3, 1, 4, 0, 2}, 2)

	m, cmd := dispatchSelect(t, m, otherDisplayRow)
	if !m.playback.pendingSelection {
		t.Fatal("a dispatched selection must be marked unconfirmed")
	}
	runPlayCmd(t, cmd)

	entry := m.queue.Entries[2]
	m, _ = m.handleTrackChangedMsg(playback.TrackChangedMsg{
		Hash: entry.Hash, Index: 2, Artist: entry.Artist, Title: entry.Title,
	})
	if m.playback.pendingSelection {
		t.Fatal("the player's start-file report must confirm the selection")
	}

	_, result := runSelect(t, m, m.queue.FindEntryFilteredPosition(2))
	if !result.Replay || p.replayCalls != 1 {
		t.Errorf("replay = %v after %d ReplayCurrent calls, want a replay once the row is confirmed",
			result.Replay, p.replayCalls)
	}
}
