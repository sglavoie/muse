package tui

import (
	"log/slog"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/player"
	"github.com/sglavoie/muse/internal/tui/playback"
)

// This file holds what "stop after this song" does once it fires. The player
// ends the song instead of advancing — arming it already dropped the gapless
// successor mpv had preloaded, so not a fraction of the next song plays — and
// the TUI then asks it to cue that successor paused at its start. The queue is
// left exactly where the song ended: the next song is current, paused, and one
// press of play away.
//
// The flag is spent when it fires, and a manual skip spends it too: it was
// armed against the song that was playing, and that song is gone.

// nextCuer is implemented by player.Manager: load the entry that would have
// played next, paused at its start.
type nextCuer interface {
	CueNext() error
}

// stopAfterCueState tracks the cue dispatched after a stop-after-current.
// awaitingTrack holds from the dispatch until the cued song's track-change
// report arrives, which must leave the model paused rather than playing.
type stopAfterCueState struct {
	request       uint64
	awaitingTrack bool
}

// setStopAfterCurrent arms or disarms stop-after-current in the model and the
// player together.
func (m Model) setStopAfterCurrent(v bool) Model {
	m.playback.stopAfterCurrent = v
	if sat, ok := m.player.(stopAfterToggler); ok {
		sat.SetStopAfterCurrent(v)
	}
	return m
}

// cueAfterStop dispatches the cue once a stop-after-current has ended the song.
// It reports false when the player cannot cue, leaving the caller's ordinary
// "stopped" handling in charge.
//
// The model goes straight to paused on the row that finished, with its
// position reset, so nothing flickers through an idle state while the cue is
// on its way; the cued song's report then moves it onto the new row.
func (m Model) cueAfterStop() (Model, tea.Cmd, bool) {
	cuer, ok := m.player.(nextCuer)
	if !ok || len(m.queue.Entries) == 0 {
		return m, nil, false
	}
	gate := m.playback.requests
	req := gate.dispatchControl(m.playback.requestSeq)
	m.playback.requestSeq = req
	m.stopCue = stopAfterCueState{request: req, awaitingTrack: true}
	m.playback.state = playback.StatePaused
	m = m.resetPlaybackProgress()
	return m, func() tea.Msg {
		err, ran := gate.run(req, cuer.CueNext)
		return stopAfterCueResultMsg{request: req, err: err, ran: ran}
	}, true
}

// handleStopAfterCueResult finishes the stop. A cue a newer request overtook
// changes nothing — that request owns playback now. A cue with nothing to load
// is the end of the queue, reported as the plain stop it always was.
func (m Model) handleStopAfterCueResult(msg stopAfterCueResultMsg) (tea.Model, tea.Cmd) {
	if msg.request != m.stopCue.request {
		return m, nil
	}
	if !msg.ran || msg.request != m.playback.requestSeq {
		m.stopCue = stopAfterCueState{}
		return m, nil
	}
	if msg.err != nil {
		m.stopCue = stopAfterCueState{}
		slog.Debug("stop after current: nothing to cue", "err", msg.err)
		notice := m.queueEndNotice(true)
		m = m.idleAfterQueueEnd(false)
		m.statusBar.Notify(notice)
		player.PlayQueueFinishedSound()
		return m, nil
	}
	m.statusBar.Notify("Stopped after this song — next song is ready, paused")
	return m, m.sessionPositionCmd()
}

// settleStopAfterOnTrackChange applies a track change to stop-after-current.
// The cued song arrives paused. Any other move onto a different row is a skip,
// which spends the flag in the player as well as here; a report that only
// re-announces the same row (a reconnect) leaves it armed.
func (m Model) settleStopAfterOnTrackChange(prevHash string, prevIndex int) Model {
	if m.stopCue.awaitingTrack {
		m.stopCue.awaitingTrack = false
		m.playback.state = playback.StatePaused
		if m.overlay.nowPlaying != nil {
			m.overlay.nowPlaying.State = playback.StatePaused
		}
		return m
	}
	if !m.playback.stopAfterCurrent {
		return m
	}
	if prevHash == m.playback.currentHash && prevIndex == m.playback.currentIndex {
		return m
	}
	m = m.setStopAfterCurrent(false)
	m.statusBar.Notify("Stop after this song cleared by the skip")
	return m
}
