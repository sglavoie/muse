package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/tui/playback"
)

type djUnpausePlayer struct {
	*djStartResultPlayer
	playErr error
}

func (p *djUnpausePlayer) Play() error {
	p.playCalls++
	return p.playErr
}

func djUnpauseStopped(t *testing.T) (Model, *djUnpausePlayer, *stubDJQueueWriter) {
	t.Helper()
	m, base, writer, late := djStartResultStopped(t)
	p := &djUnpausePlayer{djStartResultPlayer: base}
	m.player = p
	m = djStartResultDrain(m, late)
	// Stop keeps the last row loaded. Reconnection names it again, then the
	// pause property corrects the optimistic playing state of that report.
	m = djWakeDrive(m, djStopBoundaryTrackChange(), 4)
	m = djWakeDrive(m, playback.PauseChangedMsg{Paused: true}, 4)
	m = djWakeTicks(m, 3)
	if m.playback.state != playback.StatePaused || m.playback.currentIndex != 1 {
		t.Fatal("setup did not leave the stopped final row loaded and paused")
	}
	djUnpauseQuiet(t, m, p, writer, 0)
	return m, p, writer
}

func djUnpauseQuiet(t *testing.T, m Model, p *djUnpausePlayer, w *stubDJQueueWriter, playCalls int) {
	t.Helper()
	if !m.radio.stoppedByUser || len(w.prepared) != 1 || len(w.accepted) != 0 || len(m.queue.Entries) != 2 {
		t.Errorf("unexpected refill: stopped=%v preparations=%d accepts=%d rows=%d; want true/1/0/2",
			m.radio.stoppedByUser, len(w.prepared), len(w.accepted), len(m.queue.Entries))
	}
	if p.playCalls != playCalls || len(p.playFromIndexCalls) != 0 {
		t.Errorf("unexpected player starts: Play=%d index=%v; want %d/[]",
			p.playCalls, p.playFromIndexCalls, playCalls)
	}
}

func djUnpauseSpace(t *testing.T, m Model) (Model, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	if cmd == nil {
		t.Fatal("Space did not dispatch Play")
	}
	return next.(Model), cmd
}

