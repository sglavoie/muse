package tui

import (
	"fmt"
	"log/slog"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/tui/keys"
	"github.com/sglavoie/muse/internal/tui/overlay"
	"github.com/sglavoie/muse/internal/tui/playback"
)

// handlePlaybackKeyAction handles playback-related key actions (play/pause,
// stop, next, previous, seek, volume, repeat, shuffle, preview, now-playing).
// It returns true in the third value when the action was handled.
func (m Model) handlePlaybackKeyAction(action keys.Action) (tea.Model, tea.Cmd, bool) {
	switch action {
	case keys.ActionTrackInfo:
		if m.overlay.trackInfo != nil {
			m.overlay.trackInfo = nil
			return m, nil, true
		}
		track, ok := m.focusedTrackForPreview()
		if !ok {
			m.statusBar.notify("No track to inspect")
			return m, nil, true
		}
		if track.Path == "" {
			if t, ok := m.library.index.FindByHash(track.Hash); ok {
				track = t
			}
		}
		if track.Path == "" {
			m.statusBar.notify("Track path unknown")
			return m, nil, true
		}
		return m, overlay.TrackInfoCmd(m.appCtx, track.Path, track.Hash, track.PlayCount), true
	case keys.ActionNowPlayingInfo:
		if m.overlay.nowPlaying != nil {
			m.overlay.nowPlaying = nil
			return m, nil, true
		}
		if m.playback.state == playback.StateStopped || m.playback.state == playback.StateIdle {
			m.statusBar.notify("No track playing")
			return m, nil, true
		}
		overlay := overlay.NowPlayingOverlay{
			Artist:   m.statusBar.artist,
			Title:    m.statusBar.title,
			Elapsed:  m.playback.elapsedSecs,
			Total:    m.playback.totalSecs,
			State:    m.playback.state,
			TagsLine: m.buildTagsLine(m.playback.currentHash),
		}
		if m.playback.currentHash != "" {
			if t, ok := m.library.index.FindByHash(m.playback.currentHash); ok {
				if overlay.Artist == "" {
					overlay.Artist = t.Artist
				}
				if overlay.Title == "" {
					overlay.Title = t.Title
				}
				overlay.Album = t.Album
				overlay.PlayCount = t.PlayCount
			}
		}
		m.overlay.nowPlaying = &overlay
		return m, nil, true
	case keys.ActionPreview:
		if m.overlay.preview != nil {
			m = m.dismissPreview()
			return m, nil, true
		}
		track, ok := m.focusedTrackForPreview()
		if !ok {
			m.statusBar.notify("No track to preview")
			return m, nil, true
		}
		m.overlay.previewWasPlaying = m.playback.state == playback.StatePlaying
		if m.overlay.previewWasPlaying && m.player != nil {
			if err := m.player.Pause(); err != nil {
				slog.Warn("player pause failed", "err", err)
			}
			m.playback.state = playback.StatePaused
		}
		return m, m.startPreviewCmd(track), true
	case keys.ActionPlayPause:
		if m.player != nil {
			switch {
			case m.playback.state == playback.StatePlaying:
				if err := m.player.Pause(); err != nil {
					slog.Warn("pause failed", "err", err)
					m.statusBar.notify("Pause failed: " + err.Error())
				} else {
					m.playback.state = playback.StatePaused
				}
				return m, m.sessionPositionCmd(), true
			case len(m.queue.entries) == 0:
				m.statusBar.notify("Queue is empty")
				return m, nil, true
			default:
				if m.playback.state == playback.StatePaused {
					if err := m.player.Play(); err != nil {
						slog.Warn("play failed", "err", err)
						m.statusBar.notify("Play failed: " + err.Error())
					} else {
						m.playback.state = playback.StatePlaying
					}
				} else {
					idx := m.playback.currentIndex
					if idx < 0 || idx >= len(m.queue.entries) {
						idx = 0
					}
					return m, func() tea.Msg { return playback.PlayQueueIndexMsg{Index: idx} }, true
				}
			}
		}
		return m, nil, true
	case keys.ActionStop:
		if cp, ok := m.player.(commandPlayer); ok {
			if err := cp.Stop(); err != nil {
				slog.Warn("stop failed", "err", err)
				m.statusBar.notify("Stop failed: " + err.Error())
			} else {
				m = m.endCurrentListen()
				m.playback.state = playback.StateStopped
				m.playback.elapsedSecs = 0
				m.playback.totalSecs = 0
				if m.playback.stopAfterCurrent {
					m.playback.stopAfterCurrent = false
					if sat, ok := m.player.(stopAfterToggler); ok {
						sat.SetStopAfterCurrent(false)
					}
				}
			}
		}
		return m, nil, true
	case keys.ActionResetPosition:
		if cp, ok := m.player.(commandPlayer); ok {
			if err := cp.SeekAbsolute(0); err != nil {
				slog.Warn("reset position failed", "err", err)
			} else {
				m.playback.elapsedSecs = 0
			}
		}
		return m, m.sessionPositionCmd(), true
	case keys.ActionNext:
		if cp, ok := m.player.(commandPlayer); ok {
			if err := cp.Next(); err != nil {
				slog.Warn("next failed", "err", err)
				m.statusBar.notify("Next failed: " + err.Error())
			}
		}
		return m, nil, true
	case keys.ActionPrevious:
		if cp, ok := m.player.(commandPlayer); ok {
			if err := cp.Previous(); err != nil {
				slog.Warn("previous failed", "err", err)
				m.statusBar.notify("Previous failed: " + err.Error())
			}
		}
		return m, nil, true
	case keys.ActionSeekBack:
		return m.handleSeek(-m.cfg.SeekOffsetShort)
	case keys.ActionSeekForward:
		return m.handleSeek(m.cfg.SeekOffsetShort)
	case keys.ActionSeekBackLong:
		return m.handleSeek(-m.cfg.SeekOffsetLong)
	case keys.ActionSeekForwardLong:
		return m.handleSeek(m.cfg.SeekOffsetLong)
	case keys.ActionVolumeUp:
		return m.handleVolume(1)
	case keys.ActionVolumeDown:
		return m.handleVolume(-1)
	case keys.ActionRepeatCycle:
		switch m.playback.repeatMode {
		case playback.RepeatOff:
			m.playback.repeatMode = playback.RepeatAll
		case playback.RepeatAll:
			m.playback.repeatMode = playback.RepeatOne
		case playback.RepeatOne:
			m.playback.repeatMode = playback.RepeatOff
		}
		if rs, ok := m.player.(repeatShuffler); ok {
			if err := rs.SetRepeat(m.playback.repeatMode.String()); err != nil {
				slog.Warn("set repeat failed", "err", err)
			}
		}
		if m.sessionWriter != nil {
			if err := m.sessionWriter.WriteRepeatMode(m.playback.repeatMode.String()); err != nil {
				slog.Warn("write repeat mode failed", "err", err)
			}
		}
		m.statusBar.pushMsg(m.modeToggleMsg("Repeat", playback.ModeIndexRepeat), time.Duration(m.cfg.VolumeFeedbackDurationSecs)*time.Second, notifInfo)
		return m, nil, true
	case keys.ActionShuffleToggle:
		m.playback.shuffle = !m.playback.shuffle
		label := playback.ModeState(m.playback.shuffle)
		// Surface a runtime failure to the user: the traversal order did not
		// change, so revert the toggle and do not persist a mismatched flag.
		if rs, ok := m.player.(repeatShuffler); ok {
			if err := rs.SetShuffle(m.playback.shuffle); err != nil {
				slog.Warn("set shuffle failed", "err", err)
				m.playback.shuffle = !m.playback.shuffle
				m.statusBar.notify("Shuffle failed: " + err.Error())
				return m, nil, true
			}
		}
		// A persistence failure leaves runtime shuffle active but the flag will
		// not survive a restart — tell the user rather than failing silently.
		if m.sessionWriter != nil {
			if err := m.sessionWriter.WriteShuffle(m.playback.shuffle); err != nil {
				slog.Warn("write shuffle failed", "err", err)
				m.statusBar.notify(fmt.Sprintf("Shuffle %s (not saved — won't survive restart)", label))
				return m, nil, true
			}
		}
		m.statusBar.pushMsg(m.modeToggleMsg("Shuffle", playback.ModeIndexShuffle), time.Duration(m.cfg.VolumeFeedbackDurationSecs)*time.Second, notifInfo)
		return m, nil, true
	case keys.ActionStopAfterCurrent:
		if m.playback.state != playback.StatePlaying && m.playback.state != playback.StatePaused {
			m.statusBar.notify("No track playing")
			return m, nil, true
		}
		m.playback.stopAfterCurrent = !m.playback.stopAfterCurrent
		if sat, ok := m.player.(stopAfterToggler); ok {
			sat.SetStopAfterCurrent(m.playback.stopAfterCurrent)
		}
		m.statusBar.pushMsg(m.modeToggleMsg("Stop after current", playback.ModeIndexStopAfter), time.Duration(m.cfg.VolumeFeedbackDurationSecs)*time.Second, notifInfo)
		return m, nil, true
	case keys.ActionRadioToggle:
		return m.radioToggle()
	}
	return m, nil, false
}

func (m Model) handleSeek(seconds float64) (tea.Model, tea.Cmd, bool) {
	if cp, ok := m.player.(commandPlayer); ok {
		if err := cp.SeekRelative(seconds); err != nil {
			slog.Warn("seek failed", "err", err)
		}
		return m, m.sessionPositionCmd(), true
	}
	return m, nil, true
}

func (m Model) handleVolume(sign int) (tea.Model, tea.Cmd, bool) {
	cp, ok := m.player.(commandPlayer)
	if !ok {
		return m, nil, true
	}
	delta := sign * m.cfg.VolumeIncrement
	if err := cp.SetVolumeRelative(float64(delta)); err != nil {
		slog.Warn("volume change failed", "err", err)
	} else {
		m.playback.volume = max(0, min(m.playback.volume+delta, 100))
		if m.sessionWriter != nil {
			if err := m.sessionWriter.WriteVolume(m.playback.volume); err != nil {
				slog.Warn("write volume failed", "err", err)
			}
		}
	}
	m.statusBar.pushMsg(fmt.Sprintf("Volume: %d%%", m.playback.volume), time.Duration(m.cfg.VolumeFeedbackDurationSecs)*time.Second, notifInfo)
	return m, nil, true
}
