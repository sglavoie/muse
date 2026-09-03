package tui

import (
	"log/slog"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/player"
	"github.com/sglavoie/muse/internal/tui/playback"
)

// handlePlayerStatusMsg shows a transient player status notification.
func (m Model) handlePlayerStatusMsg(msg playback.PlayerStatusMsg) Model {
	m.statusBar.Notify(msg.Text)
	return m
}

// handleMissingTrackMsg marks a queue entry as missing, increments the missing
// track counter, and notifies the status bar.
func (m Model) handleMissingTrackMsg(msg playback.MissingTrackMsg) Model {
	m.queue.MarkMissing(msg.Path)
	m.statusBar.HandleMissingTrack(msg.Path)
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
func (m Model) handleMissingTrackSelectedMsg(msg playback.MissingTrackSelectedMsg) (Model, tea.Cmd) {
	idx := msg.Index
	if idx < 0 || idx >= len(m.queue.Entries) {
		m.statusBar.Notify("Cannot play — file is missing")
		return m, nil
	}
	entry := m.queue.Entries[idx]
	// Guard against a stale index (queue mutated between dispatch and Update).
	if msg.Path != "" && entry.Path != msg.Path {
		m.statusBar.Notify("Cannot play — file is missing")
		return m, nil
	}

	m.statusBar.Notify("Removed — file missing: " + entry.Display)
	player.PlaySound(player.SoundError)

	m.queue.Entries = append(m.queue.Entries[:idx], m.queue.Entries[idx+1:]...)
	for i := range m.queue.Entries {
		m.queue.Entries[i].Selected = false
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
		// The track was the currently-playing entry and its file is gone.
		// stopPlaybackIfCurrentMissing cannot detect this because
		// adjustPlaybackIndexAfterRemoval already set currentIndex to -1.
		// Reset playback state directly to avoid leaving the UI in a
		// stale PlaybackPlaying state with no valid current track.
		m = m.resetPlaybackState()
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
// handlePlayQueueIndexMsg playback call — PlayFromIndex or ReplayCurrent alike.
// Success produces no visible effect here because the player's start-file event
// will dispatch a TrackChangedMsg that updates the status bar; this handler
// exists to recover from failures without blocking the Update goroutine.
func (m Model) handlePlayFromIndexResultMsg(msg playback.PlayFromIndexResultMsg) Model {
	if msg.Err == nil {
		return m
	}
	// Discard the error if the user already moved on to a different track or
	// cleared the queue between dispatch and arrival. Resetting unconditionally
	// would clobber the now-correct playback state with a stale failure. A
	// replay is checked the same way even though it targets the row that was
	// already current: a second selection landing first makes this result just
	// as stale.
	if m.playback.currentHash != msg.Hash || m.playback.currentIndex != msg.Index {
		return m
	}
	m = m.resetPlaybackState()
	m.statusBar.Notify(playResultFailureText(msg))
	player.PlaySound(player.SoundError)
	return m
}

// playResultFailureText names the operation that failed, so a refused replay
// does not report itself as a failed jump to a track the user never selected.
func playResultFailureText(msg playback.PlayFromIndexResultMsg) string {
	if msg.Replay {
		return "Replay failed: " + msg.Err.Error()
	}
	return "Play failed: " + msg.Err.Error()
}
