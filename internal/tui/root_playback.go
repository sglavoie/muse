package tui

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/player"
)

// handleTickMsg updates elapsed time for the main player, preview player, and
// now-playing overlay, and clears expired status bar notifications.
func (m Model) handleTickMsg() (Model, tea.Cmd) {
	if m.playback.state == PlaybackPlaying || m.playback.state == PlaybackPaused {
		if er, ok := m.player.(elapsedReader); ok {
			if elapsed, total, err := er.ElapsedPosition(); err == nil {
				m.playback.elapsedSecs = elapsed
				m.playback.totalSecs = total
			}
		}
	}
	if m.overlay.preview != nil && m.overlay.previewPlayer != nil {
		if elapsed, total, err := m.overlay.previewPlayer.ElapsedPosition(); err == nil {
			m.overlay.preview.elapsed = elapsed
			m.overlay.preview.total = total
		} else {
			// Preview track ended or mpv died — auto-dismiss.
			m = m.dismissPreview()
		}
	}
	if m.overlay.nowPlaying != nil {
		m.overlay.nowPlaying.elapsed = m.playback.elapsedSecs
		m.overlay.nowPlaying.total = m.playback.totalSecs
		m.overlay.nowPlaying.state = m.playback.state
	}
	if !m.statusBar.notification.expiresAt.IsZero() && time.Now().After(m.statusBar.notification.expiresAt) {
		m.statusBar.notification = transientNotification{}
	}
	if m.undo.clearQueuePending && time.Now().After(m.undo.clearQueuePendingAt.Add(queueConfirmationWindow)) {
		m.undo.clearQueuePending = false
	}
	return m, tickCmd()
}

// handleSyncPositionMsg persists the current playback position via the player.
func (m Model) handleSyncPositionMsg() (Model, tea.Cmd) {
	if ps, ok := m.player.(positionSyncer); ok {
		_ = ps.SyncPosition()
	}
	return m, syncPositionCmd()
}

// handleSessionTickMsg fires a periodic session position update when playing.
func (m Model) handleSessionTickMsg() (Model, tea.Cmd) {
	var cmd tea.Cmd
	if m.playback.state == PlaybackPlaying {
		cmd = m.sessionPositionCmd()
	}
	return m, tea.Batch(sessionTickCmd(), cmd)
}

// handlePlayerStatusMsg shows a transient player status notification.
func (m Model) handlePlayerStatusMsg(msg PlayerStatusMsg) Model {
	m.statusBar.notification = newNotification(msg.Text)
	return m
}

// handleMissingTrackMsg marks a queue entry as missing, increments the missing
// track counter, and notifies the status bar.
func (m Model) handleMissingTrackMsg(msg MissingTrackMsg) Model {
	m.queue.markMissing(msg.Path)
	m.statusBar.handleMissingTrack(msg.Path)
	m.missingTrackCount++
	player.PlaySound(player.SoundError)
	return m
}

// handlePlaybackEndedMsg handles the end of the play queue, optionally
// wrapping around in repeat-all mode.
func (m Model) handlePlaybackEndedMsg() Model {
	if m.playback.repeatMode == RepeatAll && len(m.queue.entries) > 0 && m.player != nil {
		if err := m.player.PlayFromIndex(0); err == nil {
			m.playback.state = PlaybackPlaying
			e := m.queue.entries[0]
			m.playback.currentHash = e.Hash
			m.statusBar.artist = e.Artist
			m.statusBar.title = e.Title
			m.statusBar.playlistSource = "Queue"
			m.playback.elapsedSecs = 0
			m.playback.totalSecs = 0
			return m
		}
		slog.Warn("repeat-all playback failed", "err", "PlayFromIndex(0)")
	}
	m.playback.state = PlaybackIdle
	m.playback.elapsedSecs = 0
	m.playback.totalSecs = 0
	m.statusBar.artist = ""
	m.statusBar.title = ""
	m.statusBar.playlistSource = ""
	m.playback.currentHash = ""
	if m.playback.repeatMode == RepeatOff {
		msg := "Queue finished"
		if m.missingTrackCount > 0 {
			msg = fmt.Sprintf("Queue finished (%d %s skipped — missing files)",
				m.missingTrackCount, pluralize(m.missingTrackCount, "track", "tracks"))
		}
		m.missingTrackCount = 0
		m.statusBar.notification = newNotification(msg)
		player.PlayQueueFinishedSound()
	}
	return m
}

// handlePlayerDisconnectedMsg resets TUI playback state when mpv disconnects
// unexpectedly so the play/pause icon and progress bar are not stale during
// the reconnection window.
func (m Model) handlePlayerDisconnectedMsg() Model {
	m.playback.state = PlaybackStopped
	m.playback.elapsedSecs = 0
	m.playback.totalSecs = 0
	m.statusBar.notification = newNotification("mpv disconnected, restarting…")
	return m
}

