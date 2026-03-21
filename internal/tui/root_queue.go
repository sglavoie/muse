package tui

import (
	"fmt"
	"log/slog"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/player"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	libtui "github.com/sglavoie/muse/internal/tui/library"
)

// newQueueEntries builds QueueEntry values from matched tracks and DB row IDs.
// source is the playlist name to record on each entry (empty for individual enqueues).
func newQueueEntries(tracks []library.Track, ids []int64, source string) []QueueEntry {
	entries := make([]QueueEntry, len(tracks))
	for i, t := range tracks {
		entries[i] = QueueEntry{
			ID:             ids[i],
			Path:           t.Path,
			Hash:           t.Hash,
			Display:        trackDisplay(t.Artist, t.Title, t.Path),
			Artist:         t.Artist,
			Title:          t.Title,
			Album:          t.Album,
			DurationMs:     int64(t.Duration * 1000),
			SourcePlaylist: source,
		}
	}
	return entries
}

// clampQueueCursorAndScroll adjusts the queue cursor and scroll offset so they
// remain within bounds after entries have been removed.
func (m *Model) clampQueueCursorAndScroll() {
	m.queue.cursor, m.queue.scrollOffset = clampAfterRemoval(
		m.queue.cursor, m.queue.scrollOffset, len(m.queue.entries))
}

