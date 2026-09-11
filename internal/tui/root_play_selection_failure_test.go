package tui

import (
	"errors"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/tui/playback"
)

func TestPlaySelectionFailureAfterControlDispatchNotifies(t *testing.T) {
	for _, tc := range []struct {
		name    string
		key     tea.KeyPressMsg
		control playbackControl
	}{
		{"pause", tea.KeyPressMsg{Code: tea.KeySpace}, playbackControlPause},
		{"next", tea.KeyPressMsg{Code: 'n', Text: "n"}, playbackControlNext},
	} {
		for _, controlFirst := range []bool{false, true} {
			order := "selection result first"
			if controlFirst {
				order = "control settles first"
			}
			t.Run(tc.name+"/"+order, func(t *testing.T) {
				m, p := stoppableReplayModel([]int{0, 1, 2, 3, 4}, 0)
				p.playFromIndexErrs = []error{errStubPlay}
				sw := &stubSessionWriter{}
				m.sessionWriter = sw
				// Dispatch through Update, holding both the selection command and
				// its result across the actual transport keypress.
				next, selection := m.Update(playback.PlayQueueIndexMsg{Index: 2})
				m = next.(Model)
				next, control := m.Update(tc.key)
				m = next.(Model)
				request := m.playback.requestSeq
				if request == m.playback.playRequestSeq || control == nil {
					t.Fatal("keypress did not dispatch a newer control")
				}
				results := djWakeMessages(selection)
				if len(results) != 1 {
					t.Fatalf("selection results = %v, want one refusal", results)
				}
				refusal, ok := results[0].(playback.PlayFromIndexResultMsg)
				if !ok || !errors.Is(refusal.Err, errStubPlay) {
					t.Fatalf("selection result = %#v, want real player refusal", results[0])
				}
				settleControl := func() {
					t.Helper()
					messages := djWakeMessages(control)
					if len(messages) != 1 {
						t.Fatalf("control results = %v, want one", messages)
					}
					result := playbackControlResult(t, messages[0])
					if !result.ran || result.err != nil || result.request != request || result.control != tc.control {
						t.Fatalf("control result = %+v; newer control must execute normally", result)
					}
					writesBefore := len(sw.positionCalls)
					m = djWakeDrive(m, result, 4)
					if tc.control == playbackControlPause && len(sw.positionCalls) != writesBefore+1 {
						t.Error("successful pause result did not persist its session position")
					}
					if tc.control == playbackControlNext && !m.radio.start.succeeded {
						t.Error("successful Next result did not settle its start intent")
					}
				}
				if controlFirst {
					settleControl()
				}
				m = djWakeDrive(m, refusal, 4)
				if got := m.statusBar.TopNotification().Msg; !strings.Contains(got, "Play failed: "+errStubPlay.Error()) {
					t.Errorf("notification = %q, want selection failure", got)
				}
				if m.playback.state != playback.StateIdle || m.playback.currentIndex != -1 || m.playback.currentHash != "" || m.statusBar.Title != "" {
					t.Error("refused selection's optimistic playback display was not cleared")
				}
				if m.playback.pendingSelection || m.playback.awaitingSelectionStart {
					t.Error("refused selection is still awaiting confirmation")
				}
				if m.playback.requestSeq != request || m.playback.playRequestSeq != refusal.Request || p.stopCalls != 0 {
					t.Fatal("selection refusal superseded or stopped the newer control")
				}
				if !controlFirst {
					settleControl()
				}
				// Player events and command results arrive independently. Report
				// the physical outcome after both results have been delivered.
				if tc.control == playbackControlPause {
					m = djWakeDrive(m, playback.PauseChangedMsg{Paused: p.paused}, 4)
				} else {
					m = djWakeDrive(m, trackChangeFor(m, p.order[p.pos]), 4)
				}
				if tc.control == playbackControlNext && (m.playback.currentIndex != 1 || m.playback.state != playback.StatePlaying) {
					t.Errorf("Next left index=%d state=%v, want row 1 playing", m.playback.currentIndex, m.playback.state)
				}
				if tc.control == playbackControlPause && !p.paused {
					t.Error("newer pause did not pause the player")
				}
				if !slices.Equal(p.calls, []string{"PlayFromIndex", playbackControlLabel(tc.control)}) {
					t.Errorf("player calls = %v, want only selection and control", p.calls)
				}
			})
		}
	}
}