// handlePauseChangedMsg syncs the TUI playback state with mpv's actual pause
// property. Guards prevent redundant transitions (e.g. user presses space →
// TUI already updated → mpv confirms via event → no-op).
func (m Model) handlePauseChangedMsg(msg PauseChangedMsg) (Model, tea.Cmd) {
	if msg.Paused {
		if m.playback.state == PlaybackPlaying {
			m.playback.state = PlaybackPaused
			return m, m.sessionPositionCmd()
		}
	} else {
		if m.playback.state == PlaybackPaused || m.playback.state == PlaybackStopped {
			m.playback.state = PlaybackPlaying
		}
	}
	return m, nil
}

// handlePreviewStartedMsg stores the preview player and overlay state.
func (m Model) handlePreviewStartedMsg(msg previewStartedMsg) Model {
	m.overlay.previewPlayer = msg.player
	m.overlay.preview = &msg.overlay
	return m
}

// handlePreviewStartErrorMsg notifies the user and resumes main playback.
func (m Model) handlePreviewStartErrorMsg(msg previewStartErrorMsg) Model {
	m.statusBar.notification = newNotification("Preview error: " + msg.err.Error())
	player.PlaySound(player.SoundError)
	// Resume main player if we paused it.
	if m.overlay.previewWasPlaying && m.player != nil {
		if err := m.player.Play(); err != nil {
			slog.Debug("player resume failed", "err", err)
		}
		m.playback.state = PlaybackPlaying
	}
	m.overlay.previewWasPlaying = false
	return m
}

// handleTrackChangedMsg updates the status bar and playback state when the
// player advances to a new track.
func (m Model) handleTrackChangedMsg(msg TrackChangedMsg) (Model, tea.Cmd) {
	m.playback.currentHash = msg.Hash
	m.statusBar.artist = msg.Artist
	m.statusBar.title = msg.Title
	source := msg.PlaylistSource
	if source == "" {
		source = "Queue"
	}
	m.statusBar.playlistSource = source
	m.playback.state = PlaybackPlaying
	m.playback.elapsedSecs = 0
	m.playback.totalSecs = 0
	return m, m.sessionPositionCmd()
}

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
		return m, trackInfoCmd(m.appCtx, track.Path), true
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
			artist:  m.statusBar.artist,
			title:   m.statusBar.title,
			elapsed: m.playback.elapsedSecs,
			total:   m.playback.totalSecs,
			state:   m.playback.state,
		}
		// Enrich from library index — the status bar fields may be empty
		// after a session restore, but the hash is always set.
		if m.playback.currentHash != "" {
			if t, ok := m.library.index.FindByHash(m.playback.currentHash); ok {
				if overlay.artist == "" {
					overlay.artist = t.Artist
				}
				if overlay.title == "" {
					overlay.title = t.Title
				}
				overlay.album = t.Album
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
				slog.Debug("player pause failed", "err", err)
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
		if cp, ok := m.player.(commandPlayer); ok {
			if err := cp.SeekRelative(-seekOffsetShort); err != nil {
				slog.Debug("seek failed", "err", err)
			}
			return m, m.sessionPositionCmd(), true
		}
		return m, nil, true
	case actionSeekForward:
		if cp, ok := m.player.(commandPlayer); ok {
			if err := cp.SeekRelative(seekOffsetShort); err != nil {
				slog.Debug("seek failed", "err", err)
			}
			return m, m.sessionPositionCmd(), true
		}
		return m, nil, true
	case actionSeekBackLong:
		if cp, ok := m.player.(commandPlayer); ok {
			if err := cp.SeekRelative(-seekOffsetLong); err != nil {
				slog.Debug("seek failed", "err", err)
			}
			return m, m.sessionPositionCmd(), true
		}
		return m, nil, true
	case actionSeekForwardLong:
		if cp, ok := m.player.(commandPlayer); ok {
			if err := cp.SeekRelative(seekOffsetLong); err != nil {
				slog.Debug("seek failed", "err", err)
			}
			return m, m.sessionPositionCmd(), true
		}
		return m, nil, true
	case actionVolumeUp:
		if cp, ok := m.player.(commandPlayer); ok {
			if err := cp.SetVolumeRelative(volumeIncrement); err != nil {
				slog.Debug("volume up failed", "err", err)
			}
			m.playback.volume = clampVolume(m.playback.volume + volumeIncrement)
			if m.sessionWriter != nil {
				if err := m.sessionWriter.WriteVolume(m.playback.volume); err != nil {
					slog.Debug("write volume failed", "err", err)
				}
			}
			m.statusBar.notification = newNotificationFor(fmt.Sprintf("Volume: %d%%", m.playback.volume), volumeFeedbackDuration)
		}
		return m, nil, true
	case actionVolumeDown:
		if cp, ok := m.player.(commandPlayer); ok {
			if err := cp.SetVolumeRelative(-volumeIncrement); err != nil {
				slog.Debug("volume down failed", "err", err)
			}
			m.playback.volume = clampVolume(m.playback.volume - volumeIncrement)
			if m.sessionWriter != nil {
				if err := m.sessionWriter.WriteVolume(m.playback.volume); err != nil {
					slog.Debug("write volume failed", "err", err)
				}
			}
			m.statusBar.notification = newNotificationFor(fmt.Sprintf("Volume: %d%%", m.playback.volume), volumeFeedbackDuration)
		}
		return m, nil, true
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
				slog.Debug("set repeat failed", "err", err)
			}
		}
		if m.sessionWriter != nil {
			if err := m.sessionWriter.WriteRepeatMode(m.playback.repeatMode.String()); err != nil {
				slog.Debug("write repeat mode failed", "err", err)
			}
		}
		m.statusBar.notification = newNotificationFor("Repeat: "+m.playback.repeatMode.DisplayString(), volumeFeedbackDuration)
		return m, nil, true
	case actionShuffleToggle:
		m.playback.shuffle = !m.playback.shuffle
		if rs, ok := m.player.(repeatShuffler); ok {
			if err := rs.SetShuffle(m.playback.shuffle); err != nil {
				slog.Debug("set shuffle failed", "err", err)
			}
		}
		if m.sessionWriter != nil {
			if err := m.sessionWriter.WriteShuffle(m.playback.shuffle); err != nil {
				slog.Debug("write shuffle failed", "err", err)
			}
		}
		label := "off"
		if m.playback.shuffle {
			label = "on"
		}
		m.statusBar.notification = newNotificationFor("Shuffle: "+label, volumeFeedbackDuration)
		return m, nil, true
	}
	return m, nil, false
}

