package tui

import (
	"runtime"
	"slices"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/tui/keys"
	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/queue"
)

func waitForPlayRequestWait(t *testing.T, gate *playRequestGate, req uint64) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		gate.mu.Lock()
		state := gate.pending[req]
		waiting := state != nil && state.waiting
		gate.mu.Unlock()
		if waiting {
			return
		}
		runtime.Gosched()
	}
	t.Fatalf("request %d never waited for the pending selection", req)
}

func playbackControlResult(t *testing.T, msg tea.Msg) playbackControlResultMsg {
	t.Helper()
	result, ok := msg.(playbackControlResultMsg)
	if !ok {
		t.Fatalf("expected playbackControlResultMsg, got %T", msg)
	}
	return result
}

// This file covers the other half of the play-request ordering: what happens to
// a selection that is still queued when playback is taken away from under it.
//
// The companion files cover selections racing each other. Here the race is
// between a selection and everything else that decides what the player holds —
// an explicit stop, a queue clear, the autoplay that starts the replacement
// queue, the resume that continues an extended one. Each of those runs on the
// Update goroutine while a dispatched command may still be waiting for its
// goroutine to be scheduled. Without ordering, that command runs afterwards and
// undoes the decision: it restarts audio the user stopped, or aims its stale
// numeric index at rows that replaced the ones it was picked from.

// stoppableReplayModel is replayModel over a player that also accepts commands,
// so the teardown paths have a Stop to call and a traversal to disturb.
func stoppableReplayModel(order []int, pos int) (Model, *fakeStoppablePlayer) {
	p := newFakeStoppablePlayer(order, pos)
	p.resolvePrefetch()
	m := newModelWithTracks()
	m.player = p
	m.queue.Traversal = queueTraversalSource(p)
	m.queue.Entries = replayEntries(len(order))
	m.playback.state = playback.StatePlaying
	m.playback.currentIndex = order[pos]
	m.playback.currentHash = m.queue.Entries[order[pos]].Hash
	return m, p
}

// parkSelection presses Enter on a row that is not playing and holds the
// resulting player command unexecuted, which is the state a dispatched request
// is in between the Update that created it and the goroutine that will run it.
func parkSelection(t *testing.T, m Model, displayRow int) (Model, tea.Cmd) {
	t.Helper()
	m, cmd := dispatchSelect(t, m, displayRow)
	if !m.playback.pendingSelection {
		t.Fatal("test setup: the parked request must be an unconfirmed selection")
	}
	return m, cmd
}

// assertParkedSelectionNeverPlayed checks the released command left the player
// alone: it neither loaded its row nor resumed anything.
func assertParkedSelectionNeverPlayed(t *testing.T, p *fakeStoppablePlayer, result playback.PlayFromIndexResultMsg) {
	t.Helper()
	if len(p.playFromIndexCalls) != 0 {
		t.Errorf("PlayFromIndex calls = %v; a superseded selection must not reach the player",
			p.playFromIndexCalls)
	}
	if p.replayCalls != 0 {
		t.Errorf("ReplayCurrent calls = %d, want 0", p.replayCalls)
	}
	if result.Err != nil {
		t.Errorf("result error = %v; a superseded request did not fail, it never ran", result.Err)
	}
}

