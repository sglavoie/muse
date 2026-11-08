package tui

import (
	"log/slog"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/player"
	libtui "github.com/sglavoie/muse/internal/tui/library"
	"github.com/sglavoie/muse/internal/tui/playback"
)

// handlePlayerDisconnectedMsg resets TUI playback state when mpv disconnects
// unexpectedly so the play/pause icon and progress bar are not stale during
// the reconnection window.
func (m Model) handlePlayerDisconnectedMsg() Model {
	m = m.endCurrentListen()
	m.playback.state = playback.StateStopped
	m = m.resetPlaybackProgress()
	m.statusBar.Notify("mpv disconnected, restarting…")
	return m
}

// handlePauseChangedMsg syncs the TUI playback state with mpv's actual pause
// property. Guards prevent redundant transitions (e.g. user presses space ->
// TUI already updated -> mpv confirms via event -> no-op).
func (m Model) handlePauseChangedMsg(msg playback.PauseChangedMsg) (Model, tea.Cmd) {
	if msg.Paused {
		if m.playback.state == playback.StatePlaying {
			m.playback.state = playback.StatePaused
			return m, m.sessionPositionCmd()
		}
	} else {
		if m.playback.state == playback.StatePaused || m.playback.state == playback.StateStopped {
			m.playback.state = playback.StatePlaying
		}
	}
	return m, nil
}

// handlePreviewStartedMsg stores the preview player and overlay state.
func (m Model) handlePreviewStartedMsg(msg previewStartedMsg) Model {
	m.overlay.previewStarting = false
	if msg.request != 0 && msg.request != m.playback.requestSeq {
		_ = msg.player.Close()
		m.overlay.previewWasPlaying = false
		return m
	}
	m.overlay.previewPlayer = msg.player
	m.overlay.preview = &msg.overlay
	m.overlay.previewDurationFilled = false
	return m
}

// handlePreviewPauseResultMsg starts the preview only after the ordered pause
// succeeded and only while no newer playback request has superseded it.
func (m Model) handlePreviewPauseResultMsg(msg previewPauseResultMsg) (Model, tea.Cmd) {
	if msg.request != m.playback.requestSeq || !msg.ran {
		m.overlay.previewStarting = false
		m.overlay.previewWasPlaying = false
		return m, nil
	}
	if msg.err != nil {
		if m.playback.state == playback.StatePaused {
			m.playback.state = playback.StatePlaying
		}
		m.overlay.previewStarting = false
		m.overlay.previewWasPlaying = false
		slog.Warn("player pause for preview failed", "err", msg.err)
		m.statusBar.NotifyError("Preview pause failed: " + msg.err.Error())
		return m, nil
	}
	return m, m.startPreviewCmd(msg.track, msg.request)
}

// handlePreviewStartErrorMsg notifies the user and resumes main playback.
func (m Model) handlePreviewStartErrorMsg(msg previewStartErrorMsg) (Model, tea.Cmd) {
	m.overlay.previewStarting = false
	m.statusBar.NotifyError("Preview error: " + msg.err.Error())
	player.PlaySound(player.SoundError)
	if msg.request != 0 && msg.request != m.playback.requestSeq {
		m.overlay.previewWasPlaying = false
		return m, nil
	}
	return m.resumeAfterPreview()
}

// handlePlayRecordedMsg prepends the just-played track to the in-memory
// library History mode so it updates in realtime without a full DB reload.
func (m Model) handlePlayRecordedMsg(msg playback.PlayRecordedMsg) Model {
	// The listen is in play_events now, so every Insights aggregate that counts
	// it — including the skip counts that gate mix readiness — is out of date.
	m.insightsInvalidate()

	// History reads the counted-only play_history view, so prepending an
	// uncounted listen would show a row that vanishes on the next reload.
	if !msg.Counted {
		return m
	}

	// Only a counted listen moves the Auto-DJ's inputs: PlayCount and
	// LastPlayedAt decide discovery classification, and the completion-rate and
	// co-occurrence signals read the same counted-only history. A skip changes
	// neither, which is why this sits below the guard rather than beside the
	// Insights bump above it.
	m.djInvalidate()

	// Only update when History mode is active and has been loaded.
	if m.library.Mode != libtui.LibraryModeHistory || len(m.library.SourceTracks) == 0 {
		return m
	}

	t, ok := m.library.Index.FindByHash(msg.Hash)
	if !ok {
		return m
	}
	t.LastPlayedAt = msg.PlayedAt

	// Prepend and cap at 500 entries. Drop any pre-existing entry for the
	// same hash so replaying a track in the same session does not produce
	// stacked duplicates at the top of History.
	const maxHistory = 500
	tracks := make([]library.Track, 0, min(len(m.library.SourceTracks)+1, maxHistory))
	tracks = append(tracks, t)
	for _, existing := range m.library.SourceTracks {
		if existing.Hash == msg.Hash {
			continue
		}
		if len(tracks) >= maxHistory {
			break
		}
		tracks = append(tracks, existing)
	}
	m.library.SourceTracks = tracks

	m.library = m.library.RefreshView().ClampCursor()
	return m
}