// resetPlaybackState stops the player and clears all playback-related fields
// in the model and status bar.
func (m Model) resetPlaybackState() Model {
	if cp, ok := m.player.(commandPlayer); ok {
		_ = cp.Stop()
	}
	m.playback.state = PlaybackIdle
	m.playback.elapsedSecs = 0
	m.playback.totalSecs = 0
	m.statusBar.artist = ""
	m.statusBar.title = ""
	m.statusBar.playlistSource = ""
	m.playback.currentHash = ""
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
	display := trackDisplay(msg.Track.Artist, msg.Track.Title, msg.Track.Path)
	m.queue.entries = append(m.queue.entries, QueueEntry{
		ID:             msg.QueueID,
		Path:           msg.Track.Path,
		Hash:           msg.Track.Hash,
		Display:        display,
		Artist:         msg.Track.Artist,
		Title:          msg.Track.Title,
		Album:          msg.Track.Album,
		DurationMs:     int64(msg.Track.Duration * 1000),
		SourcePlaylist: msg.SourcePlaylist,
	})
	m = m.resetQueueSort()
	slog.Debug("track enqueued", "hash", msg.Track.Hash, "position", len(m.queue.entries)-1)
	// Keep the player's in-memory queue in sync.
	if pa, ok := m.player.(playerQueueAppender); ok {
		pa.AppendToQueue(player.QueueTrack{
			Path:    msg.Track.Path,
			Hash:    msg.Track.Hash,
			QueueID: msg.QueueID,
			Artist:  msg.Track.Artist,
			Title:   msg.Track.Title,
			Source:  msg.SourcePlaylist,
		})
	}
	// Set up undo slot so the user can revert the enqueue.
	enqueueIdx := len(m.queue.entries) - 1
	if m.undo.slot.cancelPurge != nil {
		close(m.undo.slot.cancelPurge)
	}
	m.undo.slot = undoSlot{
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
	if msg.WasEmpty && m.playback.state == PlaybackIdle {
		m = m.tryAutoPlay(msg.Track.Hash, msg.Track.Artist, msg.Track.Title, msg.SourcePlaylist)
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
	m = m.resetQueueSort()
	// Keep the player's in-memory queue in sync.
	if pa, ok := m.player.(playerQueueAppender); ok {
		for _, e := range msg.Entries {
			pa.AppendToQueue(player.QueueTrack{
				Path:    e.Path,
				Hash:    e.Hash,
				QueueID: e.ID,
				Artist:  e.Artist,
				Title:   e.Title,
				Source:  e.SourcePlaylist,
			})
		}
	}
	// Trigger immediate playback when the queue was previously empty.
	if msg.WasEmpty && m.playback.state == PlaybackIdle {
		e := msg.Entries[0]
		m = m.tryAutoPlay(e.Hash, e.Artist, e.Title, msg.PlaylistName)
	}
	// Set up undo slot.
	if m.undo.slot.cancelPurge != nil {
		close(m.undo.slot.cancelPurge)
	}
	m.undo.slot = undoSlot{
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

// tryAutoPlay attempts to start playback from index 0 after an enqueue into an
// empty queue. On failure it notifies the user and reverts to idle.
func (m Model) tryAutoPlay(hash, artist, title, source string) Model {
	if m.player == nil {
		return m
	}
	if err := m.player.PlayFromIndex(0); err != nil {
		m.playback.state = PlaybackIdle
		m.statusBar.notification = transientNotification{
			msg:       "Playback failed: " + err.Error(),
			expiresAt: time.Now().Add(transientDuration),
		}
		player.PlaySound(player.SoundError)
		return m
	}
	m.playback.state = PlaybackPlaying
	m.playback.currentHash = hash
	m.statusBar.artist = artist
	m.statusBar.title = title
	if source == "" {
		source = "Queue"
	}
	m.statusBar.playlistSource = source
	return m
}

// handlePlayQueueIndexMsg starts playback from a specific queue index.
func (m Model) handlePlayQueueIndexMsg(msg PlayQueueIndexMsg) Model {
	if m.player != nil {
		if err := m.player.PlayFromIndex(msg.Index); err == nil {
			m.playback.state = PlaybackPlaying
			if msg.Index < len(m.queue.entries) {
				e := m.queue.entries[msg.Index]
				m.playback.currentHash = e.Hash
				m.statusBar.artist = e.Artist
				m.statusBar.title = e.Title
				source := e.SourcePlaylist
				if source == "" {
					source = "Queue"
				}
				m.statusBar.playlistSource = source
			}
		} else {
			m.playback.state = PlaybackIdle
			m.statusBar.notification = transientNotification{
				msg:       "Playback failed: " + err.Error(),
				expiresAt: time.Now().Add(transientDuration),
			}
			player.PlaySound(player.SoundError)
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
		m = m.resetQueueSort()
		m.clampQueueCursorAndScroll()
		if qr, ok := m.player.(playerQueueRemover); ok {
			_ = qr.RemoveFromQueue(msg.Index)
		}
		// If the dequeued track was the currently playing one, or the queue
		// is now empty, stop playback and clear the status bar.
		if (entry.Hash != "" && entry.Hash == m.playback.currentHash) || len(m.queue.entries) == 0 {
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
			Source:  e.SourcePlaylist,
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
	m.queue.filter = m.queue.filter.Deactivate()
	m = m.resetQueueSort()
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
				if err := undoer.PurgeDeletedQueueEntries(time.Now().Unix() + 1); err != nil {
					slog.Error("purge deleted queue entries failed", "err", err)
				}
			}
		}(m.undoer)
	}
	// Cancel any in-flight undo purge timer.
	if m.undo.slot.cancelPurge != nil {
		close(m.undo.slot.cancelPurge)
	}
	m.undo.slot = undoSlot{}
	m.queue.entries = nil
	m.queue.cursor = 0
	m.queue.scrollOffset = 0
	m = m.resetQueueSort()
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
		m = m.resetQueueSort()
		slog.Debug("track reordered", "hash", entry.Hash, "from", msg.Index, "to", msg.Index-1)
		m.syncPlayerQueue()
		if m.reorderer != nil {
			r := m.reorderer
			id := entry.ID
			return m, func() tea.Msg {
				if err := r.ReorderQueueTrack(id, -1); err != nil {
					slog.Error("reorder queue track failed", "id", id, "direction", -1, "err", err)
				}
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
		m = m.resetQueueSort()
		slog.Debug("track reordered", "hash", entry.Hash, "from", msg.Index, "to", msg.Index+1)
		m.syncPlayerQueue()
		if m.reorderer != nil {
			r := m.reorderer
			id := entry.ID
			return m, func() tea.Msg {
				if err := r.ReorderQueueTrack(id, +1); err != nil {
					slog.Error("reorder queue track failed", "id", id, "direction", +1, "err", err)
				}
				return nil
			}
		}
	}
	return m, nil
}

// handleUndo processes the UndoMsg by reversing the most recent destructive
// action stored in the undo slot, if one exists and has not expired.
func (m Model) handleUndo() Model {
	if m.undo.slot.kind == undoNone || !time.Now().Before(m.undo.slot.expiresAt) {
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
	if m.undo.slot.cancelPurge != nil {
		close(m.undo.slot.cancelPurge)
		m.undo.slot.cancelPurge = nil
	}
	slog.Debug("queue undo", "kind", m.undo.slot.kind)
	switch m.undo.slot.kind {
	case undoKindEnqueue:
		idx := m.undo.slot.dequeueIdx
		if idx >= 0 && idx < len(m.queue.entries) {
			entry := m.queue.entries[idx]
			if m.undoer != nil {
				if err := m.undoer.DequeueTrackSoft(entry.ID); err != nil {
					slog.Warn("undo enqueue: soft-delete failed", "err", err)
				}
				undoer := m.undoer
				softDeleteTime := time.Now().Unix()
				time.AfterFunc(undoWindowDuration, func() {
					if err := undoer.PurgeDeletedQueueEntries(softDeleteTime + 1); err != nil {
						slog.Warn("undo enqueue: purge failed", "err", err)
					}
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
			if err := m.undoer.UndoDequeue(m.undo.slot.dequeueID); err != nil {
				slog.Warn("undo dequeue failed", "err", err)
			}
		}
		idx := min(m.undo.slot.dequeueIdx, len(m.queue.entries))
		entries := make([]QueueEntry, len(m.queue.entries)+1)
		copy(entries, m.queue.entries[:idx])
		entries[idx] = m.undo.slot.dequeueEntry
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
		m.syncPlayerQueue()
	case undoKindClear:
		if m.undoer != nil {
			if err := m.undoer.UndoClearQueue(m.undo.slot.clearIDs); err != nil {
				slog.Warn("undo clear queue failed", "err", err)
			}
		}
		m.queue.entries = m.undo.slot.clearEntries
		if qs, ok := m.player.(playerQueueSetter); ok {
			qs.SetQueue(m.undo.slot.clearTracks)
		}
	case undoKindDeletePlaylist:
		if m.playlistDeleter != nil {
			if err := m.playlistDeleter.UndoDeletePlaylist(m.undo.slot.deletePlaylistEntry.ID); err != nil {
				slog.Warn("undo delete playlist failed", "err", err)
			}
		}
		idx := min(m.undo.slot.deletePlaylistIdx, len(m.playlists.playlists))
		playlists := make([]db.PlaylistRow, len(m.playlists.playlists)+1)
		copy(playlists, m.playlists.playlists[:idx])
		playlists[idx] = m.undo.slot.deletePlaylistEntry
		copy(playlists[idx+1:], m.playlists.playlists[idx:])
		m.playlists.playlists = playlists
		m.playlists.cursor = idx
		if m.playlists.cursor < m.playlists.scrollOffset {
			m.playlists.scrollOffset = m.playlists.cursor
		}
	case undoKindRemovePlaylistTrack:
		if m.playlistDeleter != nil {
			if err := m.playlistDeleter.UndoRemovePlaylistTrack(m.undo.slot.removeTrackEntry.ID); err != nil {
				slog.Warn("undo remove playlist track failed", "err", err)
			}
		}
		idx := min(m.undo.slot.removeTrackIdx, len(m.playlists.tracks))
		tracks := make([]db.PlaylistTrackRow, len(m.playlists.tracks)+1)
		copy(tracks, m.playlists.tracks[:idx])
		tracks[idx] = m.undo.slot.removeTrackEntry
		copy(tracks[idx+1:], m.playlists.tracks[idx:])
		m.playlists.tracks = tracks
		m.playlists.cursor = idx
		if m.playlists.cursor < m.playlists.scrollOffset {
			m.playlists.scrollOffset = m.playlists.cursor
		}
	case undoKindBatchEnqueue:
		if m.batchEnqueuer != nil {
			if err := m.batchEnqueuer.DequeueTracksSoft(m.undo.slot.batchIDs); err != nil {
				slog.Warn("undo batch enqueue: soft-delete failed", "err", err)
			}
			if m.undoer != nil {
				undoer := m.undoer
				softDeleteTime := time.Now().Unix()
				time.AfterFunc(undoWindowDuration, func() {
					if err := undoer.PurgeDeletedQueueEntries(softDeleteTime + 1); err != nil {
						slog.Warn("undo batch enqueue: purge failed", "err", err)
					}
				})
			}
		}
		startIdx := m.undo.slot.batchStartIdx
		endIdx := startIdx + m.undo.slot.batchCount
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
			for _, id := range m.undo.slot.bulkDequeueIDs {
				if err := m.undoer.UndoDequeue(id); err != nil {
					slog.Warn("undo bulk dequeue failed", "err", err)
				}
			}
		}
		// Re-insert entries at their original indices in forward order.
		// Each insertion shifts later indices by one, which is exactly
		// what we need since the stored indices are from the original
		// (pre-removal) ordering.
		for i := range m.undo.slot.bulkDequeueIndices {
			idx := min(m.undo.slot.bulkDequeueIndices[i], len(m.queue.entries))
			entries := make([]QueueEntry, len(m.queue.entries)+1)
			copy(entries, m.queue.entries[:idx])
			entries[idx] = m.undo.slot.bulkDequeueEntries[i]
			copy(entries[idx+1:], m.queue.entries[idx:])
			m.queue.entries = entries
		}
		m.clampQueueCursorAndScroll()
		m.syncPlayerQueue()
	case undoKindBulkEditMetadata:
		w := m.metadataWriter
		for hash, orig := range m.undo.slot.bulkEditOriginals {
			m.library.index.UpdateMetadata(hash, orig[0], orig[1], orig[2])
			display := fmt.Sprintf("%s – %s", orig[1], orig[0])
			for i := range m.queue.entries {
				if m.queue.entries[i].Hash == hash {
					m.queue.entries[i].Title = orig[0]
					m.queue.entries[i].Artist = orig[1]
					m.queue.entries[i].Display = display
				}
			}
		}
		if w != nil {
			originals := m.undo.slot.bulkEditOriginals
			go func() {
				for h, orig := range originals {
					if err := w.UpdateTrackMetadata(h, orig[0], orig[1], orig[2]); err != nil {
						slog.Warn("undo bulk metadata edit: DB write failed", "hash", h, "err", err)
					}
				}
			}()
		}
		n := len(m.undo.slot.bulkEditOriginals)
		m.statusBar.notification = transientNotification{
			msg:       fmt.Sprintf("Undone metadata edit for %d %s", n, pluralize(n, "track", "tracks")),
			expiresAt: time.Now().Add(transientDuration),
		}
	case undoKindBulkRemovePlaylistTracks:
		// Restore soft-deleted DB rows.
		if m.playlistDeleter != nil {
			for _, t := range m.undo.slot.bulkRemoveTrackEntries {
				if err := m.playlistDeleter.UndoRemovePlaylistTrack(t.ID); err != nil {
					slog.Warn("undo bulk remove playlist track failed", "err", err)
				}
			}
		}
		// Re-insert tracks at their original indices in forward order.
		for i := range m.undo.slot.bulkRemoveTrackIndices {
			idx := min(m.undo.slot.bulkRemoveTrackIndices[i], len(m.playlists.tracks))
			tracks := make([]db.PlaylistTrackRow, len(m.playlists.tracks)+1)
			copy(tracks, m.playlists.tracks[:idx])
			tracks[idx] = m.undo.slot.bulkRemoveTrackEntries[i]
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
	case undoKindRemoveFavorite:
		typ := m.undo.slot.removeFavType
		value := m.undo.slot.removeFavValue
		if m.favoriteWriter != nil {
			if err := m.favoriteWriter.AddFavorite(typ, value); err != nil {
				slog.Warn("undo remove favorite failed", "err", err)
			}
		}
		// Restore in-memory hash set for tracks.
		if typ == "track" && value != "" {
			if m.favoriteHashes == nil {
				m.favoriteHashes = make(map[string]struct{})
			}
			m.favoriteHashes[value] = struct{}{}
		}
		// Restore in-view state if still in matching Favorites view.
		idx := m.undo.slot.removeFavIdx
		if m.activeView == viewFavorites && idx >= 0 &&
			m.favorites.level == m.undo.slot.removeFavLevel {
			switch m.undo.slot.removeFavLevel {
			case favtui.ListLevel:
				switch m.undo.slot.removeFavCategory {
				case favtui.CategoryArtists:
					m.favorites.artists = insertStringAt(m.favorites.artists, idx, value)
					m.favorites.artistCount = len(m.favorites.artists)
				case favtui.CategoryAlbums:
					m.favorites.albums = insertStringAt(m.favorites.albums, idx, value)
					m.favorites.albumCount = len(m.favorites.albums)
				case favtui.CategoryTracks:
					m.favorites.tracks = insertTrackAt(m.favorites.tracks, idx, m.undo.slot.removeFavTrack)
					m.favorites.trackCount = len(m.favorites.tracks)
				}
				m.favorites.cursor = idx
				m.favorites = m.favorites.clampCursor()
			case favtui.TrackLevel:
				m.favorites.drilldownTracks = insertTrackAt(m.favorites.drilldownTracks, idx, m.undo.slot.removeFavTrack)
				m.favorites.cursor = idx
				m.favorites = m.favorites.clampCursor()
			}
		}
		// Refresh counts if in Favorites view.
		if m.activeView == viewFavorites && m.favoriteReader != nil {
			a, al, t, err := m.favoriteReader.ReadFavoriteCounts()
			if err == nil {
				m.favorites.artistCount = a
				m.favorites.albumCount = al
				m.favorites.trackCount = t
			}
		}
		m.statusBar.notification = transientNotification{
			msg:       fmt.Sprintf("Undone: added %s back to favorites", typ),
			expiresAt: time.Now().Add(transientDuration),
		}
	case undoKindBulkRemoveFavorites:
		typ := m.undo.slot.bulkRemoveFavType
		values := m.undo.slot.bulkRemoveFavValues
		if m.favoriteWriter != nil {
			for _, v := range values {
				if err := m.favoriteWriter.AddFavorite(typ, v); err != nil {
					slog.Warn("undo bulk remove favorite failed", "err", err)
				}
			}
		}
		// Restore in-memory hash set for tracks.
		if typ == "track" {
			if m.favoriteHashes == nil {
				m.favoriteHashes = make(map[string]struct{})
			}
			for _, v := range values {
				m.favoriteHashes[v] = struct{}{}
			}
		}
		// Restore in-view state if still in matching Favorites view.
		indices := m.undo.slot.bulkRemoveFavIndices
		if m.activeView == viewFavorites && len(indices) > 0 &&
			m.favorites.level == m.undo.slot.bulkRemoveFavLevel {
			switch m.undo.slot.bulkRemoveFavLevel {
			case favtui.ListLevel:
				switch m.undo.slot.bulkRemoveFavCategory {
				case favtui.CategoryArtists:
					for i, idx := range indices {
						m.favorites.artists = insertStringAt(m.favorites.artists, min(idx, len(m.favorites.artists)), values[i])
					}
					m.favorites.artistCount = len(m.favorites.artists)
				case favtui.CategoryAlbums:
					for i, idx := range indices {
						m.favorites.albums = insertStringAt(m.favorites.albums, min(idx, len(m.favorites.albums)), values[i])
					}
					m.favorites.albumCount = len(m.favorites.albums)
				case favtui.CategoryTracks:
					tracks := m.undo.slot.bulkRemoveFavTracks
					for i, idx := range indices {
						m.favorites.tracks = insertTrackAt(m.favorites.tracks, min(idx, len(m.favorites.tracks)), tracks[i])
					}
					m.favorites.trackCount = len(m.favorites.tracks)
				}
				m.favorites = m.favorites.clampCursor()
			case favtui.TrackLevel:
				tracks := m.undo.slot.bulkRemoveFavTracks
				for i, idx := range indices {
					m.favorites.drilldownTracks = insertTrackAt(m.favorites.drilldownTracks, min(idx, len(m.favorites.drilldownTracks)), tracks[i])
				}
				m.favorites = m.favorites.clampCursor()
			}
		}
		// Refresh counts if in Favorites view.
		if m.activeView == viewFavorites && m.favoriteReader != nil {
			a, al, t, err := m.favoriteReader.ReadFavoriteCounts()
			if err == nil {
				m.favorites.artistCount = a
				m.favorites.albumCount = al
				m.favorites.trackCount = t
			}
		}
		n := len(values)
		m.statusBar.notification = transientNotification{
			msg:       fmt.Sprintf("Undone: added %d %s(s) back to favorites", n, typ),
			expiresAt: time.Now().Add(transientDuration),
		}
	}
	m = m.resetQueueSort()
	m = m.resetPlaylistsSort()
	m.undo.slot = undoSlot{}
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
			m = m.resetQueueSort()
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
		if m.undo.clearQueuePending && now.Before(m.undo.clearQueuePendingAt.Add(queueConfirmationWindow)) {
			m.undo.clearQueuePending = false
			var cmd tea.Cmd
			m.queue, cmd = m.queue.HandleAction(action)
			return m, cmd, true
		}
		m.undo.clearQueuePending = true
		m.undo.clearQueuePendingAt = now
		m.statusBar.notification = transientNotification{
			msg:       "Press C again to clear queue",
			expiresAt: now.Add(queueConfirmationWindow),
		}
		return m, nil, true
	case actionSelect,
		actionMoveTrackUp, actionMoveTrackDown:
		var cmd tea.Cmd
		m.queue, cmd = m.queue.HandleAction(action)
		return m, cmd, true
	case actionFocusNowPlaying:
		m.queue = m.queue.FocusNowPlaying(m.playback.currentHash)
		return m, nil, true
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