// TestStopSupersedesInFlightPlayRequest is FR-010's first half: an explicit stop
// must outrank a selection that has been dispatched but has not run. The stop
// used to lower the selection barrier and leave the command alone, so the
// command could reach mpv afterwards and start audio the user had just ended.
func TestStopSupersedesInFlightPlayRequest(t *testing.T) {
	t.Run("the parked selection never reaches the player", func(t *testing.T) {
		m, p := stoppableReplayModel([]int{3, 1, 4, 0, 2}, 2)
		m, parked := parkSelection(t, m, otherDisplayRow)
		loadedBefore := len(p.loaded)

		next, _, handled := m.handlePlaybackKeyAction(keys.ActionStop)
		if !handled {
			t.Fatal("stop was not handled")
		}
		m = next.(Model)

		// The goroutine finally runs, after the stop.
		result := runPlayCmd(t, parked)

		assertParkedSelectionNeverPlayed(t, p, result)
		if m.playback.state != playback.StateStopped {
			t.Errorf("state = %v, want stopped", m.playback.state)
		}
		if p.stopCalls != 1 {
			t.Errorf("Stop calls = %d, want 1", p.stopCalls)
		}
		if !p.stopped {
			t.Error("the player is playing again; the parked selection restarted it after the stop")
		}
		if len(p.loaded) != loadedBefore {
			t.Errorf("loaded = %v; nothing may load after a stop", p.loaded)
		}
		if !slices.Equal(p.calls, []string{"Stop"}) {
			t.Errorf("player calls = %v, want [Stop] — the stop must be the last word", p.calls)
		}
	})

	t.Run("the parked selection's result cannot revive playback", func(t *testing.T) {
		m, p := stoppableReplayModel([]int{3, 1, 4, 0, 2}, 2)
		m, parked := parkSelection(t, m, otherDisplayRow)

		next, _, _ := m.handlePlaybackKeyAction(keys.ActionStop)
		m = next.(Model)
		result := runPlayCmd(t, parked)

		if result.Request == m.playback.playRequestSeq {
			t.Fatalf("the parked result still carries the current identity %d; "+
				"the stop must have taken a newer one", result.Request)
		}

		m = m.handlePlayFromIndexResultMsg(result)

		if m.playback.state != playback.StateStopped {
			t.Errorf("state = %v, want stopped — a superseded result must not restate playback",
				m.playback.state)
		}
		if p.stopped != true {
			t.Error("the result handler restarted the player")
		}
	})

	t.Run("the abandoned row is still selectable afterwards", func(t *testing.T) {
		// The stop leaves the selection unconfirmed on purpose: currentIndex
		// names a row the player never started, so Enter on it must anchor that
		// row explicitly rather than replay whatever the player is holding.
		m, p := stoppableReplayModel([]int{3, 1, 4, 0, 2}, 2)
		m, parked := parkSelection(t, m, otherDisplayRow)
		next, _, _ := m.handlePlaybackKeyAction(keys.ActionStop)
		m = next.(Model)
		runPlayCmd(t, parked)

		m, result := runSelect(t, m, otherDisplayRow)

		if result.Replay {
			t.Error("the retry was classified as a replay; the row was never confirmed")
		}
		if !slices.Equal(p.playFromIndexCalls, []int{2}) {
			t.Errorf("PlayFromIndex calls = %v, want [2] — the retry names its own row",
				p.playFromIndexCalls)
		}
		if m.playback.currentIndex != 2 {
			t.Errorf("currentIndex = %d, want 2", m.playback.currentIndex)
		}
	})
}

