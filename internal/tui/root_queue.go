package tui

import (
	"fmt"
	"log/slog"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/player"
	libtui "github.com/sglavoie/muse/internal/tui/library"
)

// clampCursorAndScroll adjusts the queue cursor and scroll offset so they
// remain within bounds after entries have been removed.
func (m *Model) clampQueueCursorAndScroll() {
	if m.queue.cursor >= len(m.queue.entries) && m.queue.cursor > 0 {
		m.queue.cursor = len(m.queue.entries) - 1
	}
	if n := len(m.queue.entries); n > 0 {
		if m.queue.scrollOffset > n-1 {
			m.queue.scrollOffset = n - 1
		}
	} else {
		m.queue.scrollOffset = 0
	}
}

// resetPlaybackState stops the player and clears all playback-related fields
// in the model and status bar.
func (m Model) resetPlaybackState() Model {
	if cp, ok := m.player.(commandPlayer); ok {
		_ = cp.Stop()
	}
	m.playbackState = PlaybackIdle
	m.elapsedSecs = 0
	m.totalSecs = 0
	m.statusBar.artist = ""
	m.statusBar.title = ""
	m.statusBar.playlistSource = ""
	m.currentTrackHash = ""
	m.missingTrackCount = 0
	return m
}

// handleTrackEnqueuedMsg adds a newly enqueued track to the queue, sets up an
// undo slot, and triggers playback if the queue was empty.
func (m Model) handleTrackEnqueuedMsg(msg libtui.TrackEnqueuedMsg) Model {
	if msg.Err != nil {
		m.statusBar.notification = transientNotification{
			msg:       "Failed to enqueue track: " + msg.Err.Error(),
			expiresAt: time.Now().Add(transientDuration),
		}
		player.PlaySound(player.SoundError)
		return m
	}
	display := msg.Track.Artist + " – " + msg.Track.Title
	if msg.Track.Artist == "" && msg.Track.Title == "" {
		display = msg.Track.Path
	} else if msg.Track.Artist == "" {
		display = msg.Track.Title
	} else if msg.Track.Title == "" {
		display = msg.Track.Artist
	}
	m.queue.entries = append(m.queue.entries, QueueEntry{
		ID:             msg.QueueID,
		Path:           msg.Track.Path,
		Hash:           msg.Track.Hash,
		Display:        display,
		Artist:         msg.Track.Artist,
		Title:          msg.Track.Title,
		DurationMs:     int64(msg.Track.Duration * 1000),
		SourcePlaylist: msg.SourcePlaylist,
	})
	slog.Debug("track enqueued", "hash", msg.Track.Hash, "position", len(m.queue.entries)-1)
	// Keep the player's in-memory queue in sync.
	if pa, ok := m.player.(playerQueueAppender); ok {
		pa.AppendToQueue(player.QueueTrack{
			Path:    msg.Track.Path,
			Hash:    msg.Track.Hash,
			QueueID: msg.QueueID,
			Artist:  msg.Track.Artist,
			Title:   msg.Track.Title,
		})
	}
	// Set up undo slot so the user can revert the enqueue.
	enqueueIdx := len(m.queue.entries) - 1
	if m.undoSlot.cancelPurge != nil {
		close(m.undoSlot.cancelPurge)
	}
	m.undoSlot = undoSlot{
		kind:         undoKindEnqueue,
		expiresAt:    time.Now().Add(undoWindowDuration),
		dequeueID:    msg.QueueID,
		dequeueEntry: m.queue.entries[enqueueIdx],
		dequeueIdx:   enqueueIdx,
	}
	m.statusBar.notification = transientNotification{
		msg:       "Added " + display + " — press u to undo",
		expiresAt: time.Now().Add(undoWindowDuration),
	}
	player.PlaySound(player.SoundEnqueued)
	// Trigger immediate playback when the queue was previously empty and
	// nothing is playing.
	if msg.WasEmpty && m.playbackState == PlaybackIdle && m.player != nil {
		if err := m.player.PlayFromIndex(0); err == nil {
			m.playbackState = PlaybackPlaying
			m.currentTrackHash = msg.Track.Hash
			m.statusBar.artist = msg.Track.Artist
			m.statusBar.title = msg.Track.Title
			m.statusBar.playlistSource = "Queue"
		}
	}
	return m
}

