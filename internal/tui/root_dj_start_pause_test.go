package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/tui/playback"
)

func TestAutoDJStopResumesWhenPauseRacesStartConfirmation(t *testing.T) {
	for _, selection := range []bool{true, false} {
		name := "Next with preview pause"
		if selection {
			name = "selection with Space pause"
		}
		for _, reportFirst := range []bool{true, false} {
			order := "result before report"
			if reportFirst {
				order = "report before result"
			}
			t.Run(name+"/"+order, func(t *testing.T) {
				w := &stubDJQueueWriter{}
				m, base := djStopBoundaryModel(w)
				p := &djStartResultPlayer{stubStopAfterPlayer: base}
				m.player = p
				m.ActiveView = viewQueue
				// Hold a real unpublished batch across an actual Stop keypress.
				next, cmd := m.Update(djStopBoundaryTrackChange())
				m = next.(Model)
				late := djWakeMessages(cmd)
				m = djWakeDrive(m, tea.KeyPressMsg{Code: 's', Text: "s"}, 4)
				m = djWakeDrive(m, playback.PlaybackEndedMsg{}, 4)
				m = djStartResultDrain(m, late)
				if p.stopCalls != 1 || m.playback.state != playback.StateIdle || len(w.rejected) != 1 {
					t.Fatal("setup did not stop playback and retire the held batch")
				}
				djStartResultQuiet(t, m, p, w, 0, 0)
				if !selection {
					// Stop leaves the old row loaded; reconnect reports it paused
					// before the user requests Next. This must not arm a start.
					m = djWakeDrive(m, djStopBoundaryTrackChange(), 4)
					m = djWakeDrive(m, playback.PauseChangedMsg{Paused: true}, 4)
				}

				var gesture tea.Msg = tea.KeyPressMsg{Code: 'n', Text: "n"}
				if selection {
					gesture = playback.PlayQueueIndexMsg{Index: 1}
				}
				next, start := m.Update(gesture)
				m = next.(Model)
				request := m.radio.start.request
				djStartResultQuiet(t, m, p, w, 0, 0)
				results := djWakeMessages(start)
				if len(results) != 1 || request == 0 {
					t.Fatalf("start did not execute: request=%d results=%v", request, results)
				}
				indexCalls, transportCalls := 0, 1
				pauseKey := tea.KeyPressMsg{Code: 'P', Text: "P"}
				if selection {
					indexCalls, transportCalls = 1, 0
					pauseKey = tea.KeyPressMsg{Code: tea.KeySpace}
					result, ok := results[0].(playback.PlayFromIndexResultMsg)
					if !ok || result.Err != nil || result.Request != request {
						t.Fatalf("selection did not succeed: %#v", results[0])
					}
				} else {
					result := playbackControlResult(t, results[0])
					if !result.ran || result.err != nil || result.control != playbackControlNext {
						t.Fatalf("Next did not succeed: %+v", result)
					}
					// Next has started audio, but its track report and command result
					// are still in flight. The pause property alone authorizes nothing.
					m = djWakeDrive(m, playback.PauseChangedMsg{Paused: false}, 4)
				}
				djStartResultQuiet(t, m, p, w, indexCalls, transportCalls)
				next, pause := m.Update(pauseKey)
				m = next.(Model)
				if pause == nil || m.playback.requestSeq <= request || m.radio.start.request != request {
					t.Fatal("pause did not dispatch behind the outstanding start")
				}
				pauseResults := djWakeMessages(pause)
				if p.pauseCalls != 1 || len(pauseResults) != 1 {
					t.Fatal("pause command did not execute")
				}
				if selection {
					result := playbackControlResult(t, pauseResults[0])
					if !result.ran || result.err != nil || result.control != playbackControlPause {
						t.Fatalf("Space did not pause: %+v", result)
					}
					m = djStartResultDrain(m, pauseResults)
				} else {
					result, ok := pauseResults[0].(previewPauseResultMsg)
					if !ok || !result.ran || result.err != nil {
						t.Fatalf("preview did not pause: %#v", pauseResults[0])
					}
					// Hold preview activation: only its main-player pause is relevant.
				}
				djStartResultQuiet(t, m, p, w, indexCalls, transportCalls)
				if reportFirst {
					m = djWakeDrive(m, djStopBoundaryTrackChange(), 4)
				} else {
					m = djStartResultDrain(m, results)
				}
				// Neither half alone may prepare or publish another refill.
				djStartResultQuiet(t, m, p, w, indexCalls, transportCalls)
				if reportFirst {
					m = djStartResultDrain(m, results)
				} else {
					m = djWakeDrive(m, djStopBoundaryTrackChange(), 4)
				}
				if m.radio.stoppedByUser || len(w.prepared) != 2 {
					t.Errorf("confirmed start: suspended=%v preparations=%d; want false/2",
						m.radio.stoppedByUser, len(w.prepared))
				}
				djStopIntentAssertResumed(t, w, m, 2)
			})
		}
	}
}
