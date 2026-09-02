package tui

import (
	"fmt"
	"log/slog"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/player"
	libtui "github.com/sglavoie/muse/internal/tui/library"
	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/text"
)

// handlePlaybackEndedMsg handles the end of the play queue, optionally
// wrapping around in repeat-all mode.
func (m Model) handlePlaybackEndedMsg() (Model, tea.Cmd) {
	wasStopAfter := m.playback.stopAfterCurrent
	m.playback.stopAfterCurrent = false

	// One snapshot before the branch cascade covers all four exits below.
	m = m.endCurrentListen()

	if !wasStopAfter && m.playback.repeatMode == playback.RepeatAll && len(m.queue.entries) > 0 && m.player != nil {
		// Prefer RestartQueueCycle so shuffle reshuffles into a fresh permutation
		// (and sequential restarts from index 0). Fall back to PlayFromIndex(0)
		// for lightweight players that do not implement the optional interface.
		restart := m.player.PlayFromIndex
		if r, ok := m.player.(queueCycleRestarter); ok {
			restart = func(int) error { return r.RestartQueueCycle() }
		}
		if err := restart(0); err != nil {
			slog.Warn("repeat-all playback failed", "err", err)
		} else {
			m.playback.state = playback.StatePlaying
			m = m.resetPlaybackProgress()
			m.missingTrackCount = 0
			return m, nil
		}
	}

	// If the queue was concurrently extended (e.g. a paste was processed
	// before this message arrived), resume playback instead of stopping.
	if !wasStopAfter && m.player != nil && m.playback.currentIndex >= 0 &&
		m.playback.currentIndex+1 < len(m.queue.entries) {
		if err := m.player.PlayFromIndex(m.playback.currentIndex + 1); err == nil {
			m.playback.state = playback.StatePlaying
			m = m.resetPlaybackProgress()
			return m, nil
		}
	}

	// Radio mode: enqueue a new batch if enabled and guards pass.
	if !wasStopAfter {
		if cmd := m.radioMaybeEnqueueCmd(); cmd != nil {
			m.radio.enqueuePending = true
			m.playback.queueExhausted = true
			m.playback.state = playback.StateIdle
			m = m.resetPlaybackProgress()
			m.statusBar.Artist = ""
			m.statusBar.Title = ""
			m.statusBar.PlaylistSource = ""
			m.playback.currentHash = ""
			m.playback.currentIndex = -1
			m.missingTrackCount = 0
			player.PlayQueueFinishedSound()
			return m, cmd
		}
	}

	// Mark queue as exhausted so a subsequent paste can auto-resume.
	if !wasStopAfter {
		m.playback.queueExhausted = true
	}

	m.playback.state = playback.StateIdle
	m = m.resetPlaybackProgress()
	m.statusBar.Artist = ""
	m.statusBar.Title = ""
	m.statusBar.PlaylistSource = ""
	m.playback.currentHash = ""
	m.playback.currentIndex = -1

	var msg string
	switch {
	case wasStopAfter:
		msg = "Stopped after track"
	case m.missingTrackCount > 0:
		msg = fmt.Sprintf("Queue finished (%d %s skipped — missing files)",
			m.missingTrackCount, text.Pluralize(m.missingTrackCount, "track", "tracks"))
	default:
		msg = "Queue finished"
	}
	m.missingTrackCount = 0
	m.statusBar.Notify(msg)
	player.PlayQueueFinishedSound()
	return m, nil
}

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
	m.overlay.previewPlayer = msg.player
	m.overlay.preview = &msg.overlay
	m.overlay.previewDurationFilled = false
	return m
}

// handlePreviewStartErrorMsg notifies the user and resumes main playback.
func (m Model) handlePreviewStartErrorMsg(msg previewStartErrorMsg) Model {
	m.statusBar.Notify("Preview error: " + msg.err.Error())
	player.PlaySound(player.SoundError)
	if m.overlay.previewWasPlaying && m.player != nil {
		if err := m.player.Play(); err != nil {
			slog.Warn("player resume failed", "err", err)
		} else {
			m.playback.state = playback.StatePlaying
		}
	}
	m.overlay.previewWasPlaying = false
	return m
}