// handleBatchEnqueuedMsg adds a batch of enqueued tracks (e.g. from a playlist)
// to the queue and triggers playback if the queue was empty.
func (m Model) handleBatchEnqueuedMsg(msg BatchEnqueuedMsg) Model {
	if msg.Err != nil {
		m.statusBar.notification = transientNotification{
			msg:       "Failed to enqueue playlist: " + msg.Err.Error(),
			expiresAt: time.Now().Add(transientDuration),
		}
		player.PlaySound(player.SoundError)
		return m
	}
	if len(msg.Entries) == 0 {
		m.statusBar.notification = transientNotification{
			msg:       "Playlist is empty",
			expiresAt: time.Now().Add(transientDuration),
		}
		return m
	}
	batchStartIdx := len(m.queue.entries)
	m.queue.entries = append(m.queue.entries, msg.Entries...)
	// Keep the player's in-memory queue in sync.
	if pa, ok := m.player.(playerQueueAppender); ok {
		for _, e := range msg.Entries {
			pa.AppendToQueue(player.QueueTrack{
				Path:    e.Path,
				Hash:    e.Hash,
				QueueID: e.ID,
				Artist:  e.Artist,
				Title:   e.Title,
			})
		}
	}
	// Trigger immediate playback when the queue was previously empty.
	if msg.WasEmpty && m.playbackState == PlaybackIdle && m.player != nil {
		if err := m.player.PlayFromIndex(0); err == nil {
			m.playbackState = PlaybackPlaying
			e := msg.Entries[0]
			m.currentTrackHash = e.Hash
			m.statusBar.artist = e.Artist
			m.statusBar.title = e.Title
			m.statusBar.playlistSource = msg.PlaylistName
		}
	}
	// Set up undo slot.
	if m.undoSlot.cancelPurge != nil {
		close(m.undoSlot.cancelPurge)
	}
	m.undoSlot = undoSlot{
		kind:          undoKindBatchEnqueue,
		expiresAt:     time.Now().Add(undoWindowDuration),
		batchIDs:      msg.QueueIDs,
		batchCount:    len(msg.Entries),
		batchStartIdx: batchStartIdx,
	}
	m.statusBar.notification = transientNotification{
		msg:       fmt.Sprintf("Added %d %s from %s — press u to undo", len(msg.Entries), pluralize(len(msg.Entries), "track", "tracks"), msg.PlaylistName),
		expiresAt: time.Now().Add(undoWindowDuration),
	}
	player.PlaySound(player.SoundEnqueued)
	return m
}

// handlePlayQueueIndexMsg starts playback from a specific queue index.
func (m Model) handlePlayQueueIndexMsg(msg PlayQueueIndexMsg) Model {
	if m.player != nil {
		if err := m.player.PlayFromIndex(msg.Index); err == nil {
			m.playbackState = PlaybackPlaying
			if msg.Index < len(m.queue.entries) {
				e := m.queue.entries[msg.Index]
				m.currentTrackHash = e.Hash
				m.statusBar.artist = e.Artist
				m.statusBar.title = e.Title
			}
			m.statusBar.playlistSource = "Queue"
		}
	}
	return m
}

// handleDequeueTrackMsg removes a track from the queue with undo support.
func (m Model) handleDequeueTrackMsg(msg DequeueTrackMsg) Model {
	if msg.Index >= 0 && msg.Index < len(m.queue.entries) {
		entry := m.queue.entries[msg.Index]
		undoer := m.undoer
		cancel := m.scheduleSoftDelete(
			func() {
				if undoer != nil {
					_ = undoer.DequeueTrackSoft(entry.ID)
				}
			},
			func(before int64) {
				if undoer != nil {
					_ = undoer.PurgeDeletedQueueEntries(before)
				}
			},
		)
		m.setUndoSlot(undoSlot{
			kind:         undoKindDequeue,
			expiresAt:    time.Now().Add(undoWindowDuration),
			cancelPurge:  cancel,
			dequeueID:    entry.ID,
			dequeueEntry: entry,
			dequeueIdx:   msg.Index,
		}, "Track removed — press u to undo")
		slog.Debug("track dequeued", "hash", entry.Hash, "position", msg.Index)
		player.PlaySound(player.SoundRemoved)
		m.queue.entries = append(m.queue.entries[:msg.Index], m.queue.entries[msg.Index+1:]...)
		m.clampQueueCursorAndScroll()
		if qr, ok := m.player.(playerQueueRemover); ok {
			_ = qr.RemoveFromQueue(msg.Index)
		}
		// If the dequeued track was the currently playing one, or the queue
		// is now empty, stop playback and clear the status bar.
		if (entry.Hash != "" && entry.Hash == m.currentTrackHash) || len(m.queue.entries) == 0 {
			m = m.resetPlaybackState()
		}
	}
	return m
}