// TestPlaybackControlsOrderAfterPendingSelection covers FR-013. A transport
// control issued after an optimistic queue selection must act on the selected
// physical row, even when its tea.Cmd goroutine runs before the selection's.
func TestPlaybackControlsOrderAfterPendingSelection(t *testing.T) {
	tests := []struct {
		name        string
		action      keys.Action
		controlCall string
		wantIndex   int
		wantPaused  bool
	}{
		{name: "pause", action: keys.ActionPlayPause, controlCall: "Pause", wantIndex: 2, wantPaused: true},
		{name: "next", action: keys.ActionNext, controlCall: "Next", wantIndex: 3},
		{name: "previous", action: keys.ActionPrevious, controlCall: "Previous", wantIndex: 1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m, p := stoppableReplayModel([]int{0, 1, 2, 3, 4}, 0)
			m, selection := m.handlePlayQueueIndexMsg(playback.PlayQueueIndexMsg{Index: 2})
			if selection == nil {
				t.Fatal("selection returned no player command")
			}

			next, control, handled := m.handlePlaybackKeyAction(tc.action)
			if !handled || control == nil {
				t.Fatal("transport action was not queued")
			}
			m = next.(Model)
			controlReq := m.playback.requestSeq

			controlDone := make(chan tea.Msg, 1)
			go func() { controlDone <- control() }()
			waitForPlayRequestWait(t, m.playback.requests, controlReq)
			if len(p.calls) != 0 {
				t.Fatalf("player calls before selection release = %v", p.calls)
			}

			selectionResult := runPlayCmd(t, selection)
			controlResult := playbackControlResult(t, <-controlDone)
			if selectionResult.Err != nil || controlResult.err != nil {
				t.Fatalf("ordered calls failed: selection=%v control=%v",
					selectionResult.Err, controlResult.err)
			}
			if !slices.Equal(p.calls, []string{"PlayFromIndex", tc.controlCall}) {
				t.Fatalf("player calls = %v, want [PlayFromIndex %s]", p.calls, tc.controlCall)
			}

			// The player emits the selected row's start-file before the later
			// control's state change. Feed those reports in the same order.
			m, _ = m.handleTrackChangedMsg(trackChangeFor(m, 2))
			if tc.action == keys.ActionPlayPause {
				m, _ = m.handlePauseChangedMsg(playback.PauseChangedMsg{Paused: true})
			} else {
				m, _ = m.handleTrackChangedMsg(trackChangeFor(m, tc.wantIndex))
			}
			m, _ = m.handlePlaybackControlResultMsg(controlResult)

			traversal := p.QueueTraversal()
			if got := traversal.Order[traversal.CurrentPosition]; got != tc.wantIndex {
				t.Errorf("player index = %d, want %d", got, tc.wantIndex)
			}
			if m.playback.currentIndex != tc.wantIndex {
				t.Errorf("model index = %d, want %d", m.playback.currentIndex, tc.wantIndex)
			}
			if p.paused != tc.wantPaused {
				t.Errorf("player paused = %t, want %t", p.paused, tc.wantPaused)
			}
			wantState := playback.StatePlaying
			if tc.wantPaused {
				wantState = playback.StatePaused
			}
			if m.playback.state != wantState {
				t.Errorf("model state = %v, want %v", m.playback.state, wantState)
			}
		})
	}

	t.Run("play after pause", func(t *testing.T) {
		m, p := stoppableReplayModel([]int{0, 1, 2, 3, 4}, 0)
		m, selection := m.handlePlayQueueIndexMsg(playback.PlayQueueIndexMsg{Index: 2})

		next, pauseCmd, _ := m.handlePlaybackKeyAction(keys.ActionPlayPause)
		m = next.(Model)
		pauseReq := m.playback.requestSeq
		next, playCmd, _ := m.handlePlaybackKeyAction(keys.ActionPlayPause)
		m = next.(Model)
		playReq := m.playback.requestSeq

		pauseDone := make(chan tea.Msg, 1)
		playDone := make(chan tea.Msg, 1)
		go func() { pauseDone <- pauseCmd() }()
		go func() { playDone <- playCmd() }()
		waitForPlayRequestWait(t, m.playback.requests, pauseReq)
		waitForPlayRequestWait(t, m.playback.requests, playReq)

		selectionResult := runPlayCmd(t, selection)
		pauseResult := playbackControlResult(t, <-pauseDone)
		playResult := playbackControlResult(t, <-playDone)
		if selectionResult.Err != nil || pauseResult.err != nil || playResult.err != nil {
			t.Fatalf("ordered calls failed: selection=%v pause=%v play=%v",
				selectionResult.Err, pauseResult.err, playResult.err)
		}
		if !slices.Equal(p.calls, []string{"PlayFromIndex", "Pause", "Play"}) {
			t.Fatalf("player calls = %v, want [PlayFromIndex Pause Play]", p.calls)
		}

		m = m.handlePlayFromIndexResultMsg(selectionResult)
		m, _ = m.handleTrackChangedMsg(trackChangeFor(m, 2))
		m, _ = m.handlePauseChangedMsg(playback.PauseChangedMsg{Paused: false})
		m, _ = m.handlePlaybackControlResultMsg(pauseResult)
		m, _ = m.handlePlaybackControlResultMsg(playResult)

		traversal := p.QueueTraversal()
		if got := traversal.Order[traversal.CurrentPosition]; got != 2 {
			t.Errorf("player index = %d, want selected row 2", got)
		}
		if p.paused {
			t.Error("player stayed paused after the later Play control")
		}
		if m.playback.currentIndex != 2 || m.playback.state != playback.StatePlaying {
			t.Errorf("model = index %d state %v, want selected row 2 playing",
				m.playback.currentIndex, m.playback.state)
		}
	})
}

