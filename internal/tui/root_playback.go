package tui

import (
	"fmt"
	"path/filepath"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/player"
)

// handleTickMsg updates elapsed time for the main player, preview player, and
// now-playing overlay, and clears expired status bar notifications.
func (m Model) handleTickMsg() (Model, tea.Cmd) {
	if m.playbackState == PlaybackPlaying || m.playbackState == PlaybackPaused {
		if er, ok := m.player.(elapsedReader); ok {
			if elapsed, total, err := er.ElapsedPosition(); err == nil {
				m.elapsedSecs = elapsed
				m.totalSecs = total
			}
		}
	}
	if m.preview != nil && m.previewPlayer != nil {
		if elapsed, total, err := m.previewPlayer.ElapsedPosition(); err == nil {
			m.preview.elapsed = elapsed
			m.preview.total = total
		} else {
			// Preview track ended or mpv died — auto-dismiss.
			m = m.dismissPreview()
		}
	}
	if m.nowPlayingInfo != nil {
		m.nowPlayingInfo.elapsed = m.elapsedSecs
		m.nowPlayingInfo.total = m.totalSecs
		m.nowPlayingInfo.state = m.playbackState
	}
	if !m.statusBar.notification.expiresAt.IsZero() && time.Now().After(m.statusBar.notification.expiresAt) {
		m.statusBar.notification = transientNotification{}
	}
	if m.clearQueuePending && time.Now().After(m.clearQueuePendingAt.Add(2*time.Second)) {
		m.clearQueuePending = false
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
	if m.playbackState == PlaybackPlaying {
		cmd = m.sessionPositionCmd()
	}
	return m, tea.Batch(sessionTickCmd(), cmd)
}

// handlePlayerStatusMsg shows a transient player status notification.
func (m Model) handlePlayerStatusMsg(msg PlayerStatusMsg) Model {
	m.statusBar.notification = transientNotification{
		msg:       msg.Text,
		expiresAt: time.Now().Add(transientDuration),
	}
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
	if m.repeatMode == RepeatAll && len(m.queue.entries) > 0 && m.player != nil {
		if err := m.player.PlayFromIndex(0); err == nil {
			m.playbackState = PlaybackPlaying
			e := m.queue.entries[0]
			m.currentTrackHash = e.Hash
			m.statusBar.artist = e.Artist
			m.statusBar.title = e.Title
			m.statusBar.playlistSource = "Queue"
			m.elapsedSecs = 0
			m.totalSecs = 0
			return m
		}
	}
	m.playbackState = PlaybackIdle
	m.elapsedSecs = 0
	m.totalSecs = 0
	m.statusBar.artist = ""
	m.statusBar.title = ""
	m.statusBar.playlistSource = ""
	m.currentTrackHash = ""
	if m.repeatMode == RepeatOff {
		msg := "Queue finished"
		if m.missingTrackCount > 0 {
			msg = fmt.Sprintf("Queue finished (%d %s skipped — missing files)",
				m.missingTrackCount, pluralize(m.missingTrackCount, "track", "tracks"))
		}
		m.missingTrackCount = 0
		m.statusBar.notification = transientNotification{
			msg:       msg,
			expiresAt: time.Now().Add(transientDuration),
		}
		player.PlayQueueFinishedSound()
	}
	return m
}

// handlePreviewStartedMsg stores the preview player and overlay state.
func (m Model) handlePreviewStartedMsg(msg previewStartedMsg) Model {
	m.previewPlayer = msg.player
	m.preview = &msg.overlay
	return m
}

// handlePreviewStartErrorMsg notifies the user and resumes main playback.
func (m Model) handlePreviewStartErrorMsg(msg previewStartErrorMsg) Model {
	m.statusBar.notification = transientNotification{
		msg:       "Preview error: " + msg.err.Error(),
		expiresAt: time.Now().Add(transientDuration),
	}
	player.PlaySound(player.SoundError)
	// Resume main player if we paused it.
	if m.previewWasPlaying && m.player != nil {
		_ = m.player.Play()
		m.playbackState = PlaybackPlaying
	}
	m.previewWasPlaying = false
	return m
}

// handleTrackChangedMsg updates the status bar and playback state when the
// player advances to a new track.
func (m Model) handleTrackChangedMsg(msg TrackChangedMsg) (Model, tea.Cmd) {
	m.currentTrackHash = msg.Hash
	m.statusBar.artist = msg.Artist
	m.statusBar.title = msg.Title
	source := msg.PlaylistSource
	if source == "" {
		source = "Queue"
	}
	m.statusBar.playlistSource = source
	m.playbackState = PlaybackPlaying
	m.elapsedSecs = 0
	m.totalSecs = 0
	return m, m.sessionPositionCmd()
}

// handlePlaybackKeyAction handles playback-related key actions (play/pause,
// stop, next, previous, seek, volume, repeat, shuffle, preview, now-playing).
// It returns true in the third value when the action was handled.
func (m Model) handlePlaybackKeyAction(action action) (tea.Model, tea.Cmd, bool) {
	switch action {
	case actionTrackInfo:
		if m.trackInfo != nil {
			m.trackInfo = nil
			return m, nil, true
		}
		track, ok := m.focusedTrackForPreview()
		if !ok {
			m.statusBar.notification = transientNotification{
				msg:       "No track to inspect",
				expiresAt: time.Now().Add(transientDuration),
			}
			return m, nil, true
		}
		if track.Path == "" {
			for _, t := range m.library.index.Tracks() {
				if t.Hash == track.Hash {
					track = t
					break
				}
			}
		}
		if track.Path == "" {
			m.statusBar.notification = transientNotification{
				msg:       "Track path unknown",
				expiresAt: time.Now().Add(transientDuration),
			}
			return m, nil, true
		}
		return m, trackInfoCmd(track.Path), true
	case actionNowPlayingInfo:
		if m.nowPlayingInfo != nil {
			m.nowPlayingInfo = nil
			return m, nil, true
		}
		if m.playbackState == PlaybackStopped || m.playbackState == PlaybackIdle {
			m.statusBar.notification = transientNotification{
				msg:       "No track playing",
				expiresAt: time.Now().Add(transientDuration),
			}
			return m, nil, true
		}
		overlay := NowPlayingOverlay{
			artist:  m.statusBar.artist,
			title:   m.statusBar.title,
			elapsed: m.elapsedSecs,
			total:   m.totalSecs,
			state:   m.playbackState,
		}
		// Enrich from library index — the status bar fields may be empty
		// after a session restore, but the hash is always set.
		if m.currentTrackHash != "" {
			for _, t := range m.library.index.Tracks() {
				if t.Hash == m.currentTrackHash {
					if overlay.artist == "" {
						overlay.artist = t.Artist
					}
					if overlay.title == "" {
						overlay.title = t.Title
					}
					overlay.album = t.Album
					break
				}
			}
		}
		m.nowPlayingInfo = &overlay
		return m, nil, true
	case actionPreview:
		if m.preview != nil {
			m = m.dismissPreview()
			return m, nil, true
		}
		track, ok := m.focusedTrackForPreview()
		if !ok {
			m.statusBar.notification = transientNotification{
				msg:       "No track to preview",
				expiresAt: time.Now().Add(transientDuration),
			}
			return m, nil, true
		}
		m.previewWasPlaying = m.playbackState == PlaybackPlaying
		if m.previewWasPlaying && m.player != nil {
			_ = m.player.Pause()
			m.playbackState = PlaybackPaused
		}
		return m, m.startPreviewCmd(track), true
	case actionPlayPause:
		if m.player != nil {
			if m.playbackState == PlaybackPlaying {
				_ = m.player.Pause()
				m.playbackState = PlaybackPaused
				return m, m.sessionPositionCmd(), true
			} else if len(m.queue.entries) == 0 {
				return m, nil, true
			} else {
				_ = m.player.Play()
				m.playbackState = PlaybackPlaying
			}
		}
		return m, nil, true
	case actionStop:
		if cp, ok := m.player.(commandPlayer); ok {
			_ = cp.Stop()
			m.playbackState = PlaybackStopped
			m.elapsedSecs = 0
			m.totalSecs = 0
		}
		return m, nil, true
	case actionNext:
		if cp, ok := m.player.(commandPlayer); ok {
			_ = cp.Next()
		}
		return m, nil, true
	case actionPrevious:
		if cp, ok := m.player.(commandPlayer); ok {
			_ = cp.Previous()
		}
		return m, nil, true
	case actionSeekBack:
		if cp, ok := m.player.(commandPlayer); ok {
			_ = cp.SeekRelative(-5)
			return m, m.sessionPositionCmd(), true
		}
		return m, nil, true
	case actionSeekForward:
		if cp, ok := m.player.(commandPlayer); ok {
			_ = cp.SeekRelative(5)
			return m, m.sessionPositionCmd(), true
		}
		return m, nil, true
	case actionSeekBackLong:
		if cp, ok := m.player.(commandPlayer); ok {
			_ = cp.SeekRelative(-30)
			return m, m.sessionPositionCmd(), true
		}
		return m, nil, true
	case actionSeekForwardLong:
		if cp, ok := m.player.(commandPlayer); ok {
			_ = cp.SeekRelative(30)
			return m, m.sessionPositionCmd(), true
		}
		return m, nil, true
	case actionVolumeUp:
		if cp, ok := m.player.(commandPlayer); ok {
			_ = cp.SetVolumeRelative(5)
			m.volume = clampVolume(m.volume + 5)
			m.statusBar.notification = transientNotification{
				msg:       fmt.Sprintf("Volume: %d%%", m.volume),
				expiresAt: time.Now().Add(volumeFeedbackDuration),
			}
		}
		return m, nil, true
	case actionVolumeDown:
		if cp, ok := m.player.(commandPlayer); ok {
			_ = cp.SetVolumeRelative(-5)
			m.volume = clampVolume(m.volume - 5)
			m.statusBar.notification = transientNotification{
				msg:       fmt.Sprintf("Volume: %d%%", m.volume),
				expiresAt: time.Now().Add(volumeFeedbackDuration),
			}
		}
		return m, nil, true
	case actionRepeatCycle:
		switch m.repeatMode {
		case RepeatOff:
			m.repeatMode = RepeatAll
		case RepeatAll:
			m.repeatMode = RepeatOne
		case RepeatOne:
			m.repeatMode = RepeatOff
		}
		if rs, ok := m.player.(repeatShuffler); ok {
			_ = rs.SetRepeat(m.repeatMode.String())
		}
		if m.sessionWriter != nil {
			_ = m.sessionWriter.WriteRepeatMode(m.repeatMode.String())
		}
		var modeStr string
		switch m.repeatMode {
		case RepeatOff:
			modeStr = "off"
		case RepeatAll:
			modeStr = "all"
		case RepeatOne:
			modeStr = "one"
		}
		m.statusBar.notification = transientNotification{
			msg:       "Repeat: " + modeStr,
			expiresAt: time.Now().Add(volumeFeedbackDuration),
		}
		return m, nil, true
	case actionShuffleToggle:
		m.shuffle = !m.shuffle
		if rs, ok := m.player.(repeatShuffler); ok {
			_ = rs.SetShuffle(m.shuffle)
		}
		if m.sessionWriter != nil {
			_ = m.sessionWriter.WriteShuffle(m.shuffle)
		}
		label := "off"
		if m.shuffle {
			label = "on"
		}
		m.statusBar.notification = transientNotification{
			msg:       "Shuffle: " + label,
			expiresAt: time.Now().Add(volumeFeedbackDuration),
		}
		return m, nil, true
	}
	return m, nil, false
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
	if m.previewPlayer != nil {
		_ = m.previewPlayer.Close()
	}
	m.preview = nil
	m.previewPlayer = nil
	if m.previewWasPlaying && m.player != nil {
		_ = m.player.Play()
		m.playbackState = PlaybackPlaying
	}
	m.previewWasPlaying = false
	return m
}
