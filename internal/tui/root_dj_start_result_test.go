package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/tui/keys"
	"github.com/sglavoie/muse/internal/tui/playback"
)

// Count requested transport calls separately from automatic Play/index starts.
type djStartResultPlayer struct {
	*stubStopAfterPlayer
	transportCalls int
	transportErr   error
}

func (p *djStartResultPlayer) Next() error {
	p.transportCalls++
	return p.transportErr
}

func (p *djStartResultPlayer) Previous() error {
	p.transportCalls++
	return p.transportErr
}

func djStartResultDispatch(t *testing.T, m Model, operation string) (Model, tea.Cmd) {
	t.Helper()
	if operation == "selection" {
		next, cmd := m.Update(playback.PlayQueueIndexMsg{Index: 1})
		return next.(Model), cmd
	}
	action := keys.ActionNext
	if operation == "previous" {
		action = keys.ActionPrevious
	}
	next, cmd, handled := m.handlePlaybackKeyAction(action)
	if !handled || cmd == nil {
		t.Fatal("transport start was not dispatched")
	}
	return next.(Model), cmd
}

// Reach the low-water mark, hold a real prepared batch across an actual Stop,
// then leave that late result for the caller to deliver at the race boundary.
func djStartResultStopped(t *testing.T) (Model, *djStartResultPlayer, *stubDJQueueWriter, []tea.Msg) {
	t.Helper()
	writer := &stubDJQueueWriter{}
	m, base := djStopBoundaryModel(writer)
	p := &djStartResultPlayer{stubStopAfterPlayer: base}
	m.player = p
	next, cmd := m.Update(djStopBoundaryTrackChange())
	m = next.(Model)
	late := djWakeMessages(cmd)
	if len(writer.prepared) != 1 || len(writer.accepted) != 0 {
		t.Fatal("setup did not hold one unpublished preparation")
	}
	m = djStartResultStop(t, m)
	return m, p, writer, late
}

func djStartResultStop(t *testing.T, m Model) Model {
	t.Helper()
	next, cmd, handled := m.handlePlaybackKeyAction(keys.ActionStop)
	if !handled {
		t.Fatal("Stop was not handled")
	}
	m = next.(Model)
	for _, msg := range djWakeMessages(cmd) {
		m = djWakeDrive(m, msg, 4)
	}
	m = djWakeDrive(m, playback.PlaybackEndedMsg{}, 4)
	if !m.radio.stoppedByUser || m.playback.state != playback.StateIdle {
		t.Fatal("setup did not complete an actual Stop")
	}
	return m
}

func djStartResultDrain(m Model, messages []tea.Msg) Model {
	for _, msg := range messages {
		m = djWakeDrive(m, msg, 4)
	}
	return djWakeTicks(m, djMaxTransientRetries+3)
}

func djStartResultQuiet(t *testing.T, m Model, p *djStartResultPlayer, w *stubDJQueueWriter,
	indexCalls, transportCalls int,
) {
	t.Helper()
	if len(w.prepared) != 1 || len(w.accepted) != 0 || len(m.queue.Entries) != 2 {
		t.Errorf("post-stop DJ work: preparations=%d accepts=%d queue=%d; want 1/0/2",
			len(w.prepared), len(w.accepted), len(m.queue.Entries))
	}
	if !m.radio.stoppedByUser {
		t.Error("start without a successful result lifted the suspension")
	}
	if p.playCalls != 0 || len(p.playFromIndexCalls) != indexCalls || p.transportCalls != transportCalls {
		t.Errorf("unexpected playback calls: Play=%d index=%d transport=%d; want 0/%d/%d",
			p.playCalls, len(p.playFromIndexCalls), p.transportCalls, indexCalls, transportCalls)
	}
}