func TestAutoDJStopResumesOnSuccessfulUnpause(t *testing.T) {
	for _, eventFirst := range []bool{true, false} {
		name := "result before pause event"
		if eventFirst {
			name = "pause event before result"
		}
		t.Run(name, func(t *testing.T) {
			m, p, w := djUnpauseStopped(t)
			m, cmd := djUnpauseSpace(t, m)
			result := playbackControlResult(t, cmd())
			if result.control != playbackControlPlay || result.err != nil || !result.ran || p.playCalls != 1 {
				t.Fatalf("Space did not successfully execute Play: %+v, calls=%d", result, p.playCalls)
			}
			if eventFirst {
				m = djWakeDrive(m, playback.PauseChangedMsg{Paused: false}, 4)
				m = djWakeTicks(m, 3)
				djUnpauseQuiet(t, m, p, w, 1)
			}
			// Hold the refill's preparation across the last track's end. There
			// is deliberately no TrackChangedMsg anywhere after Space.
			next, refill := m.Update(result)
			m = next.(Model)
			prepared := djWakeMessages(refill)
			if !eventFirst {
				m = djWakeDrive(m, playback.PauseChangedMsg{Paused: false}, 4)
			}
			if m.radio.stoppedByUser || len(w.prepared) != 2 || len(w.accepted) != 0 {
				t.Fatalf("successful unpause did not launch a refill: stopped=%v prepared=%d accepted=%d",
					m.radio.stoppedByUser, len(w.prepared), len(w.accepted))
			}
			m = djWakeDrive(m, playback.PlaybackEndedMsg{}, 4)
			m = djStartResultDrain(m, prepared)
			djStopIntentAssertResumed(t, w, m, 2)
			if m.playback.state != playback.StatePlaying || m.playback.queueExhausted || m.playback.currentIndex != 2 {
				t.Errorf("late refill stranded playback: state=%v exhausted=%v index=%d",
					m.playback.state, m.playback.queueExhausted, m.playback.currentIndex)
			}
			if p.playCalls != 1 || len(p.playFromIndexCalls) != 1 || p.playFromIndexCalls[0] != 2 {
				t.Errorf("want one deliberate unpause and continuation at first new row; Play=%d index=%v",
					p.playCalls, p.playFromIndexCalls)
			}
		})
		t.Run("refused Play/"+name, func(t *testing.T) {
			m, p, w := djUnpauseStopped(t)
			p.playErr = errStubPlay
			m, cmd := djUnpauseSpace(t, m)
			result := playbackControlResult(t, cmd())
			if eventFirst {
				m = djWakeDrive(m, playback.PauseChangedMsg{Paused: false}, 4)
			}
			m = djWakeDrive(m, result, 4)
			m = djWakeDrive(m, playback.PauseChangedMsg{Paused: false}, 4)
			m = djWakeTicks(m, 3)
			djUnpauseQuiet(t, m, p, w, 1)
		})
	}
	for _, held := range []string{"command", "result", "reported result"} {
		t.Run("newer Stop supersedes "+held, func(t *testing.T) {
			m, p, w := djUnpauseStopped(t)
			m, cmd := djUnpauseSpace(t, m)
			var result playbackControlResultMsg
			calls := 0
			if held != "command" {
				result = playbackControlResult(t, cmd())
				calls = 1
			}
			if held == "reported result" {
				m = djWakeDrive(m, playback.PauseChangedMsg{Paused: false}, 4)
			}
			m = djStartResultStop(t, m)
			if held == "command" {
				result = playbackControlResult(t, cmd())
			}
			m = djWakeDrive(m, result, 4)
			m = djWakeDrive(m, playback.PauseChangedMsg{Paused: false}, 4)
			m = djWakeTicks(m, 3)
			djUnpauseQuiet(t, m, p, w, calls)
		})
	}
	t.Run("unsolicited unpause", func(t *testing.T) {
		m, p, w := djUnpauseStopped(t)
		m = djWakeDrive(m, playback.PauseChangedMsg{Paused: false}, 4)
		m = djWakeTicks(m, 3)
		djUnpauseQuiet(t, m, p, w, 0)
	})
	t.Run("ordinary paused startup and pause resume", func(t *testing.T) {
		w := &stubDJQueueWriter{}
		m, p := djStopBoundaryModel(w)
		m.playback.state = playback.StatePaused
		m = djWakeDrive(m, playback.PauseChangedMsg{Paused: true}, 4)
		m = djWakeTicks(m, 3)
		if m.playback.state != playback.StatePaused || p.playCalls != 0 {
			t.Fatal("paused startup started playback")
		}
		for _, paused := range []bool{false, true, false} {
			m = djWakeDrive(m, tea.KeyPressMsg{Code: tea.KeySpace}, 4)
			m = djWakeDrive(m, playback.PauseChangedMsg{Paused: paused}, 4)
			m = djWakeTicks(m, 3)
			want := playback.StatePlaying
			if paused {
				want = playback.StatePaused
			}
			if m.playback.state != want || m.radio.stoppedByUser {
				t.Errorf("pause/resume state=%v stopped=%v; want %v/false", m.playback.state, m.radio.stoppedByUser, want)
			}
		}
		if p.playCalls != 2 || p.pauseCalls != 1 || len(p.playFromIndexCalls) != 0 {
			t.Errorf("ordinary pause/resume calls: Play=%d Pause=%d index=%v", p.playCalls, p.pauseCalls, p.playFromIndexCalls)
		}
		if len(w.prepared) != 0 || len(w.accepted) != 0 || len(m.queue.Entries) != 2 {
			t.Error("ordinary pause/resume unexpectedly launched DJ work without a prior suspension")
		}
	})
}