// handleNowPlayingKeyPress handles key events while the now-playing overlay is
// active. Returns (model, cmd, true) when the key was consumed.
func (m Model) handleNowPlayingKeyPress(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	if m.overlay.nowPlaying == nil {
		return m, nil, false
	}
	switch msg.String() {
	case "ctrl+c":
		m.overlay.nowPlaying = nil
		return m, tea.Quit, true
	case "esc", "I":
		m.overlay.nowPlaying = nil
		return m, nil, true
	default:
		return m, nil, true
	}
}

// handlePreviewKeyPress handles key events while the preview overlay is active.
// Returns (model, cmd, true) when the key was consumed.
func (m Model) handlePreviewKeyPress(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	if m.overlay.preview == nil {
		return m, nil, false
	}
	switch msg.String() {
	case "ctrl+c":
		m = m.dismissPreview()
		return m, tea.Quit, true
	case "esc", "P":
		m = m.dismissPreview()
		return m, nil, true
	default:
		return m, nil, true
	}
}

// focusedTrackForPreview returns the track under the cursor in whichever view
// is active, or false when no track is focused.
func (m Model) focusedTrackForPreview() (library.Track, bool) {
	switch m.activeView {
	case viewLibrary:
		return m.library.view().FocusedTrack()
	case viewQueue:
		if m.queue.cursor >= 0 && m.queue.cursor < len(m.queue.entries) {
			e := m.queue.entries[m.queue.cursor]
			if e.Missing {
				return library.Track{}, false
			}
			return library.Track{
				Hash:     e.Hash,
				Path:     e.Path,
				Artist:   e.Artist,
				Title:    e.Title,
				Duration: float64(e.DurationMs) / 1000,
			}, true
		}
	case viewPlaylists:
		return m.playlists.focusedTrack()
	case viewArtists:
		return m.artists.focusedTrack()
	case viewAlbums:
		return m.albums.focusedTrack()
	case viewFavorites:
		return m.favorites.focusedTrack()
	}
	return library.Track{}, false
}

// startPreviewCmd returns a tea.Cmd that launches a PreviewPlayer, loads the
// given track, and returns a previewStartedMsg on success.
func (m Model) startPreviewCmd(track library.Track) tea.Cmd {
	ctx := m.appCtx
	runtimeDir := m.runtimeDir
	return func() tea.Msg {
		socketPath := filepath.Join(runtimeDir, "muse-preview.sock")
		pp := player.NewPreviewPlayer(ctx, socketPath)
		if err := pp.Start(); err != nil {
			return previewStartErrorMsg{err: err}
		}
		if err := pp.LoadAndPlay(track.Path); err != nil {
			_ = pp.Close()
			return previewStartErrorMsg{err: err}
		}
		return previewStartedMsg{
			player: pp,
			overlay: PreviewOverlay{
				artist: track.Artist,
				title:  track.Title,
				album:  track.Album,
			},
		}
	}
}

// dismissPreview stops the preview player, clears the overlay, and resumes
// main playback if it was playing before the preview started.
func (m Model) dismissPreview() Model {
	if m.overlay.previewPlayer != nil {
		_ = m.overlay.previewPlayer.Close()
	}
	m.overlay.preview = nil
	m.overlay.previewPlayer = nil
	if m.overlay.previewWasPlaying && m.player != nil {
		_ = m.player.Play()
		m.playback.state = PlaybackPlaying
	}
	m.overlay.previewWasPlaying = false
	return m
}