// handleBulkSelectionToggleMsg toggles the selection state of a queue entry.
func (m Model) handleBulkSelectionToggleMsg(msg BulkSelectionToggleMsg) Model {
	if msg.Index >= 0 && msg.Index < len(m.queue.entries) {
		m.queue.entries[msg.Index].Selected = !m.queue.entries[msg.Index].Selected
	}
	return m
}

// handleClearQueueMsg clears the entire queue with undo support, stopping
// active playback.
func (m Model) handleClearQueueMsg() Model {
	savedEntries := make([]QueueEntry, len(m.queue.entries))
	copy(savedEntries, m.queue.entries)
	savedTracks := make([]player.QueueTrack, len(savedEntries))
	for i, e := range savedEntries {
		savedTracks[i] = player.QueueTrack{
			Path:    e.Path,
			Hash:    e.Hash,
			QueueID: e.ID,
			Artist:  e.Artist,
			Title:   e.Title,
		}
	}
	var clearIDs []int64
	undoer := m.undoer
	cancel := m.scheduleSoftDelete(
		func() {
			if undoer != nil {
				clearIDs, _ = undoer.ClearQueueSoft()
			}
		},
		func(before int64) {
			if undoer != nil {
				_ = undoer.PurgeDeletedQueueEntries(before)
			}
		},
	)
	m.setUndoSlot(undoSlot{
		kind:         undoKindClear,
		expiresAt:    time.Now().Add(undoWindowDuration),
		cancelPurge:  cancel,
		clearIDs:     clearIDs,
		clearEntries: savedEntries,
		clearTracks:  savedTracks,
	}, "Queue cleared — press u to undo")
	slog.Debug("queue cleared", "count", len(savedEntries))
	player.PlaySound(player.SoundRemoved)
	m.queue.entries = nil
	m.queue.cursor = 0
	m.queue.scrollOffset = 0
	m = m.resetPlaybackState()
	if qs, ok := m.player.(playerQueueSetter); ok {
		qs.SetQueue(nil)
	}
	return m
}

// clearQueueForReplace silently clears the queue and stops playback so that a
// subsequent enqueue becomes the only entry. Unlike handleClearQueueMsg this
// does not set up an undo slot — the enqueue that follows will create its own.
func (m Model) clearQueueForReplace() Model {
	if len(m.queue.entries) > 0 && m.undoer != nil {
		go func(undoer queueDequeuer) {
			if _, err := undoer.ClearQueueSoft(); err == nil {
				_ = undoer.PurgeDeletedQueueEntries(time.Now().Unix() + 1)
			}
		}(m.undoer)
	}
	// Cancel any in-flight undo purge timer.
	if m.undoSlot.cancelPurge != nil {
		close(m.undoSlot.cancelPurge)
	}
	m.undoSlot = undoSlot{}
	m.queue.entries = nil
	m.queue.cursor = 0
	m.queue.scrollOffset = 0
	m = m.resetPlaybackState()
	if qs, ok := m.player.(playerQueueSetter); ok {
		qs.SetQueue(nil)
	}
	return m
}

// handleMoveTrackUpMsg swaps a queue entry with the one above it and persists
// the reorder.
func (m Model) handleMoveTrackUpMsg(msg MoveTrackUpMsg) (Model, tea.Cmd) {
	if msg.Index > 0 && msg.Index < len(m.queue.entries) {
		entry := m.queue.entries[msg.Index]
		m.queue.entries[msg.Index-1], m.queue.entries[msg.Index] = m.queue.entries[msg.Index], m.queue.entries[msg.Index-1]
		m.queue.cursor = msg.Index - 1
		slog.Debug("track reordered", "hash", entry.Hash, "from", msg.Index, "to", msg.Index-1)
		m.syncPlayerQueue()
		if m.reorderer != nil {
			r := m.reorderer
			id := entry.ID
			return m, func() tea.Msg {
				_ = r.ReorderQueueTrack(id, -1)
				return nil
			}
		}
	}
	return m, nil
}

