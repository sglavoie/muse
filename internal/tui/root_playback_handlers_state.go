package tui

import (
	"fmt"
	"log/slog"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/player"
)

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

	// Radio mode: enqueue a new batch if enabled and guards pass.
	if !wasStopAfter {
		if cmd := m.radioMaybeEnqueueCmd(); cmd != nil {
			m.playback.queueExhausted = true
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
		if idx < 0 {
			idx = 0
		}
	}
	m.playback.currentIndex = idx
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
	// Radio mode: update seed and check low-water-mark.
	m.radio.seedHash = msg.Hash
	radioCmd := m.radioMaybeEnqueueCmd()

	return m, tea.Batch(m.sessionPositionCmd(), incrementCmd, radioCmd)
}
