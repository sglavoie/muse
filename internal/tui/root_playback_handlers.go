package tui

import (
	"fmt"
	"log/slog"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/player"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	gentui "github.com/sglavoie/muse/internal/tui/genres"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
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
	var mainDurationCmd tea.Cmd
	if m.playback.state == PlaybackPlaying || m.playback.state == PlaybackPaused {
		if er, ok := m.player.(elapsedReader); ok {
			if elapsed, total, err := er.ElapsedPosition(); err == nil {
				m.playback.elapsedSecs = elapsed
				m.playback.totalSecs = total
				if elapsed > m.playback.peakElapsedSecs {
					m.playback.peakElapsedSecs = elapsed
					m.playback.peakTotalSecs = total
				}
			}
		}
		if !m.playback.durationCorrected && m.playback.totalSecs > 0 && m.playback.currentHash != "" {
			m, mainDurationCmd = m.correctStoredDuration(m.playback.currentHash, m.playback.totalSecs)
			m.playback.durationCorrected = true
		}
	}

	var previewDurationCmd tea.Cmd
	if m.overlay.preview != nil && m.overlay.previewPlayer != nil {
		if elapsed, total, err := m.overlay.previewPlayer.ElapsedPosition(); err == nil {
			m.overlay.preview.elapsed = elapsed
			m.overlay.preview.total = total
			if !m.overlay.previewDurationFilled && total > 0 {
				m, previewDurationCmd = m.correctStoredDuration(m.overlay.preview.hash, total)
				m.overlay.previewDurationFilled = true
			}
		} else {
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
	return m, tea.Batch(tickCmd(), previewDurationCmd, mainDurationCmd)
}

// correctStoredDuration compares the mpv-reported total with the stored library
// duration and, if they differ by more than 2 seconds (or the stored value is
// zero), updates the library index, queue entries, and returns a tea.Cmd that
// persists the correction to the database.
func (m Model) correctStoredDuration(hash string, mpvTotal float64) (Model, tea.Cmd) {
	const threshold = 2.0
	t, ok := m.library.index.FindByHash(hash)
	if !ok || (t.Duration == 0 && mpvTotal <= 0) {
		return m, nil
	}
	diff := t.Duration - mpvTotal
	if diff < 0 {
		diff = -diff
	}
	if t.Duration != 0 && diff <= threshold {
		return m, nil
	}

	m.library.index.UpdateDuration(hash, mpvTotal)
	if m.library.sortState.Field != SortDefault {
		m.library = m.library.recomputeSortedTracks()
	}
	if m.library.filter.Text != "" {
		m.library = m.library.rebuildFilterCache()
	}
	m = m.syncRecentlyAddedFromLibrary()
	newDurationMs := int64(mpvTotal * 1000)
	for i := range m.queue.entries {
		if m.queue.entries[i].Hash == hash {
			m.queue.entries[i].DurationMs = newDurationMs
		}
	}
	for i := range m.history.sourceTracks {
		if m.history.sourceTracks[i].Hash == hash {
			m.history.sourceTracks[i].Duration = mpvTotal
		}
	}
	if m.history.sortState.Field != SortDefault {
		m.history = m.history.recomputeSortedTracks()
	}
	if m.history.filter.Text != "" {
		m.history = m.history.rebuildFilterCache()
	}
	if m.playlists.level == pltui.TrackLevel {
		for i := range m.playlists.tracks {
			if m.playlists.tracks[i].Hash == hash {
				m.playlists.tracks[i].DurationMs = newDurationMs
			}
		}
	}
	if m.favorites.level == favtui.ListLevel && m.favorites.category == favtui.CategoryTracks {
		for i := range m.favorites.tracks {
			if m.favorites.tracks[i].Hash == hash {
				m.favorites.tracks[i].Duration = mpvTotal
			}
		}
	}
	if m.favorites.level == favtui.TrackLevel {
		for i := range m.favorites.drilldownTracks {
			if m.favorites.drilldownTracks[i].Hash == hash {
				m.favorites.drilldownTracks[i].Duration = mpvTotal
			}
		}
	}
	if m.tagsView.level == tagtui.TrackLevel {
		for i := range m.tagsView.tracks {
			if m.tagsView.tracks[i].Hash == hash {
				m.tagsView.tracks[i].DurationMs = newDurationMs
			}
		}
	}
	if m.artists.level == arttui.TrackLevel {
		for i := range m.artists.tracks {
			if m.artists.tracks[i].Hash == hash {
				m.artists.tracks[i].Duration = mpvTotal
			}
		}
	}
	if m.albums.level == albtui.TrackLevel {
		for i := range m.albums.tracks {
			if m.albums.tracks[i].Hash == hash {
				m.albums.tracks[i].Duration = mpvTotal
			}
		}
	}
	if m.genres.level == gentui.TrackLevel {
		for i := range m.genres.tracks {
			if m.genres.tracks[i].Hash == hash {
				m.genres.tracks[i].Duration = mpvTotal
			}
		}
	}

	var cmd tea.Cmd
	if dw := m.durationWriter; dw != nil {
		cmd = func() tea.Msg {
			_ = dw.UpdateTrackDuration(hash, newDurationMs)
			return nil
		}
	}
	return m, cmd
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
	m.statusBar.notify(msg.Text)
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

// handleMissingTrackSelectedMsg handles an explicit Enter on a track whose
// file is gone (detected by the pre-flight stat in actionSelect or already
// flagged .Missing by the watcher). It shows a warning, splices the entry
// out of the in-memory queue, syncs the player queue, and returns a Cmd
// that hard-deletes the row from the database (no undo slot — restoring a
// reference to a file that disappeared off disk would only re-create the
// same grayed-out entry).
func (m Model) handleMissingTrackSelectedMsg(msg MissingTrackSelectedMsg) (Model, tea.Cmd) {
	idx := msg.Index
	if idx < 0 || idx >= len(m.queue.entries) {
		m.statusBar.notify("Cannot play — file is missing")
		return m, nil
	}
	entry := m.queue.entries[idx]
	// Guard against a stale index (queue mutated between dispatch and Update).
	if msg.Path != "" && entry.Path != msg.Path {
		m.statusBar.notify("Cannot play — file is missing")
		return m, nil
	}

	m.statusBar.notify("Removed — file missing: " + entry.Display)
	player.PlaySound(player.SoundError)

	m.queue.entries = append(m.queue.entries[:idx], m.queue.entries[idx+1:]...)
	for i := range m.queue.entries {
		m.queue.entries[i].Selected = false
	}
	m = exitVisualMode(m)
	m = m.preserveQueueSort()
	m.clampQueueCursorAndScroll()
	if remover, ok := m.player.(playerQueueRemover); ok {
		_ = remover.RemoveFromQueue(idx) // best-effort player-side sync
	}
	wasCurrent := idx == m.playback.currentIndex
	m.adjustPlaybackIndexAfterRemoval(idx)
	if wasCurrent {
		m.playback.currentHash = ""
		m.statusBar.artist = ""
		m.statusBar.title = ""
		m.statusBar.playlistSource = ""
		m = m.stopPlaybackIfCurrentMissing()
	}

	id := entry.ID
	undoer := m.undoer
	if undoer == nil || id == 0 {
		return m, nil
	}
	return m, func() tea.Msg {
		if err := undoer.DequeueTrackSoft(id); err != nil {
			slog.Warn("remove missing: soft-delete failed", "id", id, "err", err)
			return nil
		}
		if err := undoer.PurgeDeletedQueueEntries(time.Now().Unix() + 1); err != nil {
			slog.Warn("remove missing: purge failed", "id", id, "err", err)
		}
		return nil
	}
}

// handlePlayFromIndexResultMsg surfaces the outcome of an asynchronous
// handlePlayQueueIndexMsg playback call. Success produces no visible
// effect here because the player's start-file event will dispatch a
// TrackChangedMsg that updates the status bar; this handler exists to
// recover from failures without blocking the Update goroutine.
func (m Model) handlePlayFromIndexResultMsg(msg playFromIndexResultMsg) Model {
	if msg.Err == nil {
		return m
	}
	m = m.resetPlaybackState()
	m.statusBar.notify("Play failed: " + msg.Err.Error())
	player.PlaySound(player.SoundError)
	return m
}

// handlePlaybackEndedMsg handles the end of the play queue, optionally
// wrapping around in repeat-all mode.
func (m Model) handlePlaybackEndedMsg() (Model, tea.Cmd) {
	wasStopAfter := m.playback.stopAfterCurrent
	m.playback.stopAfterCurrent = false

	var incrementCmd tea.Cmd
	if m.playback.currentHash != "" &&
		shouldCountPlay(m.playback.peakElapsedSecs, m.playback.peakTotalSecs) {
		h := m.playback.currentHash
		now := time.Now().Unix()
		m.library.index.IncrementPlayCount(h, now)
		if m.historyRecorder != nil {
			hr := m.historyRecorder
			elapsed := int(m.playback.peakElapsedSecs)
			incrementCmd = func() tea.Msg {
				if err := hr.RecordPlay(h, now, elapsed); err != nil {
					slog.Warn("play count: RecordPlay failed", "hash", h, "err", err)
				}
				return nil
			}
		} else if m.playCountWriter != nil {
			pcw := m.playCountWriter
			incrementCmd = func() tea.Msg {
				if err := pcw.IncrementPlayCount(h); err != nil {
					slog.Warn("play count: IncrementPlayCount failed", "hash", h, "err", err)
				}
				return nil
			}
		}
	}

	if !wasStopAfter && m.playback.repeatMode == RepeatAll && len(m.queue.entries) > 0 && m.player != nil {
		if err := m.player.PlayFromIndex(0); err != nil {
			slog.Warn("repeat-all playback failed", "err", err)
		} else {
			m.playback.state = PlaybackPlaying
			m.playback.elapsedSecs = 0
			m.playback.totalSecs = 0
			m.playback.peakElapsedSecs = 0
			m.playback.peakTotalSecs = 0
			m.playback.durationCorrected = false
			m.missingTrackCount = 0
			return m, incrementCmd
		}
	}

	// If the queue was concurrently extended (e.g. a paste was processed
	// before this message arrived), resume playback instead of stopping.
	if !wasStopAfter && m.player != nil && m.playback.currentIndex >= 0 &&
		m.playback.currentIndex+1 < len(m.queue.entries) {
		if err := m.player.PlayFromIndex(m.playback.currentIndex + 1); err == nil {
			m.playback.state = PlaybackPlaying
			m.playback.elapsedSecs = 0
			m.playback.totalSecs = 0
			m.playback.peakElapsedSecs = 0
			m.playback.peakTotalSecs = 0
			m.playback.durationCorrected = false
			return m, incrementCmd
		}
	}

	// Mark queue as exhausted so a subsequent paste can auto-resume.
	if !wasStopAfter {
		m.playback.queueExhausted = true
	}

	m.playback.state = PlaybackIdle
	m.playback.elapsedSecs = 0
	m.playback.totalSecs = 0
	m.playback.peakElapsedSecs = 0
	m.playback.peakTotalSecs = 0
	m.playback.durationCorrected = false
	m.statusBar.artist = ""
	m.statusBar.title = ""
	m.statusBar.playlistSource = ""
	m.playback.currentHash = ""
	m.playback.currentIndex = -1

	var msg string
	switch {
	case wasStopAfter:
		msg = "Stopped after track"
	case m.missingTrackCount > 0:
		msg = fmt.Sprintf("Queue finished (%d %s skipped — missing files)",
			m.missingTrackCount, pluralize(m.missingTrackCount, "track", "tracks"))
	default:
		msg = "Queue finished"
	}
	m.missingTrackCount = 0
	m.statusBar.notify(msg)
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
	m.playback.peakElapsedSecs = 0
	m.playback.peakTotalSecs = 0
	m.statusBar.notify("mpv disconnected, restarting…")
	return m
}

// handlePauseChangedMsg syncs the TUI playback state with mpv's actual pause
// property. Guards prevent redundant transitions (e.g. user presses space ->
// TUI already updated -> mpv confirms via event -> no-op).
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
	m.statusBar.notify("Preview error: " + msg.err.Error())
	player.PlaySound(player.SoundError)
	if m.overlay.previewWasPlaying && m.player != nil {
		if err := m.player.Play(); err != nil {
			slog.Warn("player resume failed", "err", err)
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
	previousElapsed := m.playback.peakElapsedSecs
	previousTotal := m.playback.peakTotalSecs

	m.playback.currentHash = msg.Hash
	m.playback.currentIndex = msg.Index
	m.playback.queueExhausted = false
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
	m.playback.peakElapsedSecs = 0
	m.playback.peakTotalSecs = 0
	m.playback.durationCorrected = false

	// Refresh the now-playing overlay with the new track's metadata.
	if m.overlay.nowPlaying != nil {
		m.overlay.nowPlaying.artist = msg.Artist
		m.overlay.nowPlaying.title = msg.Title
		m.overlay.nowPlaying.album = ""
		m.overlay.nowPlaying.playCount = 0
		m.overlay.nowPlaying.elapsed = 0
		m.overlay.nowPlaying.total = 0
		m.overlay.nowPlaying.state = PlaybackPlaying
		m.overlay.nowPlaying.tagsLine = m.buildTagsLine(msg.Hash)
		if t, ok := m.library.index.FindByHash(msg.Hash); ok {
			if m.overlay.nowPlaying.artist == "" {
				m.overlay.nowPlaying.artist = t.Artist
			}
			if m.overlay.nowPlaying.title == "" {
				m.overlay.nowPlaying.title = t.Title
			}
			m.overlay.nowPlaying.album = t.Album
			m.overlay.nowPlaying.playCount = t.PlayCount
		}
	}

	var incrementCmd tea.Cmd
	if previousHash != "" && shouldCountPlay(previousElapsed, previousTotal) {
		now := time.Now().Unix()
		m.library.index.IncrementPlayCount(previousHash, now)
		if m.historyRecorder != nil {
			hr := m.historyRecorder
			h := previousHash
			elapsed := int(previousElapsed)
			incrementCmd = func() tea.Msg {
				if err := hr.RecordPlay(h, now, elapsed); err != nil {
					slog.Warn("play count: RecordPlay failed", "hash", h, "err", err)
				}
				return nil
			}
		} else if m.playCountWriter != nil {
			pcw := m.playCountWriter
			h := previousHash
			incrementCmd = func() tea.Msg {
				if err := pcw.IncrementPlayCount(h); err != nil {
					slog.Warn("play count: IncrementPlayCount failed", "hash", h, "err", err)
				}
				return nil
			}
		}
	}
	return m, tea.Batch(m.sessionPositionCmd(), incrementCmd)
}
