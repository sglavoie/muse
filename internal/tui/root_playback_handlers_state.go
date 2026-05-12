package tui

import (
	"fmt"
	"log/slog"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/player"
)

func (m Model) playCountIncrementCmd(hash string, elapsedSecs, totalSecs float64) tea.Cmd {
	if hash == "" || !shouldCountPlay(elapsedSecs, totalSecs) {
		return nil
	}

	now := time.Now().Unix()
	m.library.index.IncrementPlayCount(hash, now)
	if m.historyRecorder != nil {
		hr := m.historyRecorder
		elapsed := int(elapsedSecs)
		duration := totalSecs
		return func() tea.Msg {
			if err := hr.RecordPlay(hash, now, elapsed, duration); err != nil {
				slog.Warn("play count: RecordPlay failed", "hash", hash, "err", err)
				return nil
			}
			return playRecordedMsg{Hash: hash, PlayedAt: now}
		}
	}
	if m.playCountWriter != nil {
		pcw := m.playCountWriter
		return func() tea.Msg {
			if err := pcw.IncrementPlayCount(hash); err != nil {
				slog.Warn("play count: IncrementPlayCount failed", "hash", hash, "err", err)
			}
			return nil
		}
	}

	return nil
}

// handlePlaybackEndedMsg handles the end of the play queue, optionally
// wrapping around in repeat-all mode.
func (m Model) handlePlaybackEndedMsg() (Model, tea.Cmd) {
	wasStopAfter := m.playback.stopAfterCurrent
	m.playback.stopAfterCurrent = false

	if m.playback.currentHash != "" && shouldCountPlay(m.playback.peakElapsedSecs, m.playback.peakTotalSecs) {
		m.mixes.playsSinceRefresh++
	}

	incrementCmd := m.playCountIncrementCmd(
		m.playback.currentHash,
		m.playback.peakElapsedSecs,
		m.playback.peakTotalSecs,
	)

	if !wasStopAfter && m.playback.repeatMode == RepeatAll && len(m.queue.entries) > 0 && m.player != nil {
		if err := m.player.PlayFromIndex(0); err != nil {
			slog.Warn("repeat-all playback failed", "err", err)
		} else {
			m.playback.state = PlaybackPlaying
			m = m.resetPlaybackProgress()
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
			m = m.resetPlaybackProgress()
			return m, incrementCmd
		}
	}

	// Radio mode: enqueue a new batch if enabled and guards pass.
	if !wasStopAfter {
		if cmd := m.radioMaybeEnqueueCmd(); cmd != nil {
			m.radio.enqueuePending = true
			m.playback.queueExhausted = true
			m.playback.state = PlaybackIdle
			m = m.resetPlaybackProgress()
			m.statusBar.artist = ""
			m.statusBar.title = ""
			m.statusBar.playlistSource = ""
			m.playback.currentHash = ""
			m.playback.currentIndex = -1
			m.missingTrackCount = 0
			player.PlayQueueFinishedSound()
			return m, tea.Batch(incrementCmd, cmd)
		}
	}

	// Mark queue as exhausted so a subsequent paste can auto-resume.
	if !wasStopAfter {
		m.playback.queueExhausted = true
	}

	m.playback.state = PlaybackIdle
	m = m.resetPlaybackProgress()
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
	m = m.resetPlaybackProgress()
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
	// Guard: discard stale TrackChangedMsg that arrives after the queue was
	// cleared (e.g. mpv's gapless prefetch fired a start-file event before
	// the clear was processed). Without this check the TUI would resurrect
	// playback state on an empty queue, showing a playing status bar with
	// no corresponding queue entry.
	if len(m.queue.entries) == 0 {
		return m, nil
	}

	previousHash := m.playback.currentHash
	previousElapsed := m.playback.peakElapsedSecs
	previousTotal := m.playback.peakTotalSecs

	m.playback.currentHash = msg.Hash
	// Clamp index to queue bounds in case the queue shrank between the mpv
	// event dispatch and TUI processing (e.g. dequeue raced with gapless
	// transition).
	idx := msg.Index
	if idx < 0 || idx >= len(m.queue.entries) {
		idx = -1
		for i, e := range m.queue.entries {
			if e.Hash == msg.Hash {
				idx = i
				break
			}
		}
		// When the hash is not found in the queue, keep idx = -1 rather
		// than falling back to 0. This prevents currentIndex from pointing
		// at an unrelated track while still updating the status bar with
		// the metadata from the message.
	}
	m.playback.currentIndex = idx
	m.playback.queueExhausted = false
	m.statusBar.artist = msg.Artist
	m.statusBar.title = msg.Title
	if t, ok := m.library.index.FindByHash(msg.Hash); ok {
		m.statusBar.album = t.Album
	}
	source := msg.PlaylistSource
	if source == "" {
		source = "Queue"
	}
	m.statusBar.playlistSource = source
	m.playback.state = PlaybackPlaying
	m = m.resetPlaybackProgress()

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

	if previousHash != "" && shouldCountPlay(previousElapsed, previousTotal) {
		m.mixes.playsSinceRefresh++
	}
	incrementCmd := m.playCountIncrementCmd(previousHash, previousElapsed, previousTotal)
	// Radio mode: update seed and check low-water-mark.
	m.radio.seedHash = msg.Hash
	radioCmd := m.radioMaybeEnqueueCmd()
	if radioCmd != nil {
		m.radio.enqueuePending = true
	}

	return m, tea.Batch(m.sessionPositionCmd(), incrementCmd, radioCmd)
}

// handlePlayRecordedMsg prepends the just-played track to the in-memory
// library History mode so it updates in realtime without a full DB reload.
func (m Model) handlePlayRecordedMsg(msg playRecordedMsg) Model {
	// Only update when History mode is active and has been loaded.
	if m.library.mode != LibraryModeHistory || len(m.library.sourceTracks) == 0 {
		return m
	}

	t, ok := m.library.index.FindByHash(msg.Hash)
	if !ok {
		return m
	}
	t.LastPlayedAt = msg.PlayedAt

	// Prepend and cap at 500 entries.
	const maxHistory = 500
	tracks := make([]library.Track, 0, min(len(m.library.sourceTracks)+1, maxHistory))
	tracks = append(tracks, t)
	limit := len(m.library.sourceTracks)
	if limit > maxHistory-1 {
		limit = maxHistory - 1
	}
	tracks = append(tracks, m.library.sourceTracks[:limit]...)
	m.library.sourceTracks = tracks

	m.library = m.library.recomputeSortedTracks()
	if m.library.filter.Text != "" {
		m.library = m.library.rebuildFilterCache()
	}
	return m
}
