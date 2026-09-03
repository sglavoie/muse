package tui

import (
	"errors"
	"slices"
	"strings"
	"testing"

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
func TestRapidSameTargetPlaybackResultsUseLatestRequest(t *testing.T) {
	t.Run("older failure cannot undo a newer success", func(t *testing.T) {
		m, p := replayModel([]int{3, 1, 4, 0, 2}, 2) // playing queue row 4

		// Two Enters on the playing row before either reaches the player.
		m, first := dispatchSelect(t, m, replayDisplayRow)
		m, second := dispatchSelect(t, m, replayDisplayRow)

		p.replayErr = errRefusedPlay
		firstResult := runPlayCmd(t, first)
		p.replayErr = nil
		secondResult := runPlayCmd(t, second)

		if p.replayCalls != 2 {
			t.Fatalf("ReplayCurrent calls = %d, want 2 — both presses are replays", p.replayCalls)
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
		m, second := dispatchSelect(t, m, replayDisplayRow)

		p.replayErr = errRefusedPlay
		firstResult := runPlayCmd(t, first)
		p.replayErr = nil
		secondResult := runPlayCmd(t, second)

		// The abandoned failure lands before the newer success this time.
		m = m.handlePlayFromIndexResultMsg(firstResult)
		m = m.handlePlayFromIndexResultMsg(secondResult)

		assertStillPlayingRow4(t, m)
	})

	t.Run("newest failure still resets and notifies", func(t *testing.T) {
		m, p := replayModel([]int{3, 1, 4, 0, 2}, 2)

		m, first := dispatchSelect(t, m, replayDisplayRow)
		m, second := dispatchSelect(t, m, replayDisplayRow)

		firstResult := runPlayCmd(t, first) // succeeds
		p.replayErr = errRefusedPlay
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
		m, second := dispatchSelect(t, m, otherDisplayRow)
		firstResult := runPlayCmd(t, first)
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

	// The second request wins the race to the player.
	secondResult := runPlayCmd(t, second)
	firstResult := runPlayCmd(t, first)

	if p.replayCalls != 0 {
		t.Errorf("ReplayCurrent calls = %d, want 0 — the selected row was never confirmed", p.replayCalls)
	}
	if secondResult.Replay || firstResult.Replay {
		t.Error("neither request may report itself as a replay")
	}
	if !slices.Equal(p.playFromIndexCalls, []int{2, 2}) {
		t.Errorf("PlayFromIndex calls = %v, want both presses to name row 2", p.playFromIndexCalls)
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
	if !slices.Equal(p.playFromIndexCalls, []int{2, 2}) {
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
