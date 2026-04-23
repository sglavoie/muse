package tui

import (
	"fmt"
	"log/slog"
	"math/rand/v2"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/player"
	libtui "github.com/sglavoie/muse/internal/tui/library"
)

// handleTrackEnqueuedMsg adds a newly enqueued track to the queue, sets up an
// undo slot, and triggers playback if the queue was empty. When InsertAfter is
// >= 0 the track is inserted at that position ("play next") instead of being
// appended to the end.
func (m Model) handleTrackEnqueuedMsg(msg libtui.TrackEnqueuedMsg) Model {
	if msg.Err != nil {
		m.statusBar.notify("Enqueue track failed: " + msg.Err.Error())
		player.PlaySound(player.SoundError)
		return m
	}
	entry := QueueEntry{
		ID:             msg.QueueID,
		Path:           msg.Track.Path,
		Hash:           msg.Track.Hash,
		Display:        trackDisplay(msg.Track.Artist, msg.Track.Title, msg.Track.Path),
		Artist:         msg.Track.Artist,
		Title:          msg.Track.Title,
		Album:          msg.Track.Album,
		DurationMs:     int64(msg.Track.Duration * 1000),
		SourcePlaylist: msg.SourcePlaylist,
	}
	var enqueueIdx int
	if msg.InsertAfter >= 0 && msg.InsertAfter <= len(m.queue.entries) {
		idx := msg.InsertAfter
		entries := make([]QueueEntry, len(m.queue.entries)+1)
		copy(entries, m.queue.entries[:idx])
		entries[idx] = entry
		copy(entries[idx+1:], m.queue.entries[idx:])
		m.queue.entries = entries
		enqueueIdx = idx
		slog.Debug("track play-next", "hash", msg.Track.Hash, "position", idx)
		if m.playback.currentIndex >= 0 && idx <= m.playback.currentIndex {
			m.playback.currentIndex++
		}
		m.syncPlayerQueue()
	} else {
		m.queue.entries = append(m.queue.entries, entry)
		enqueueIdx = len(m.queue.entries) - 1
		slog.Debug("track enqueued", "hash", msg.Track.Hash, "position", enqueueIdx)
		if appender, ok := m.player.(playerQueueAppender); ok {
			appender.AppendToQueue(queueTrackFromEntry(entry))
		}
	}
	m = m.preserveQueueSort()
	m.setUndoSlot(undoEnqueueAction{
		id:    msg.QueueID,
		entry: m.queue.entries[enqueueIdx],
		idx:   enqueueIdx,
	}, nil)
	player.PlaySound(player.SoundEnqueued)
	if msg.WasEmpty && m.playback.state == PlaybackIdle {
		m = m.tryAutoPlay(msg.Track.Hash, msg.Track.Artist, msg.Track.Title, msg.SourcePlaylist)
	} else if m.playback.queueExhausted && m.playback.state == PlaybackIdle && m.player != nil {
		if err := m.player.PlayFromIndex(enqueueIdx); err == nil {
			m.playback.state = PlaybackPlaying
			m.playback.currentHash = msg.Track.Hash
			m.playback.currentIndex = enqueueIdx
			m.statusBar.artist = msg.Track.Artist
			m.statusBar.title = msg.Track.Title
			m.statusBar.playlistSource = queueSourceName(msg.SourcePlaylist)
		}
		m.playback.queueExhausted = false
	}
	return m
}

// handleBatchEnqueuedMsg adds a batch of enqueued tracks (e.g. from a playlist)
// to the queue and triggers playback if the queue was empty.
func (m Model) handleBatchEnqueuedMsg(msg BatchEnqueuedMsg) Model {
	if msg.Err != nil {
		m.statusBar.notify("Enqueue playlist failed: " + msg.Err.Error())
		player.PlaySound(player.SoundError)
		return m
	}
	if len(msg.Entries) == 0 {
		m.statusBar.notify("Playlist is empty")
		return m
	}
	batchStartIdx := len(m.queue.entries)
	m.queue.entries = append(m.queue.entries, msg.Entries...)
	m = m.preserveQueueSort()
	if appender, ok := m.player.(playerQueueAppender); ok {
		for _, entry := range msg.Entries {
			appender.AppendToQueue(queueTrackFromEntry(entry))
		}
	}
	if msg.WasEmpty && m.playback.state == PlaybackIdle {
		entry := msg.Entries[0]
		m = m.tryAutoPlay(entry.Hash, entry.Artist, entry.Title, msg.PlaylistName)
	} else if m.playback.queueExhausted && m.playback.state == PlaybackIdle && m.player != nil {
		if err := m.player.PlayFromIndex(batchStartIdx); err == nil {
			entry := msg.Entries[0]
			m.playback.state = PlaybackPlaying
			m.playback.currentHash = entry.Hash
			m.playback.currentIndex = batchStartIdx
			m.statusBar.artist = entry.Artist
			m.statusBar.title = entry.Title
			m.statusBar.playlistSource = queueSourceName(msg.PlaylistName)
		}
		m.playback.queueExhausted = false
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
		m.statusBar.notify("Play failed: " + err.Error())
		player.PlaySound(player.SoundError)
		return m
	}
	m.playback.state = PlaybackPlaying
	m.playback.currentHash = hash
	m.playback.currentIndex = 0
	m.statusBar.artist = artist
	m.statusBar.title = title
	m.statusBar.playlistSource = queueSourceName(source)
	return m
}