// handleTrackChangedMsg updates the status bar and playback state when the
// player advances to a new track.
func (m Model) handleTrackChangedMsg(msg playback.TrackChangedMsg) (Model, tea.Cmd) {
	// End the previous listen above the empty-queue guard below: when the queue
	// was cleared mid-track the listen still happened, and returning early
	// would strand it armed.
	m = m.endCurrentListen()

	// Guard: discard stale TrackChangedMsg that arrives after the queue was
	// cleared (e.g. mpv's gapless prefetch fired a start-file event before
	// the clear was processed). Without this check the TUI would resurrect
	// playback state on an empty queue, showing a playing status bar with
	// no corresponding queue entry.
	if len(m.queue.entries) == 0 {
		return m, nil
	}

	// Capture the outgoing hash before the overwrite below: the notification
	// at the end of this handler must only fire on a genuine track change.
	prevHash := m.playback.currentHash

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
	if m.followNowPlaying && m.activeView == viewQueue {
		m.queue = m.queue.ScrollToNowPlaying(idx)
	}
	m.statusBar.Artist = msg.Artist
	m.statusBar.Title = msg.Title
	if t, ok := m.library.Index.FindByHash(msg.Hash); ok {
		m.statusBar.Album = t.Album
	} else {
		// Clear the previous track's album so it does not leak into the
		// status bar when the new hash isn't in the library index.
		m.statusBar.Album = ""
	}
	source := msg.PlaylistSource
	if source == "" {
		source = "Queue"
	}
	m.statusBar.PlaylistSource = source
	m.playback.state = playback.StatePlaying
	m = m.resetPlaybackProgress()
	m = m.armPendingListen(msg.Hash, msg.Artist)

	// Refresh the now-playing overlay with the new track's metadata.
	if m.overlay.nowPlaying != nil {
		m.overlay.nowPlaying.Artist = msg.Artist
		m.overlay.nowPlaying.Title = msg.Title
		m.overlay.nowPlaying.Album = ""
		m.overlay.nowPlaying.PlayCount = 0
		m.overlay.nowPlaying.Elapsed = 0
		m.overlay.nowPlaying.Total = 0
		m.overlay.nowPlaying.State = playback.StatePlaying
		m.overlay.nowPlaying.TagsLine = m.buildTagsLine(msg.Hash)
		if t, ok := m.library.Index.FindByHash(msg.Hash); ok {
			if m.overlay.nowPlaying.Artist == "" {
				m.overlay.nowPlaying.Artist = t.Artist
			}
			if m.overlay.nowPlaying.Title == "" {
				m.overlay.nowPlaying.Title = t.Title
			}
			m.overlay.nowPlaying.Album = t.Album
			m.overlay.nowPlaying.PlayCount = t.PlayCount
		}
	}

	// Radio mode: update seed and check low-water-mark.
	m.radio.seedHash = msg.Hash
	radioCmd := m.radioMaybeEnqueueCmd()
	if radioCmd != nil {
		m.radio.enqueuePending = true
	}

	// Pulse the now-playing row briefly so the user notices the transition.
	m.pulseUntil = time.Now().Add(pulseDuration)

	return m, tea.Batch(m.sessionPositionCmd(), radioCmd, pulseCmd(),
		m.trackNotifyCmd(msg, prevHash))
}

// trackNotifyCmd posts a desktop banner for a newly started track, or returns
// nil when notifications are off, no notifier is wired, or the hash did not
// actually change (mpv reconnect and session restore both re-emit the current
// track). A failing notifier — a missing alerter binary, most likely — is
// logged and otherwise ignored: it must never disturb playback.
func (m Model) trackNotifyCmd(msg playback.TrackChangedMsg, prevHash string) tea.Cmd {
	if !m.notifyOnTrackChange || m.notifier == nil || msg.Hash == "" || msg.Hash == prevHash {
		return nil
	}
	notifier := m.notifier
	title, artist, album := msg.Title, msg.Artist, m.statusBar.Album
	return func() tea.Msg {
		if err := notifier.NotifyTrack(title, artist, album); err != nil {
			slog.Warn("track notification failed", "err", err)
		}
		return nil
	}
}

// bumpSessionArtist records one more play for artist in the session
// artist-count map, allocating it lazily. No-op for empty strings.
func (m *Model) bumpSessionArtist(artist string) {
	if artist == "" {
		return
	}
	if m.sessionArtistCounts == nil {
		m.sessionArtistCounts = make(map[string]int)
	}
	m.sessionArtistCounts[artist]++
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