// handleMoveTrackDownMsg swaps a queue entry with the one below it and persists
// the reorder.
func (m Model) handleMoveTrackDownMsg(msg MoveTrackDownMsg) (Model, tea.Cmd) {
	if msg.Index >= 0 && msg.Index < len(m.queue.entries)-1 {
		entry := m.queue.entries[msg.Index]
		m.queue.entries[msg.Index], m.queue.entries[msg.Index+1] = m.queue.entries[msg.Index+1], m.queue.entries[msg.Index]
		m.queue.cursor = msg.Index + 1
		slog.Debug("track reordered", "hash", entry.Hash, "from", msg.Index, "to", msg.Index+1)
		m.syncPlayerQueue()
		if m.reorderer != nil {
			r := m.reorderer
			id := entry.ID
			return m, func() tea.Msg {
				_ = r.ReorderQueueTrack(id, +1)
				return nil
			}
		}
	}
	return m, nil
}

// handleUndo processes the UndoMsg by reversing the most recent destructive
// action stored in the undo slot, if one exists and has not expired.
func (m Model) handleUndo() Model {
	if m.undoSlot.kind == undoNone || !time.Now().Before(m.undoSlot.expiresAt) {
		m.statusBar.notification = transientNotification{
			msg:       "Nothing to undo",
			expiresAt: time.Now().Add(transientDuration),
		}
		return m
	}
	// Cancel any in-flight purge timer from the undo slot being consumed.
	// This must happen before the switch so that undo cases that schedule
	// new purge timers do not have them immediately cancelled by the
	// post-switch cleanup.
	if m.undoSlot.cancelPurge != nil {
		close(m.undoSlot.cancelPurge)
		m.undoSlot.cancelPurge = nil
	}
	slog.Debug("queue undo", "kind", m.undoSlot.kind)
	switch m.undoSlot.kind {
	case undoKindEnqueue:
		idx := m.undoSlot.dequeueIdx
		if idx >= 0 && idx < len(m.queue.entries) {
			entry := m.queue.entries[idx]
			if m.undoer != nil {
				_ = m.undoer.DequeueTrackSoft(entry.ID)
				undoer := m.undoer
				softDeleteTime := time.Now().Unix()
				time.AfterFunc(undoWindowDuration, func() {
					_ = undoer.PurgeDeletedQueueEntries(softDeleteTime + 1)
				})
			}
			m.queue.entries = append(m.queue.entries[:idx], m.queue.entries[idx+1:]...)
			m.clampQueueCursorAndScroll()
			if qr, ok := m.player.(playerQueueRemover); ok {
				_ = qr.RemoveFromQueue(idx)
			}
		}
	case undoKindDequeue:
		if m.undoer != nil {
			_ = m.undoer.UndoDequeue(m.undoSlot.dequeueID)
		}
		idx := min(m.undoSlot.dequeueIdx, len(m.queue.entries))
		entries := make([]QueueEntry, len(m.queue.entries)+1)
		copy(entries, m.queue.entries[:idx])
		entries[idx] = m.undoSlot.dequeueEntry
		copy(entries[idx+1:], m.queue.entries[idx:])
		m.queue.entries = entries
		m.queue.cursor = idx
		// Ensure the restored entry's cursor position is visible.
		if m.queue.cursor < m.queue.scrollOffset {
			m.queue.scrollOffset = m.queue.cursor
		} else {
			maxRows := m.queue.visibleRows()
			if m.queue.cursor >= m.queue.scrollOffset+maxRows {
				m.queue.scrollOffset = m.queue.cursor - maxRows + 1
			}
		}
		// Rebuild the player's in-memory queue so it stays in sync with the
		// TUI queue after the re-insertion.
		if qs, ok := m.player.(playerQueueSetter); ok {
			pt := make([]player.QueueTrack, len(m.queue.entries))
			for i, e := range m.queue.entries {
				pt[i] = player.QueueTrack{
					Path:    e.Path,
					Hash:    e.Hash,
					QueueID: e.ID,
					Artist:  e.Artist,
					Title:   e.Title,
				}
			}
			qs.SetQueue(pt)
		}
	case undoKindClear:
		if m.undoer != nil {
			_ = m.undoer.UndoClearQueue(m.undoSlot.clearIDs)
		}
		m.queue.entries = m.undoSlot.clearEntries
		if qs, ok := m.player.(playerQueueSetter); ok {
			qs.SetQueue(m.undoSlot.clearTracks)
		}
	case undoKindDeletePlaylist:
		if m.playlistDeleter != nil {
			_ = m.playlistDeleter.UndoDeletePlaylist(m.undoSlot.deletePlaylistEntry.ID)
		}
		idx := min(m.undoSlot.deletePlaylistIdx, len(m.playlists.playlists))
		playlists := make([]db.PlaylistRow, len(m.playlists.playlists)+1)
		copy(playlists, m.playlists.playlists[:idx])
		playlists[idx] = m.undoSlot.deletePlaylistEntry
		copy(playlists[idx+1:], m.playlists.playlists[idx:])
		m.playlists.playlists = playlists
		m.playlists.cursor = idx
		if m.playlists.cursor < m.playlists.scrollOffset {
			m.playlists.scrollOffset = m.playlists.cursor
		}
	case undoKindRemovePlaylistTrack:
		if m.playlistDeleter != nil {
			_ = m.playlistDeleter.UndoRemovePlaylistTrack(m.undoSlot.removeTrackEntry.ID)
		}
		idx := min(m.undoSlot.removeTrackIdx, len(m.playlists.tracks))
		tracks := make([]db.PlaylistTrackRow, len(m.playlists.tracks)+1)
		copy(tracks, m.playlists.tracks[:idx])
		tracks[idx] = m.undoSlot.removeTrackEntry
		copy(tracks[idx+1:], m.playlists.tracks[idx:])
		m.playlists.tracks = tracks
		m.playlists.cursor = idx
		if m.playlists.cursor < m.playlists.scrollOffset {
			m.playlists.scrollOffset = m.playlists.cursor
		}
	case undoKindBatchEnqueue:
		if m.batchEnqueuer != nil {
			_ = m.batchEnqueuer.DequeueTracksSoft(m.undoSlot.batchIDs)
			if m.undoer != nil {
				undoer := m.undoer
				softDeleteTime := time.Now().Unix()
				time.AfterFunc(undoWindowDuration, func() {
					_ = undoer.PurgeDeletedQueueEntries(softDeleteTime + 1)
				})
			}
		}
		startIdx := m.undoSlot.batchStartIdx
		endIdx := startIdx + m.undoSlot.batchCount
		if endIdx > len(m.queue.entries) {
			endIdx = len(m.queue.entries)
		}
		if startIdx < len(m.queue.entries) {
			m.queue.entries = append(m.queue.entries[:startIdx], m.queue.entries[endIdx:]...)
		}
		m.clampQueueCursorAndScroll()
		if qr, ok := m.player.(playerQueueRemover); ok {
			for i := endIdx - 1; i >= startIdx; i-- {
				_ = qr.RemoveFromQueue(i)
			}
		}
	case undoKindBulkDequeue:
		// Restore soft-deleted DB rows.
		if m.undoer != nil {
			for _, id := range m.undoSlot.bulkDequeueIDs {
				_ = m.undoer.UndoDequeue(id)
			}
		}
		// Re-insert entries at their original indices in forward order.
		// Each insertion shifts later indices by one, which is exactly
		// what we need since the stored indices are from the original
		// (pre-removal) ordering.
		for i := range m.undoSlot.bulkDequeueIndices {
			idx := min(m.undoSlot.bulkDequeueIndices[i], len(m.queue.entries))
			entries := make([]QueueEntry, len(m.queue.entries)+1)
			copy(entries, m.queue.entries[:idx])
			entries[idx] = m.undoSlot.bulkDequeueEntries[i]
			copy(entries[idx+1:], m.queue.entries[idx:])
			m.queue.entries = entries
		}
		m.clampQueueCursorAndScroll()
		m.syncPlayerQueue()
	case undoKindBulkRemovePlaylistTracks:
		// Restore soft-deleted DB rows.
		if m.playlistDeleter != nil {
			for _, t := range m.undoSlot.bulkRemoveTrackEntries {
				_ = m.playlistDeleter.UndoRemovePlaylistTrack(t.ID)
			}
		}
		// Re-insert tracks at their original indices in forward order.
		for i := range m.undoSlot.bulkRemoveTrackIndices {
			idx := min(m.undoSlot.bulkRemoveTrackIndices[i], len(m.playlists.tracks))
			tracks := make([]db.PlaylistTrackRow, len(m.playlists.tracks)+1)
			copy(tracks, m.playlists.tracks[:idx])
			tracks[idx] = m.undoSlot.bulkRemoveTrackEntries[i]
			copy(tracks[idx+1:], m.playlists.tracks[idx:])
			m.playlists.tracks = tracks
		}
		if m.playlists.cursor < 0 {
			m.playlists.cursor = 0
		} else if m.playlists.cursor >= len(m.playlists.tracks) {
			m.playlists.cursor = len(m.playlists.tracks) - 1
		}
		if m.playlists.cursor < m.playlists.scrollOffset {
			m.playlists.scrollOffset = m.playlists.cursor
		}
	}
	m.undoSlot = undoSlot{}
	return m
}