func TestAutoDJStopWaitsForSuccessfulStartResult(t *testing.T) {
	t.Run("stale outcomes cannot settle newer intent", djStartResultStaleOutcomes)
	for _, operation := range []string{"next", "previous", "selection"} {
		t.Run(operation, func(t *testing.T) {
			indexCalls, transportCalls := 0, 1
			if operation == "selection" {
				indexCalls, transportCalls = 1, 0
			}
			for _, held := range []string{"command", "result"} {
				t.Run("report before refused "+held, func(t *testing.T) {
					m, p, w, late := djStartResultStopped(t)
					p.transportErr, p.playFromIndexErr = errStubPlay, errStubPlay
					m, cmd := djStartResultDispatch(t, m, operation)
					var results []tea.Msg
					if held == "result" {
						results = djWakeMessages(cmd)
					}
					// Same loaded occurrence: it passes the selection identity barrier.
					m = djWakeDrive(m, djStopBoundaryTrackChange(), 4)
					m = djStartResultDrain(m, late)
					if held == "command" {
						djStartResultQuiet(t, m, p, w, 0, 0)
						results = djWakeMessages(cmd)
					}
					m = djStartResultDrain(m, results)
					m = djWakeDrive(m, djStopBoundaryTrackChange(), 4)
					m = djWakeTicks(m, djMaxTransientRetries+3)
					djStartResultQuiet(t, m, p, w, indexCalls, transportCalls)
					if len(w.rejected) != 1 {
						t.Errorf("late preparation cleanup=%d, want 1", len(w.rejected))
					}
				})
			}
			for _, eventFirst := range []bool{true, false} {
				name := "success before report"
				if eventFirst {
					name = "report before success"
				}
				t.Run(name, func(t *testing.T) {
					m, p, w, late := djStartResultStopped(t)
					m = djStartResultDrain(m, late)
					m, cmd := djStartResultDispatch(t, m, operation)
					results := djWakeMessages(cmd)
					if eventFirst {
						m = djWakeDrive(m, djStopBoundaryTrackChange(), 4)
					} else {
						m = djStartResultDrain(m, results)
					}
					djStartResultQuiet(t, m, p, w, indexCalls, transportCalls)
					if eventFirst {
						m = djStartResultDrain(m, results)
					} else {
						m = djWakeDrive(m, djStopBoundaryTrackChange(), 4)
					}
					if m.radio.stoppedByUser {
						t.Error("successful confirmed start left DJ suspended")
					}
					djStopIntentAssertResumed(t, w, m, 2)
				})
			}
			for _, held := range []string{"command", "result", "reported result"} {
				t.Run("newer Stop supersedes "+held, func(t *testing.T) {
					m, p, w, late := djStartResultStopped(t)
					m, cmd := djStartResultDispatch(t, m, operation)
					var results []tea.Msg
					if held != "command" {
						results = djWakeMessages(cmd)
					}
					if held == "reported result" {
						m = djWakeDrive(m, djStopBoundaryTrackChange(), 4)
					}
					m = djStartResultStop(t, m)
					if held == "command" {
						results = djWakeMessages(cmd)
					}
					m = djStartResultDrain(m, results)
					m = djWakeDrive(m, djStopBoundaryTrackChange(), 4)
					m = djStartResultDrain(m, late)
					if held == "command" {
						djStartResultQuiet(t, m, p, w, 0, 0)
					} else {
						djStartResultQuiet(t, m, p, w, indexCalls, transportCalls)
					}
				})
			}
		})
	}
}

func djStartResultStaleOutcomes(t *testing.T) {
	for _, pair := range [][2]string{
		{"selection", "next"}, {"next", "selection"},
		{"selection", "selection"}, {"next", "previous"},
	} {
		for _, oldFails := range []bool{true, false} {
			name := pair[0] + " then " + pair[1] + "/old success new refusal"
			if oldFails {
				name = pair[0] + " then " + pair[1] + "/old refusal new success"
			}
			t.Run(name, func(t *testing.T) {
				m, p, w, late := djStartResultStopped(t)
				m = djStartResultDrain(m, late)
				if oldFails {
					p.transportErr, p.playFromIndexErr = errStubPlay, errStubPlay
				}
				m, oldCmd := djStartResultDispatch(t, m, pair[0])
				oldResults := djWakeMessages(oldCmd)
				// This old report must not be inherited by the next request.
				m = djWakeDrive(m, djStopBoundaryTrackChange(), 4)
				p.transportErr, p.playFromIndexErr = errStubPlay, errStubPlay
				if oldFails {
					p.transportErr, p.playFromIndexErr = nil, nil
				}
				m, newCmd := djStartResultDispatch(t, m, pair[1])
				newResults := djWakeMessages(newCmd)
				indexCalls := 0
				for _, operation := range pair {
					if operation == "selection" {
						indexCalls++
					}
				}
				// An old outcome cannot confirm or cancel the new request, even
				// when playRequestSeq still names the older queue selection.
				m = djStartResultDrain(m, oldResults)
				djStartResultQuiet(t, m, p, w, indexCalls, 2-indexCalls)
				m = djStartResultDrain(m, newResults)
				djStartResultQuiet(t, m, p, w, indexCalls, 2-indexCalls)
				m = djWakeDrive(m, djStopBoundaryTrackChange(), 4)
				m = djWakeTicks(m, djMaxTransientRetries+3)
				if oldFails {
					djStopIntentAssertResumed(t, w, m, 2)
				} else {
					djStartResultQuiet(t, m, p, w, indexCalls, 2-indexCalls)
				}
			})
		}
	}
}
