package tui

import (
	"fmt"
	"log/slog"
	"time"

	tea "charm.land/bubbletea/v2"
)

// handlePlaybackKeyAction handles playback-related key actions (play/pause,
// stop, next, previous, seek, volume, repeat, shuffle, preview, now-playing).
// It returns true in the third value when the action was handled.
func (m Model) handlePlaybackKeyAction(action action) (tea.Model, tea.Cmd, bool) {
	switch action {
	case actionTrackInfo:
		if m.overlay.trackInfo != nil {
			m.overlay.trackInfo = nil
			return m, nil, true
		}
		track, ok := m.focusedTrackForPreview()
		if !ok {
			m.statusBar.notification = newNotification("No track to inspect")
			return m, nil, true
		}
		if track.Path == "" {
			if t, ok := m.library.index.FindByHash(track.Hash); ok {
				track = t
			}
		}
		if track.Path == "" {
			m.statusBar.notification = newNotification("Track path unknown")
			return m, nil, true
		}
		return m, trackInfoCmd(m.appCtx, track.Path, track.Hash, track.PlayCount), true
	case actionNowPlayingInfo:
		if m.overlay.nowPlaying != nil {
			m.overlay.nowPlaying = nil
			return m, nil, true
		}
		if m.playback.state == PlaybackStopped || m.playback.state == PlaybackIdle {
			m.statusBar.notification = newNotification("No track playing")
			return m, nil, true
		}
		overlay := NowPlayingOverlay{
			artist:   m.statusBar.artist,
			title:    m.statusBar.title,
			elapsed:  m.playback.elapsedSecs,
			total:    m.playback.totalSecs,
			state:    m.playback.state,
			tagsLine: m.buildTagsLine(m.playback.currentHash),
		}
		if m.playback.currentHash != "" {
			if t, ok := m.library.index.FindByHash(m.playback.currentHash); ok {
				if overlay.artist == "" {
					overlay.artist = t.Artist
				}
				if overlay.title == "" {
					overlay.title = t.Title
				}
				overlay.album = t.Album
				overlay.playCount = t.PlayCount
			}
		}
		m.overlay.nowPlaying = &overlay
		return m, nil, true
	case actionPreview:
		if m.overlay.preview != nil {
			m = m.dismissPreview()
			return m, nil, true
		}
		track, ok := m.focusedTrackForPreview()
		if !ok {
			m.statusBar.notification = newNotification("No track to preview")
			return m, nil, true
		}
		m.overlay.previewWasPlaying = m.playback.state == PlaybackPlaying
		if m.overlay.previewWasPlaying && m.player != nil {
			if err := m.player.Pause(); err != nil {
				slog.Warn("player pause failed", "err", err)
			}
			m.playback.state = PlaybackPaused
		}
		return m, m.startPreviewCmd(track), true
	case actionPlayPause:
		if m.player != nil {
			switch {
			case m.playback.state == PlaybackPlaying:
				if err := m.player.Pause(); err != nil {
					slog.Warn("pause failed", "err", err)
					m.statusBar.notification = newNotification("Pause failed: " + err.Error())
				} else {
					m.playback.state = PlaybackPaused
				}
				return m, m.sessionPositionCmd(), true
			case len(m.queue.entries) == 0:
				m.statusBar.notification = newNotification("Queue is empty")
				return m, nil, true
			default:
				if err := m.player.Play(); err != nil {
					slog.Warn("play failed", "err", err)
					m.statusBar.notification = newNotification("Play failed: " + err.Error())
				} else {
					m.playback.state = PlaybackPlaying
				}
			}
		}
		return m, nil, true
	case actionStop:
		if cp, ok := m.player.(commandPlayer); ok {
			if err := cp.Stop(); err != nil {
				slog.Warn("stop failed", "err", err)
				m.statusBar.notification = newNotification("Stop failed: " + err.Error())
			} else {
				m.playback.state = PlaybackStopped
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
	case actionNext:
		if cp, ok := m.player.(commandPlayer); ok {
			if err := cp.Next(); err != nil {
				slog.Warn("next failed", "err", err)
				m.statusBar.notification = newNotification("Next failed: " + err.Error())
			}
		}
		return m, nil, true
	case actionPrevious:
		if cp, ok := m.player.(commandPlayer); ok {
			if err := cp.Previous(); err != nil {
				slog.Warn("previous failed", "err", err)
				m.statusBar.notification = newNotification("Previous failed: " + err.Error())
			}
		}
		return m, nil, true
	case actionSeekBack:
		return m.handleSeek(-m.cfg.SeekOffsetShort)
	case actionSeekForward:
		return m.handleSeek(m.cfg.SeekOffsetShort)
	case actionSeekBackLong:
		return m.handleSeek(-m.cfg.SeekOffsetLong)
	case actionSeekForwardLong:
		return m.handleSeek(m.cfg.SeekOffsetLong)
	case actionVolumeUp:
		return m.handleVolume(1)
	case actionVolumeDown:
		return m.handleVolume(-1)
	case actionRepeatCycle:
		switch m.playback.repeatMode {
		case RepeatOff:
			m.playback.repeatMode = RepeatAll
		case RepeatAll:
			m.playback.repeatMode = RepeatOne
		case RepeatOne:
			m.playback.repeatMode = RepeatOff
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
		m.statusBar.notification = newNotificationFor("Repeat: "+m.playback.repeatMode.DisplayString(), time.Duration(m.cfg.VolumeFeedbackDurationSecs)*time.Second)
		return m, nil, true
	case actionShuffleToggle:
		m.playback.shuffle = !m.playback.shuffle
		if rs, ok := m.player.(repeatShuffler); ok {
			if err := rs.SetShuffle(m.playback.shuffle); err != nil {
				slog.Warn("set shuffle failed", "err", err)
			}
		}
		if m.sessionWriter != nil {
			if err := m.sessionWriter.WriteShuffle(m.playback.shuffle); err != nil {
				slog.Warn("write shuffle failed", "err", err)
			}
		}
		label := "off"
		if m.playback.shuffle {
			label = "on"
		}
		m.statusBar.notification = newNotificationFor("Shuffle: "+label, time.Duration(m.cfg.VolumeFeedbackDurationSecs)*time.Second)
		return m, nil, true
	case actionStopAfterCurrent:
		if m.playback.state != PlaybackPlaying && m.playback.state != PlaybackPaused {
			m.statusBar.notification = newNotification("No track playing")
			return m, nil, true
		}
		m.playback.stopAfterCurrent = !m.playback.stopAfterCurrent
		if sat, ok := m.player.(stopAfterToggler); ok {
			sat.SetStopAfterCurrent(m.playback.stopAfterCurrent)
		}
		label := "off"
		if m.playback.stopAfterCurrent {
			label = "on"
		}
		m.statusBar.notification = newNotificationFor("Stop after current: "+label, time.Duration(m.cfg.VolumeFeedbackDurationSecs)*time.Second)
		return m, nil, true
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
		m.playback.volume = clampVolume(m.playback.volume + delta)
		if m.sessionWriter != nil {
			if err := m.sessionWriter.WriteVolume(m.playback.volume); err != nil {
				slog.Warn("write volume failed", "err", err)
			}
		}
	}
	m.statusBar.notification = newNotificationFor(fmt.Sprintf("Volume: %d%%", m.playback.volume), time.Duration(m.cfg.VolumeFeedbackDurationSecs)*time.Second)
	return m, nil, true
}