// TestQueueResetAndAutoplaySupersedeInFlightPlayRequest is FR-010's second half.
// A clear tears the queue down while a selection is still queued against it, and
// what starts next — the autoplay onto the repopulated queue, or the resume into
// an extended one — is a synchronous player call made on the Update goroutine,
// outside the gate entirely. The parked selection has to lose to both, and its
// stale index must never be applied to rows it was not chosen from.
func TestQueueResetAndAutoplaySupersedeInFlightPlayRequest(t *testing.T) {
	t.Run("clear plus autoplay cannot be retargeted", func(t *testing.T) {
		m, p := stoppableReplayModel([]int{3, 1, 4, 0, 2}, 2)
		m, parked := parkSelection(t, m, otherDisplayRow)

		m = m.handleClearQueueMsg()
		if len(m.queue.Entries) != 0 {
			t.Fatalf("queue length = %d, want 0 after a clear", len(m.queue.Entries))
		}

		// Repopulate the way an enqueue onto an empty queue does: one new row,
		// with the player's traversal covering it and nothing current.
		m.queue.Entries = []queue.Entry{{
			ID: 99, Hash: "fresh", Path: "/fresh.flac", Artist: "Fresh", Title: "Fresh Row",
		}}
		p.order = []int{0}
		p.pos = -1
		p.resolvePrefetch()
		m = m.tryAutoPlay("fresh", "Fresh", "Fresh Row", "")

		// Only now does the selection dispatched against the old queue run.
		result := runPlayCmd(t, parked)

		if !slices.Equal(p.playFromIndexCalls, []int{0}) {
			t.Errorf("PlayFromIndex calls = %v, want [0] — only the autoplay may load",
				p.playFromIndexCalls)
		}
		if !slices.Equal(p.calls, []string{"Stop", "PlayFromIndex"}) {
			t.Errorf("player calls = %v, want [Stop PlayFromIndex]", p.calls)
		}
		if result.Err != nil {
			t.Errorf("result error = %v; the superseded request never ran", result.Err)
		}

		m = m.handlePlayFromIndexResultMsg(result)

		if m.playback.state != playback.StatePlaying {
			t.Errorf("state = %v, want playing", m.playback.state)
		}
		if m.playback.currentHash != "fresh" || m.playback.currentIndex != 0 {
			t.Errorf("current = %d/%q, want 0/\"fresh\" — the stale request retargeted playback",
				m.playback.currentIndex, m.playback.currentHash)
		}
	})

	t.Run("an extension resume outranks a parked selection", func(t *testing.T) {
		m, p := stoppableReplayModel([]int{3, 1, 4, 0, 2}, 2)
		m, parked := parkSelection(t, m, otherDisplayRow)

		m, resumed := m.resumeExtendedQueue(0)
		if !resumed {
			t.Fatal("the resume did not start playback")
		}

		result := runPlayCmd(t, parked)

		if p.resumeCalls != 1 {
			t.Errorf("ResumeAfterQueueExtension calls = %d, want 1", p.resumeCalls)
		}
		assertParkedSelectionNeverPlayed(t, p, result)
		// The traversal steps from position 2 onto position 3, which is queue row 0.
		if m.playback.currentIndex != 0 {
			t.Fatalf("currentIndex = %d, want 0 — the resume decides the row", m.playback.currentIndex)
		}

		m = m.handlePlayFromIndexResultMsg(result)

		if m.playback.currentIndex != 0 || m.playback.currentHash != m.queue.Entries[0].Hash {
			t.Errorf("current = %d/%q, want the resumed row 0/%q",
				m.playback.currentIndex, m.playback.currentHash, m.queue.Entries[0].Hash)
		}
		if m.playback.state != playback.StatePlaying {
			t.Errorf("state = %v, want playing", m.playback.state)
		}
	})

	t.Run("the fallback resume supersedes too", func(t *testing.T) {
		// A player without ResumeAfterQueueExtension takes the PlayFromIndex
		// fallback. It is the branch that made resumeExtendedQueueIndex a pointer
		// receiver: the identity it takes has to reach the caller's model, or the
		// parked request's result still matches and stays authoritative.
		p := &stubQueueLifecyclePlayer{}
		m := newModelWithTracks()
		m.player = p
		m.queue.Entries = replayEntries(5)
		m.playback.state = playback.StatePlaying
		m.playback.currentIndex = 4
		m.playback.currentHash = m.queue.Entries[4].Hash
		m, parked := parkSelection(t, m, 2)

		seqBefore := m.playback.playRequestSeq
		m, resumed := m.resumeExtendedQueue(1)
		if !resumed {
			t.Fatal("the resume did not start playback")
		}
		if m.playback.playRequestSeq == seqBefore {
			t.Fatal("the resume left the request identity behind, so the parked result still looks current")
		}

		result := runPlayCmd(t, parked)

		if !slices.Equal(p.playFromIndexCalls, []int{1}) {
			t.Errorf("PlayFromIndex calls = %v, want [1] — only the resume may load",
				p.playFromIndexCalls)
		}
		if result.Request >= m.playback.playRequestSeq {
			t.Errorf("parked request %d is not older than the current %d",
				result.Request, m.playback.playRequestSeq)
		}
	})
}
