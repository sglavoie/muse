package tui

import (
	"log/slog"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/player"
)

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