// handlePlayQueueIndexMsg starts playback from a specific queue index.
// The actual player.PlayFromIndex call runs in a goroutine (via tea.Cmd)
// so that synchronous mpv IPC, DB writes, and tea.Program.Send() from
// inside the player never block the Update goroutine. A successful call
// is followed up by a TrackChangedMsg from the player's start-file
// event; a failure surfaces as playFromIndexResultMsg so the TUI can
// recover (including removal when the file disappeared mid-flight).
func (m Model) handlePlayQueueIndexMsg(msg PlayQueueIndexMsg) (Model, tea.Cmd) {
	if m.player == nil {
		return m, nil
	}
	if msg.Index < 0 || msg.Index >= len(m.queue.entries) {
		return m, nil
	}
	entry := m.queue.entries[msg.Index]
	// Optimistic UI update: show the track as playing immediately. The
	// player will either confirm via TrackChangedMsg or we'll reset
	// state from the result handler if PlayFromIndex fails.
	m.playback.state = PlaybackPlaying
	m.playback.currentHash = entry.Hash
	m.playback.currentIndex = msg.Index
	m.statusBar.artist = entry.Artist
	m.statusBar.title = entry.Title
	m.statusBar.playlistSource = queueSourceName(entry.SourcePlaylist)

	p := m.player
	idx := msg.Index
	path := entry.Path
	hash := entry.Hash
	return m, func() tea.Msg {
		err := p.PlayFromIndex(idx)
		return playFromIndexResultMsg{Index: idx, Path: path, Hash: hash, Err: err}
	}
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
		for i := range m.queue.entries {
			m.queue.entries[i].Selected = false
		}
		m = exitVisualMode(m)
		m = m.preserveQueueSort()
		m.clampQueueCursorAndScroll()
		if remover, ok := m.player.(playerQueueRemover); ok {
			_ = remover.RemoveFromQueue(msg.Index) // player queue sync is best-effort, TUI is source of truth
		}
		wasCurrent := msg.Index == m.playback.currentIndex
		m.adjustPlaybackIndexAfterRemoval(msg.Index)
		if wasCurrent && len(m.queue.entries) > 0 {
			// The player's in-flight gapless prefetch will carry playback to
			// the next track. Do NOT call resetPlaybackState here — it would
			// invoke player.Stop() which globally pauses mpv, and that
			// paused state persists across subsequent gapless transitions,
			// causing every next track to start paused until relaunch.
			// Clear only the TUI bookkeeping; start-file will repopulate it.
			m.playback.currentHash = ""
			m.statusBar.artist = ""
			m.statusBar.title = ""
			m.statusBar.playlistSource = ""
		} else {
			m = m.stopPlaybackIfCurrentMissing()
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
	for i, entry := range savedEntries {
		savedTracks[i] = queueTrackFromEntry(entry)
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
	m = m.resetQueueState()
	m = m.resetPlaybackState()
	if setter, ok := m.player.(playerQueueSetter); ok {
		setter.SetQueue(nil)
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
	if m.undo.slot.cancelPurge != nil {
		close(m.undo.slot.cancelPurge)
	}
	m.undo.slot = undoSlot{}
	m = m.resetQueueState()
	m = m.resetPlaybackState()
	if setter, ok := m.player.(playerQueueSetter); ok {
		setter.SetQueue(nil)
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
		switch m.playback.currentIndex {
		case msg.Index:
			m.playback.currentIndex = msg.Index - 1
		case msg.Index - 1:
			m.playback.currentIndex = msg.Index
		}
		if m.reorderer != nil {
			reorderer := m.reorderer
			id := entry.ID
			return m, func() tea.Msg {
				if err := reorderer.ReorderQueueTrack(id, -1); err != nil {
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
		switch m.playback.currentIndex {
		case msg.Index:
			m.playback.currentIndex = msg.Index + 1
		case msg.Index + 1:
			m.playback.currentIndex = msg.Index
		}
		if m.reorderer != nil {
			reorderer := m.reorderer
			id := entry.ID
			return m, func() tea.Msg {
				if err := reorderer.ReorderQueueTrack(id, +1); err != nil {
					slog.Error("reorder queue track failed", "id", id, "direction", +1, "err", err)
				}
				return nil
			}
		}
	}
	return m, nil
}

// handleQueuePastedMsg splices pasted entries into the queue, syncs the player,
// and adjusts the playback index.
func (m Model) handleQueuePastedMsg(msg QueuePastedMsg) Model {
	if msg.Err != nil {
		m.statusBar.notify("Paste failed: " + msg.Err.Error())
		player.PlaySound(player.SoundError)
		return m
	}
	if len(msg.Entries) == 0 {
		return m
	}
	insertPos := min(msg.InsertPos, len(m.queue.entries))
	newEntries := make([]QueueEntry, 0, len(m.queue.entries)+len(msg.Entries))
	newEntries = append(newEntries, m.queue.entries[:insertPos]...)
	newEntries = append(newEntries, msg.Entries...)
	newEntries = append(newEntries, m.queue.entries[insertPos:]...)
	m.queue.entries = newEntries
	m = m.preserveQueueSort()
	if m.playback.currentIndex >= 0 && insertPos <= m.playback.currentIndex {
		m.playback.currentIndex += len(msg.Entries)
	}
	m.syncPlayerQueue()

	// If the queue was exhausted and we just pasted tracks, resume playback
	// from the first pasted entry.
	if m.playback.queueExhausted && m.playback.state == PlaybackIdle && m.player != nil {
		if err := m.player.PlayFromIndex(insertPos); err == nil {
			entry := msg.Entries[0]
			m.playback.state = PlaybackPlaying
			m.playback.currentHash = entry.Hash
			m.playback.currentIndex = insertPos
			m.playback.elapsedSecs = 0
			m.playback.totalSecs = 0
			m.statusBar.artist = entry.Artist
			m.statusBar.title = entry.Title
			m.statusBar.playlistSource = queueSourceName(entry.SourcePlaylist)
		}
		m.playback.queueExhausted = false
	}

	// If this is a cut-paste, remove the original entries.
	var removedOriginals []QueueEntry
	var removedOriginalIndices []int
	if len(msg.CutIDs) > 0 {
		cutIDSet := make(map[int64]struct{}, len(msg.CutIDs))
		for _, id := range msg.CutIDs {
			cutIDSet[id] = struct{}{}
		}
		// Freshly pasted entries have new IDs, so they won't match cutIDs.
		var kept []QueueEntry
		for i, e := range m.queue.entries {
			if _, isCut := cutIDSet[e.ID]; isCut {
				removedOriginals = append(removedOriginals, e)
				removedOriginalIndices = append(removedOriginalIndices, i)
				continue
			}
			kept = append(kept, e)
		}
		m.queue.entries = kept
		if m.batchEnqueuer != nil {
			if err := m.batchEnqueuer.DequeueTracksSoft(msg.CutIDs); err != nil {
				slog.Warn("cut-paste: soft-delete originals failed", "err", err)
			}
			if m.undoer != nil {
				schedulePurge(&m.undo.slot, m.undoer.PurgeDeletedQueueEntries)
			}
		}
		// Adjust playback index for removed originals.
		if m.playback.currentIndex >= 0 && len(removedOriginalIndices) > 0 {
			removedBefore := 0
			removedCurrent := false
			for _, ri := range removedOriginalIndices {
				if ri < m.playback.currentIndex {
					removedBefore++
				} else if ri == m.playback.currentIndex {
					removedCurrent = true
				}
			}
			if removedCurrent {
				m.playback.currentIndex = -1
			} else {
				m.playback.currentIndex -= removedBefore
			}
		}
		if qr, ok := m.player.(playerQueueRemover); ok {
			for i := len(removedOriginalIndices) - 1; i >= 0; i-- {
				_ = qr.RemoveFromQueue(removedOriginalIndices[i])
			}
		}
		m.syncPlayerQueue()
		m.yankIsCut = false
	}

	m.queue.cursor = insertPos + len(msg.Entries) - 1
	// Adjust cursor for removed entries that were before the cursor.
	if len(removedOriginalIndices) > 0 {
		removedBeforeCursor := 0
		for _, ri := range removedOriginalIndices {
			if ri < m.queue.cursor {
				removedBeforeCursor++
			}
		}
		m.queue.cursor -= removedBeforeCursor
		if m.queue.cursor < 0 {
			m.queue.cursor = 0
		}
	}
	maxRows := m.queue.visibleRows()
	if maxRows > 0 && m.queue.cursor >= m.queue.scrollOffset+maxRows {
		m.queue.scrollOffset = m.queue.cursor - maxRows + 1
	}
	m.clampQueueCursorAndScroll()
	pastedIDs := make([]int64, len(msg.Entries))
	for i, entry := range msg.Entries {
		pastedIDs[i] = entry.ID
	}
	if len(msg.CutIDs) > 0 {
		// Convert post-insert indices back to pre-paste positions so that
		// undo re-inserts originals at the correct spots. Pasted entries
		// occupied [insertPos, insertPos+pasteCount), so any original
		// index >= insertPos+pasteCount was shifted right by pasteCount.
		prePasteIndices := make([]int, len(removedOriginalIndices))
		pasteCount := len(msg.Entries)
		for i, ri := range removedOriginalIndices {
			if ri >= insertPos+pasteCount {
				prePasteIndices[i] = ri - pasteCount
			} else {
				prePasteIndices[i] = ri
			}
		}
		// Preserve the cancel channel set by schedulePurge above so that
		// setUndoSlot does not immediately cancel the pending purge of the
		// soft-deleted originals.
		cutPurgeCancel := m.undo.slot.cancelPurge
		m.undo.slot.cancelPurge = nil // prevent setUndoSlot from closing it
		m.setUndoSlot(undoCutPasteAction{
			pastedIDs:       pastedIDs,
			originalEntries: removedOriginals,
			originalIndices: prePasteIndices,
			count:           len(msg.Entries),
		}, cutPurgeCancel)
	} else {
		m.setUndoSlot(undoPasteAction{
			ids:   pastedIDs,
			count: len(msg.Entries),
		}, nil)
	}
	player.PlaySound(player.SoundEnqueued)
	return m
}

// handleQueueShuffleMsg applies a Fisher-Yates shuffle to the in-memory queue,
// updates the current-playing index, syncs the player queue, and persists the
// new order to the database when a queueShuffler is available.
func (m Model) handleQueueShuffleMsg() (Model, tea.Cmd) {
	if len(m.queue.entries) <= 1 {
		m.statusBar.notify("Queue too small to shuffle")
		return m, nil
	}

	entries := make([]QueueEntry, len(m.queue.entries))
	copy(entries, m.queue.entries)
	for i := len(entries) - 1; i > 0; i-- {
		j := rand.IntN(i + 1) //nolint:gosec // non-security random for queue shuffle
		entries[i], entries[j] = entries[j], entries[i]
	}
	m.queue.entries = entries
	for i := range m.queue.entries {
		m.queue.entries[i].Selected = false
	}
	m = exitVisualMode(m)

	m.playback.currentIndex = -1
	if m.playback.currentHash != "" {
		for i, entry := range m.queue.entries {
			if entry.Hash == m.playback.currentHash {
				m.playback.currentIndex = i
				break
			}
		}
	}

	m = m.resetQueueSort()
	m.syncPlayerQueue()
	m.statusBar.notify(fmt.Sprintf("Queue shuffled (%d tracks)", len(m.queue.entries)))

	if shuffler, ok := m.reorderer.(queueShuffler); ok {
		ids := make([]int64, len(m.queue.entries))
		for i, entry := range m.queue.entries {
			ids[i] = entry.ID
		}
		return m, func() tea.Msg { _ = shuffler.SetQueueOrder(ids); return nil }
	}
	return m, nil
}

// handleQueueDedupMsg removes duplicate queue entries, keeping the first
// occurrence of each track hash. Removed entries are soft-deleted with undo
// support.
func (m Model) handleQueueDedupMsg() (Model, tea.Cmd) {
	seen := make(map[string]struct{})
	var keep []QueueEntry
	var removedEntries []QueueEntry
	var removedIndices []int
	var removedIDs []int64

	for i, entry := range m.queue.entries {
		if _, ok := seen[entry.Hash]; ok {
			removedEntries = append(removedEntries, entry)
			removedIndices = append(removedIndices, i)
			removedIDs = append(removedIDs, entry.ID)
		} else {
			seen[entry.Hash] = struct{}{}
			keep = append(keep, entry)
		}
	}

	if len(removedIDs) == 0 {
		m.statusBar.notify("No duplicates found")
		return m, nil
	}

	m.queue.entries = keep
	for i := range m.queue.entries {
		m.queue.entries[i].Selected = false
	}
	m = exitVisualMode(m)
	m = m.preserveQueueSort()
	m.clampQueueCursorAndScroll()
	m.syncPlayerQueue()
	m.adjustPlaybackIndexAfterRemovals(removedIndices)
	m = m.stopPlaybackIfCurrentMissing()

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
	m.setUndoSlot(undoBulkDequeueAction{
		entries: removedEntries,
		indices: removedIndices,
		ids:     removedIDs,
	}, cancel)
	player.PlaySound(player.SoundRemoved)
	return m, nil
}