// handleQueueKeyAction handles queue-related key actions (select, dequeue,
// clear, move, save-as-playlist, undo). It returns true in the third value
// when the action was handled.
func (m Model) handleQueueKeyAction(action action) (tea.Model, tea.Cmd, bool) {
	switch action {
	case actionUndo:
		return m, func() tea.Msg { return UndoMsg{} }, true
	case actionDequeue:
		if m.queue.hasSelection() {
			// Bulk dequeue all selected entries — collect removed entries
			// and their original indices for undo support.
			var removedEntries []QueueEntry
			var removedIndices []int
			var removedIDs []int64
			for i, e := range m.queue.entries {
				if e.Selected {
					removedEntries = append(removedEntries, e)
					removedIndices = append(removedIndices, i)
					removedIDs = append(removedIDs, e.ID)
				}
			}
			undoer := m.undoer
			cancel := m.scheduleSoftDelete(
				func() {
					if undoer != nil {
						for _, id := range removedIDs {
							_ = undoer.DequeueTrackSoft(id)
						}
					}
				},
				func(before int64) {
					if undoer != nil {
						_ = undoer.PurgeDeletedQueueEntries(before)
					}
				},
			)
			var remaining []QueueEntry
			for _, e := range m.queue.entries {
				if !e.Selected {
					remaining = append(remaining, e)
				}
			}
			m.queue.entries = remaining
			m.clampQueueCursorAndScroll()
			m.syncPlayerQueue()
			// If the queue is now empty, stop playback and clear status bar.
			if len(m.queue.entries) == 0 {
				m = m.resetPlaybackState()
			}
			n := len(removedIDs)
			m.setUndoSlot(undoSlot{
				kind:               undoKindBulkDequeue,
				expiresAt:          time.Now().Add(undoWindowDuration),
				cancelPurge:        cancel,
				bulkDequeueEntries: removedEntries,
				bulkDequeueIndices: removedIndices,
				bulkDequeueIDs:     removedIDs,
			}, fmt.Sprintf("%d %s removed — press u to undo", n, pluralize(n, "track", "tracks")))
			player.PlaySound(player.SoundRemoved)
			return m, nil, true
		}
		var cmd tea.Cmd
		m.queue, cmd = m.queue.HandleAction(action)
		return m, cmd, true
	case actionClearQueue:
		if len(m.queue.entries) == 0 {
			return m, nil, true
		}
		now := time.Now()
		if m.clearQueuePending && now.Before(m.clearQueuePendingAt.Add(2*time.Second)) {
			m.clearQueuePending = false
			var cmd tea.Cmd
			m.queue, cmd = m.queue.HandleAction(action)
			return m, cmd, true
		}
		m.clearQueuePending = true
		m.clearQueuePendingAt = now
		m.statusBar.notification = transientNotification{
			msg:       "Press C again to clear queue",
			expiresAt: now.Add(2 * time.Second),
		}
		return m, nil, true
	case actionSelect,
		actionMoveTrackUp, actionMoveTrackDown:
		var cmd tea.Cmd
		m.queue, cmd = m.queue.HandleAction(action)
		return m, cmd, true
	case actionSaveQueueAsPlaylist:
		if len(m.queue.entries) == 0 {
			m.statusBar.notification = transientNotification{
				msg:       "Queue is empty — nothing to save",
				expiresAt: time.Now().Add(transientDuration),
			}
		} else {
			fromSelection := m.queue.hasSelection()
			m.queue = m.queue.startSaveAsPlaylist(fromSelection)
		}
		return m, nil, true
	}
	return m, nil, false
}
