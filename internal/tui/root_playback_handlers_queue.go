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
	// Discard everything a newer request has superseded. Two requests aimed at
	// the same queue occurrence — holding Enter on one row, or replaying it and
	// then selecting it again — produce results the row identity below cannot
	// tell apart, so without this the older one's failure would reset the state
	// the newer one's success had just established, in whichever order the two
	// goroutines happen to finish.
	if msg.Request != m.playback.playRequestSeq {
		return m
	}
	// The latest request has now been answered, so the current row is whatever
	// the player made of it rather than an unconfirmed optimistic guess. This
	// runs for a failure too: the reset below is itself a resolution. Enter on
	// the row is a genuine replay again from here.
	m.playback.pendingSelection = false
	if msg.Err == nil {
		// The track-event barrier is deliberately left standing. A successful
		// load says mpv accepted the selected row, not that it has finished
		// reporting the rows it started before it — the event reader syncs the
		// position between reading a start-file and dispatching it, so an
		// abandoned selection's report can still be in flight. Only that row's
		// own start-file retires the barrier, in handleTrackChangedMsg.
		return m
	}
	// A refused load will never produce a start-file, so nothing else would ever
	// lower the barrier. Drop it here whether or not the reset below runs.
	m.playback.lowerSelectionBarrier()
	// For the same reason the selection's start request is withdrawn: the row the
	// user asked for is not playing, so the Auto-DJ refill stays as suspended as a
	// deliberate stop left it, and the next report — a reconnect echo, say — finds
	// no request of its own to answer.
	m = m.radioNoteStartRefused()
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
