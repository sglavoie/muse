package tui

import (
	"log/slog"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/player"
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

// queueContainsHash reports whether any entry in entries has the given hash.
func queueContainsHash(entries []QueueEntry, hash string) bool {
	for _, e := range entries {
		if e.Hash == hash {
			return true
		}
	}
	return false
}

// clampQueueCursorAndScroll adjusts the queue cursor and scroll offset so they
// remain within bounds after entries have been removed. When a filter is active
// the cursor indexes into the filtered list, so we clamp against the filtered
// count rather than the full entry count.
func (m *Model) clampQueueCursorAndScroll() {
	n := len(m.queue.filteredEntries())
	m.queue.cursor, m.queue.scrollOffset = clampAfterRemoval(
		m.queue.cursor, m.queue.scrollOffset, n)
}

// resetPlaybackState stops the player and clears all playback-related fields
// in the model and status bar.
func (m Model) resetPlaybackState() Model {
	if cp, ok := m.player.(commandPlayer); ok {
		_ = cp.Stop() // stop before new playback, failure is non-fatal
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

// playNextIndex returns the queue index where a "play next" track should be
// inserted: immediately after the currently playing track. When nothing is
// playing, it returns 0 (front of queue).
func (m Model) playNextIndex() int {
	if m.playback.currentHash == "" {
		return 0
	}
	for i, e := range m.queue.entries {
		if e.Hash == m.playback.currentHash {
			return i + 1
		}
	}
	return 0
}

// handleTrackEnqueuedMsg adds a newly enqueued track to the queue, sets up an
// undo slot, and triggers playback if the queue was empty. When InsertAfter is
// >= 0 the track is inserted at that position ("play next") instead of being
// appended to the end.
func (m Model) handleTrackEnqueuedMsg(msg libtui.TrackEnqueuedMsg) Model {
	if msg.Err != nil {
		m.statusBar.notification = newNotification("Enqueue track failed: " + msg.Err.Error())
		player.PlaySound(player.SoundError)
		return m
	}
	display := trackDisplay(msg.Track.Artist, msg.Track.Title, msg.Track.Path)
	entry := QueueEntry{
		ID:             msg.QueueID,
		Path:           msg.Track.Path,
		Hash:           msg.Track.Hash,
		Display:        display,
		Artist:         msg.Track.Artist,
		Title:          msg.Track.Title,
		Album:          msg.Track.Album,
		DurationMs:     int64(msg.Track.Duration * 1000),
		SourcePlaylist: msg.SourcePlaylist,
	}
	var enqueueIdx int
	if msg.InsertAfter >= 0 && msg.InsertAfter <= len(m.queue.entries) {
		// Insert at the requested position ("play next").
		idx := msg.InsertAfter
		entries := make([]QueueEntry, len(m.queue.entries)+1)
		copy(entries, m.queue.entries[:idx])
		entries[idx] = entry
		copy(entries[idx+1:], m.queue.entries[idx:])
		m.queue.entries = entries
		enqueueIdx = idx
		slog.Debug("track play-next", "hash", msg.Track.Hash, "position", idx)
		m.syncPlayerQueue()
	} else {
		// Append to end (default enqueue).
		m.queue.entries = append(m.queue.entries, entry)
		enqueueIdx = len(m.queue.entries) - 1
		slog.Debug("track enqueued", "hash", msg.Track.Hash, "position", enqueueIdx)
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
	}
	m = m.resetQueueSort()
	// Set up undo slot so the user can revert the enqueue.
	if m.undo.slot.cancelPurge != nil {
		close(m.undo.slot.cancelPurge)
	}
	m.setUndoSlot(undoEnqueueAction{
		id:    msg.QueueID,
		entry: m.queue.entries[enqueueIdx],
		idx:   enqueueIdx,
	}, nil)
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
		m.statusBar.notification = newNotification("Enqueue playlist failed: " + msg.Err.Error())
		player.PlaySound(player.SoundError)
		return m
	}
	if len(msg.Entries) == 0 {
		m.statusBar.notification = newNotification("Playlist is empty")
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
	m.setUndoSlot(undoBatchEnqueueAction{
		ids:          msg.QueueIDs,
		count:        len(msg.Entries),
		startIdx:     batchStartIdx,
		playlistName: msg.PlaylistName,
	}, nil)
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
		m.statusBar.notification = newNotification("Play failed: " + err.Error())
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
			m.playback.currentHash = ""
			m.playback.elapsedSecs = 0
			m.playback.totalSecs = 0
			m.statusBar.artist = ""
			m.statusBar.title = ""
			m.statusBar.playlistSource = ""
			m.statusBar.notification = newNotification("Play failed: " + err.Error())
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
					if err := undoer.DequeueTrackSoft(entry.ID); err != nil {
						slog.Warn("dequeue soft-delete failed", "err", err)
					}
				}
			},
			func(before int64) {
				if undoer != nil {
					if err := undoer.PurgeDeletedQueueEntries(before); err != nil {
						slog.Warn("queue purge failed", "err", err)
					}
				}
			},
		)
		m.setUndoSlot(undoDequeueAction{
			id:    entry.ID,
			entry: entry,
			idx:   msg.Index,
		}, cancel)
		slog.Debug("track dequeued", "hash", entry.Hash, "position", msg.Index)
		player.PlaySound(player.SoundRemoved)
		m.queue.entries = append(m.queue.entries[:msg.Index], m.queue.entries[msg.Index+1:]...)
		m = m.resetQueueSort()
		m.clampQueueCursorAndScroll()
		if qr, ok := m.player.(playerQueueRemover); ok {
			_ = qr.RemoveFromQueue(msg.Index) // player queue sync is best-effort, TUI is source of truth
		}
		// If the queue is now empty, or the currently-playing hash is no
		// longer present in the remaining queue entries, stop playback.
		// Comparing by hash presence (not entry identity) handles the case
		// where the same track is enqueued multiple times: removing one
		// instance does not stop playback if another instance remains.
		if len(m.queue.entries) == 0 || (m.playback.currentHash != "" && !queueContainsHash(m.queue.entries, m.playback.currentHash)) {
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
				var err error
				clearIDs, err = undoer.ClearQueueSoft()
				if err != nil {
					slog.Warn("queue clear soft-delete failed", "err", err)
				}
			}
		},
		func(before int64) {
			if undoer != nil {
				if err := undoer.PurgeDeletedQueueEntries(before); err != nil {
					slog.Warn("queue purge failed", "err", err)
				}
			}
		},
	)
	m.setUndoSlot(undoClearAction{
		ids:     clearIDs,
		entries: savedEntries,
		tracks:  savedTracks,
	}, cancel)
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
	m.queue.filter = m.queue.filter.Deactivate()
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
		if m.queue.cursor < m.queue.scrollOffset {
			m.queue.scrollOffset = m.queue.cursor
		}
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
		maxRows := m.queue.visibleRows()
		if maxRows > 0 && m.queue.cursor >= m.queue.scrollOffset+maxRows {
			m.queue.scrollOffset = m.queue.cursor - maxRows + 1
		}
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
	if m.undo.slot.action == nil || !time.Now().Before(m.undo.slot.expiresAt) {
		m.statusBar.notification = newNotification("Nothing to undo")
		return m
	}
	// Cancel any in-flight purge timer before delegating to perform(). This
	// must happen first so that undo actions which schedule new purge timers
	// do not have them immediately cancelled by the post-perform cleanup.
	if m.undo.slot.cancelPurge != nil {
		close(m.undo.slot.cancelPurge)
		m.undo.slot.cancelPurge = nil
	}
	slog.Debug("queue undo", "kind", m.undo.slot.action.kind())
	m, _ = m.undo.slot.action.perform(m) // undo action may fail silently, user can retry
	m = m.resetAllSorts()
	// Preserve any cancel channel set by perform() (e.g. enqueue undo
	// schedules a purge timer that must be cancellable by the next
	// destructive action).
	pendingCancel := m.undo.slot.cancelPurge
	m.undo.slot = undoSlot{}
	m.undo.slot.cancelPurge = pendingCancel
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
							if err := undoer.DequeueTrackSoft(id); err != nil {
								slog.Warn("dequeue soft-delete failed", "err", err)
							}
						}
					}
				},
				func(before int64) {
					if undoer != nil {
						if err := undoer.PurgeDeletedQueueEntries(before); err != nil {
							slog.Warn("queue purge failed", "err", err)
						}
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
			// If the queue is now empty, or the currently-playing hash is no
			// longer present in the remaining entries, stop playback.
			if len(m.queue.entries) == 0 || (m.playback.currentHash != "" && !queueContainsHash(m.queue.entries, m.playback.currentHash)) {
				m = m.resetPlaybackState()
			}
			m.setUndoSlot(undoBulkDequeueAction{
				entries: removedEntries,
				indices: removedIndices,
				ids:     removedIDs,
			}, cancel)
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
		m.statusBar.notification = newNotificationFor("Press C again to clear queue", queueConfirmationWindow)
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
			m.statusBar.notification = newNotification("Queue is empty — nothing to save")
		} else {
			fromSelection := m.queue.hasSelection()
			m.queue = m.queue.startSaveAsPlaylist(fromSelection)
		}
		return m, nil, true
	}
	return m, nil, false
}
