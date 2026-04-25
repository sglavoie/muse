package tui

import (
	"log/slog"
	"time"

	"github.com/sglavoie/muse/internal/player"
)

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
	// Collect entry IDs before scheduling the goroutine so the undo action
	// receives the correct slice immediately. Previously clearIDs was captured
	// after scheduleSoftDelete returned but before the goroutine ran, resulting
	// in a nil slice that made UndoClearQueue a no-op.
	entryIDs := make([]int64, len(savedEntries))
	for i, e := range savedEntries {
		entryIDs[i] = e.ID
	}
	undoer := m.undoer
	cancel := m.scheduleSoftDelete(
		func() {
			if undoer != nil {
				if _, err := undoer.ClearQueueSoft(); err != nil {
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
		ids:        entryIDs,
		entries:    savedEntries,
		tracks:     savedTracks,
		savedIndex: m.playback.currentIndex,
		savedHash:  m.playback.currentHash,
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
			m.playback.peakElapsedSecs = 0
			m.playback.peakTotalSecs = 0
			m.playback.durationCorrected = false
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
