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

// shouldCountPlay returns true when the user has listened to enough of the
// track to count it as a play: at least 50% of the duration or 30 seconds,
// whichever comes first.
func shouldCountPlay(elapsedSecs, totalSecs float64) bool {
	if totalSecs <= 0 || elapsedSecs <= 0 {
		return false
	}
	return elapsedSecs >= 30 || elapsedSecs >= totalSecs*0.5
}

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
	var previewDurationCmd tea.Cmd
	if m.overlay.preview != nil && m.overlay.previewPlayer != nil {
		if elapsed, total, err := m.overlay.previewPlayer.ElapsedPosition(); err == nil {
			m.overlay.preview.elapsed = elapsed
			m.overlay.preview.total = total
			if !m.overlay.previewDurationFilled && total > 0 {
				hash := m.overlay.preview.hash
				if t, ok := m.library.index.FindByHash(hash); ok && t.Duration == 0 {
					m.library.index.UpdateDuration(hash, total)
					m = m.syncRecentlyAddedFromLibrary()
					newDurationMs := int64(total * 1000)
					for i := range m.queue.entries {
						if m.queue.entries[i].Hash == hash {
							m.queue.entries[i].DurationMs = newDurationMs
						}
					}
					if dw := m.durationWriter; dw != nil {
						previewDurationCmd = func() tea.Msg {
							_ = dw.UpdateTrackDuration(hash, newDurationMs)
							return nil
						}
					}
				}
				m.overlay.previewDurationFilled = true
			}
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
	if m.undo.bulkRemovePending && time.Now().After(m.undo.bulkRemovePendingAt.Add(confirmationWindow)) {
		m.undo.bulkRemovePending = false
	}
	return m, tea.Batch(tickCmd(), previewDurationCmd)
}

// handleSyncPositionMsg persists the current playback position via the player.
func (m Model) handleSyncPositionMsg() (Model, tea.Cmd) {
	if ps, ok := m.player.(positionSyncer); ok {
		_ = ps.SyncPosition() // periodic sync, next tick will retry
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
func (m Model) handlePlaybackEndedMsg() (Model, tea.Cmd) {
	var incrementCmd tea.Cmd
	if m.playback.currentHash != "" &&
		shouldCountPlay(m.playback.elapsedSecs, m.playback.totalSecs) {
		h := m.playback.currentHash
		now := time.Now().Unix()
		m.library.index.IncrementPlayCount(h, now)
		if m.historyRecorder != nil {
			hr := m.historyRecorder
			elapsed := int(m.playback.elapsedSecs)
			incrementCmd = func() tea.Msg {
				_ = hr.RecordPlay(h, now, elapsed)
				return nil
			}
		} else if m.playCountWriter != nil {
			pcw := m.playCountWriter
			incrementCmd = func() tea.Msg {
				_ = pcw.IncrementPlayCount(h)
				return nil
			}
		}
	}
	if m.playback.repeatMode == RepeatAll && len(m.queue.entries) > 0 && m.player != nil {
		if err := m.player.PlayFromIndex(0); err != nil {
			slog.Warn("repeat-all playback failed", "err", err)
		} else {
			// PlayFromIndex(0) may skip missing entries at the start of
			// the queue and actually begin playing a later track. Do not
			// speculatively set currentHash/artist/title from entries[0];
			// the incoming TrackChangedMsg will carry the correct metadata
			// for whichever track mpv actually started.
			m.playback.state = PlaybackPlaying
			m.playback.elapsedSecs = 0
			m.playback.totalSecs = 0
			// Reset the missing counter at the start of a new repeat-all
			// cycle so it does not accumulate across loops and produce a
			// stale count if the user later switches to repeat-off.
			m.missingTrackCount = 0
			return m, incrementCmd
		}
	}
	m.playback.state = PlaybackIdle
	m.playback.elapsedSecs = 0
	m.playback.totalSecs = 0
	m.statusBar.artist = ""
	m.statusBar.title = ""
	m.statusBar.playlistSource = ""
	m.playback.currentHash = ""
	m.playback.currentIndex = -1
	msg := "Queue finished"
	if m.missingTrackCount > 0 {
		msg = fmt.Sprintf("Queue finished (%d %s skipped — missing files)",
			m.missingTrackCount, pluralize(m.missingTrackCount, "track", "tracks"))
	}
	m.missingTrackCount = 0
	m.statusBar.notification = newNotification(msg)
	player.PlayQueueFinishedSound()
	return m, incrementCmd
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
	m.overlay.previewDurationFilled = false
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
		} else {
			m.playback.state = PlaybackPlaying
		}
	}
	m.overlay.previewWasPlaying = false
	return m
}

// handleTrackChangedMsg updates the status bar and playback state when the
// player advances to a new track.
func (m Model) handleTrackChangedMsg(msg TrackChangedMsg) (Model, tea.Cmd) {
	previousHash := m.playback.currentHash
	previousElapsed := m.playback.elapsedSecs // capture before zero
	previousTotal := m.playback.totalSecs

	m.playback.currentHash = msg.Hash
	m.playback.currentIndex = msg.Index
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

	var incrementCmd tea.Cmd
	if previousHash != "" && shouldCountPlay(previousElapsed, previousTotal) {
		now := time.Now().Unix()
		m.library.index.IncrementPlayCount(previousHash, now)
		if m.historyRecorder != nil {
			hr := m.historyRecorder
			h := previousHash
			elapsed := int(previousElapsed)
			incrementCmd = func() tea.Msg {
				_ = hr.RecordPlay(h, now, elapsed)
				return nil
			}
		} else if m.playCountWriter != nil {
			pcw := m.playCountWriter
			h := previousHash
			incrementCmd = func() tea.Msg {
				_ = pcw.IncrementPlayCount(h)
				return nil
			}
		}
	}
	return m, tea.Batch(m.sessionPositionCmd(), incrementCmd)
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
		return m, trackInfoCmd(m.appCtx, track.Path, track.PlayCount), true
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
			if err := cp.SeekRelative(-m.cfg.SeekOffsetShort); err != nil {
				slog.Debug("seek failed", "err", err)
			}
			return m, m.sessionPositionCmd(), true
		}
		return m, nil, true
	case actionSeekForward:
		if cp, ok := m.player.(commandPlayer); ok {
			if err := cp.SeekRelative(m.cfg.SeekOffsetShort); err != nil {
				slog.Debug("seek failed", "err", err)
			}
			return m, m.sessionPositionCmd(), true
		}
		return m, nil, true
	case actionSeekBackLong:
		if cp, ok := m.player.(commandPlayer); ok {
			if err := cp.SeekRelative(-m.cfg.SeekOffsetLong); err != nil {
				slog.Debug("seek failed", "err", err)
			}
			return m, m.sessionPositionCmd(), true
		}
		return m, nil, true
	case actionSeekForwardLong:
		if cp, ok := m.player.(commandPlayer); ok {
			if err := cp.SeekRelative(m.cfg.SeekOffsetLong); err != nil {
				slog.Debug("seek failed", "err", err)
			}
			return m, m.sessionPositionCmd(), true
		}
		return m, nil, true
	case actionVolumeUp:
		if cp, ok := m.player.(commandPlayer); ok {
			if err := cp.SetVolumeRelative(float64(m.cfg.VolumeIncrement)); err != nil {
				slog.Debug("volume up failed", "err", err)
			} else {
				m.playback.volume = clampVolume(m.playback.volume + m.cfg.VolumeIncrement)
				if m.sessionWriter != nil {
					if err := m.sessionWriter.WriteVolume(m.playback.volume); err != nil {
						slog.Debug("write volume failed", "err", err)
					}
				}
			}
			m.statusBar.notification = newNotificationFor(fmt.Sprintf("Volume: %d%%", m.playback.volume), time.Duration(m.cfg.VolumeFeedbackDurationSecs)*time.Second)
		}
		return m, nil, true
	case actionVolumeDown:
		if cp, ok := m.player.(commandPlayer); ok {
			if err := cp.SetVolumeRelative(-float64(m.cfg.VolumeIncrement)); err != nil {
				slog.Debug("volume down failed", "err", err)
			} else {
				m.playback.volume = clampVolume(m.playback.volume - m.cfg.VolumeIncrement)
				if m.sessionWriter != nil {
					if err := m.sessionWriter.WriteVolume(m.playback.volume); err != nil {
						slog.Debug("write volume failed", "err", err)
					}
				}
			}
			m.statusBar.notification = newNotificationFor(fmt.Sprintf("Volume: %d%%", m.playback.volume), time.Duration(m.cfg.VolumeFeedbackDurationSecs)*time.Second)
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
		m.statusBar.notification = newNotificationFor("Repeat: "+m.playback.repeatMode.DisplayString(), time.Duration(m.cfg.VolumeFeedbackDurationSecs)*time.Second)
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
		m.statusBar.notification = newNotificationFor("Shuffle: "+label, time.Duration(m.cfg.VolumeFeedbackDurationSecs)*time.Second)
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
	case "space":
		if m.overlay.previewPlayer != nil {
			if err := m.overlay.previewPlayer.TogglePause(); err != nil {
				slog.Debug("preview pause toggle failed", "err", err)
				m.statusBar.notification = newNotification("Preview pause failed: " + err.Error())
			}
		}
		return m, nil, true
	case "left":
		if m.overlay.previewPlayer != nil {
			if err := m.overlay.previewPlayer.SeekRelative(-m.cfg.SeekOffsetShort); err != nil {
				slog.Debug("preview seek failed", "offset", -m.cfg.SeekOffsetShort, "err", err)
			}
		}
		return m, nil, true
	case "right":
		if m.overlay.previewPlayer != nil {
			if err := m.overlay.previewPlayer.SeekRelative(m.cfg.SeekOffsetShort); err != nil {
				slog.Debug("preview seek failed", "offset", m.cfg.SeekOffsetShort, "err", err)
			}
		}
		return m, nil, true
	case "shift+left":
		if m.overlay.previewPlayer != nil {
			if err := m.overlay.previewPlayer.SeekRelative(-m.cfg.SeekOffsetLong); err != nil {
				slog.Debug("preview seek failed", "offset", -m.cfg.SeekOffsetLong, "err", err)
			}
		}
		return m, nil, true
	case "shift+right":
		if m.overlay.previewPlayer != nil {
			if err := m.overlay.previewPlayer.SeekRelative(m.cfg.SeekOffsetLong); err != nil {
				slog.Debug("preview seek failed", "offset", m.cfg.SeekOffsetLong, "err", err)
			}
		}
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
		filtered := m.queue.filteredEntries()
		if m.queue.cursor >= 0 && m.queue.cursor < len(filtered) {
			e := filtered[m.queue.cursor]
			if e.Missing {
				return library.Track{}, false
			}
			track := library.Track{
				Hash:     e.Hash,
				Path:     e.Path,
				Artist:   e.Artist,
				Title:    e.Title,
				Album:    e.Album,
				Duration: float64(e.DurationMs) / 1000,
			}
			if t, ok := m.library.index.FindByHash(e.Hash); ok {
				track.PlayCount = t.PlayCount
			}
			return track, true
		}
	case viewPlaylists:
		track, ok := m.playlists.focusedTrack()
		if ok && track.Hash != "" {
			if t, found := m.library.index.FindByHash(track.Hash); found {
				track.PlayCount = t.PlayCount
			}
		}
		return track, ok
	case viewArtists:
		return m.artists.focusedTrack()
	case viewAlbums:
		return m.albums.focusedTrack()
	case viewFavorites:
		return m.favorites.focusedTrack()
	case viewGenres:
		return m.genres.focusedTrack()
	case viewRecentlyAdded:
		return m.recentlyAdded.view().FocusedTrack()
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
			_ = pp.Close() // preview cleanup, player is being dismissed
			return previewStartErrorMsg{err: err}
		}
		return previewStartedMsg{
			player: pp,
			overlay: PreviewOverlay{
				hash:      track.Hash,
				artist:    track.Artist,
				title:     track.Title,
				album:     track.Album,
				playCount: track.PlayCount,
			},
		}
	}
}

// dismissPreview stops the preview player, clears the overlay, and resumes
// main playback if it was playing before the preview started.
func (m Model) dismissPreview() Model {
	if m.overlay.previewPlayer != nil {
		_ = m.overlay.previewPlayer.Close() // preview cleanup before new playback
	}
	m.overlay.preview = nil
	m.overlay.previewPlayer = nil
	m.overlay.previewDurationFilled = false
	if m.overlay.previewWasPlaying && m.player != nil {
		if err := m.player.Play(); err != nil {
			slog.Debug("player resume after preview failed", "err", err)
		} else {
			m.playback.state = PlaybackPlaying
		}
	}
	m.overlay.previewWasPlaying = false
	return m
}
