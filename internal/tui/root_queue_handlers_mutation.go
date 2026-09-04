package tui

import (
	"log/slog"
	"time"

	"github.com/sglavoie/muse/internal/player"
	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/queue"
)

// handleDequeueTrackMsg removes a track from the queue with undo support.
func (m Model) handleDequeueTrackMsg(msg queue.DequeueTrackMsg) Model {
	if msg.Index >= 0 && msg.Index < len(m.queue.Entries) {
		entry := m.queue.Entries[msg.Index]
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
		m.queue.Entries = append(m.queue.Entries[:msg.Index], m.queue.Entries[msg.Index+1:]...)
		for i := range m.queue.Entries {
			m.queue.Entries[i].Selected = false
		}
		m = exitVisualMode(m)
		m = m.preserveQueueSort()
		m.clampQueueCursorAndScroll()
		if remover, ok := m.player.(playerQueueRemover); ok {
			_ = remover.RemoveFromQueue(msg.Index) // player queue sync is best-effort, TUI is source of truth
		}
		wasCurrent := msg.Index == m.playback.currentIndex
		m.adjustPlaybackIndexAfterRemoval(msg.Index)
		if wasCurrent && len(m.queue.Entries) > 0 {
			m = m.endCurrentListen()
			// The player's in-flight gapless prefetch will carry playback to
			// the next track. Do NOT call resetPlaybackState here — it would
			// invoke player.Stop() which globally pauses mpv, and that
			// paused state persists across subsequent gapless transitions,
			// causing every next track to start paused until relaunch.
			// Clear only the TUI bookkeeping; start-file will repopulate it.
			// That means the track-event barrier has to come down too: it admits
			// reports matching currentHash, so leaving it up would refuse the very
			// start-file this branch is relying on.
			m.playback.currentHash = ""
			m.playback.lowerSelectionBarrier()
			m.statusBar.Artist = ""
			m.statusBar.Title = ""
			m.statusBar.PlaylistSource = ""
		} else {
			m = m.stopPlaybackIfCurrentMissing()
		}
	}
	m.queueDirty = true
	return m
}

// handleBulkSelectionToggleMsg toggles the selection state of a queue entry.
func (m Model) handleBulkSelectionToggleMsg(msg queue.BulkSelectionToggleMsg) Model {
	if msg.Index >= 0 && msg.Index < len(m.queue.Entries) {
		m.queue.Entries[msg.Index].Selected = !m.queue.Entries[msg.Index].Selected
	}
	return m
}

// handleClearQueueMsg clears the entire queue with undo support, stopping
// active playback.
func (m Model) handleClearQueueMsg() Model {
	savedEntries := make([]queue.Entry, len(m.queue.Entries))
	copy(savedEntries, m.queue.Entries)
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
	// Exit visual mode: clearing the queue removes every entry the visual
	// snapshot referenced; leaving visual.active=true with stale indices
	// would corrupt the next visual selection after a re-enqueue.
	m = exitVisualMode(m)
	// Reset the radio enqueue-pending flag so the next track change can fire
	// a fresh radio batch. The in-flight batch (if any) is discarded by the
	// queue-cleared guard in handleRadioBatchEnqueuedMsg.
	m.radio.enqueuePending = false
	if setter, ok := m.player.(playerQueueSetter); ok {
		setter.SetQueue(nil)
	}
	m.queueDirty = false
	return m
}

// clearQueueForReplace silently clears the queue and stops playback so that a
// subsequent enqueue becomes the only entry. Unlike handleClearQueueMsg this
// does not set up an undo slot — the enqueue that follows will create its own.
func (m Model) clearQueueForReplace() Model {
	if len(m.queue.Entries) > 0 && m.undoer != nil {
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
	m.queueDirty = false
	return m
}

// handleQueuePastedMsg splices pasted entries into the queue, syncs the player,
// and adjusts the playback index.
func (m Model) handleQueuePastedMsg(msg QueuePastedMsg) Model {
	if msg.Err != nil {
		m.statusBar.Notify("Paste failed: " + msg.Err.Error())
		player.PlaySound(player.SoundError)
		return m
	}
	if len(msg.Entries) == 0 {
		return m
	}
	insertPos := min(msg.InsertPos, len(m.queue.Entries))
	newEntries := make([]queue.Entry, 0, len(m.queue.Entries)+len(msg.Entries))
	newEntries = append(newEntries, m.queue.Entries[:insertPos]...)
	newEntries = append(newEntries, msg.Entries...)
	newEntries = append(newEntries, m.queue.Entries[insertPos:]...)
	m.queue.Entries = newEntries
	m = m.preserveQueueSort()
	if m.playback.currentIndex >= 0 && insertPos <= m.playback.currentIndex {
		m.playback.currentIndex += len(msg.Entries)
	}
	m.syncPlayerQueue()
	m = m.resumeExhaustedPlayback(insertPos)

	var removedOriginals []queue.Entry
	var removedOriginalIndices []int
	if len(msg.CutIDs) > 0 {
		removedOriginals, removedOriginalIndices = m.removeCutOriginals(msg.CutIDs)
		// removeCutOriginals shrinks m.queue.entries, invalidating the
		// displayOrder computed by preserveQueueSort above. Recompute so
		// the sorted view stays consistent instead of silently dropping
		// back to insertion order.
		m = m.preserveQueueSort()
	}

	m.queue.Cursor = insertPos + len(msg.Entries) - 1
	m = m.adjustCursorForRemovedEntries(removedOriginalIndices)
	m.clampQueueCursorAndScroll()

	pastedIDs := make([]int64, len(msg.Entries))
	for i, entry := range msg.Entries {
		pastedIDs[i] = entry.ID
	}
	m.setupPasteUndo(pastedIDs, msg.Entries, msg.CutIDs, removedOriginals, removedOriginalIndices, insertPos)
	m.persistQueueOrder()
	m.queueDirty = true
	player.PlaySound(player.SoundEnqueued)
	return m
}

// persistQueueOrder writes the TUI queue order back to storage when the
// configured queue dependency supports full-order persistence. Paste creates
// rows through AppendTrack, which stores them at the DB tail; this sync keeps
// cursor-position inserts durable across restarts.
func (m Model) persistQueueOrder() {
	shuffler, ok := m.reorderer.(queueShuffler)
	if !ok {
		return
	}
	ids := make([]int64, 0, len(m.queue.Entries))
	for _, entry := range m.queue.Entries {
		if entry.ID != 0 {
			ids = append(ids, entry.ID)
		}
	}
	if err := shuffler.SetQueueOrder(ids); err != nil {
		slog.Warn("queue order sync failed", "err", err)
	}
}

// resumeExhaustedPlayback continues the traversal from the pasted rows when the
// queue was previously exhausted. insertPos only anchors players without the
// resume extension; otherwise the traversal picks the row itself.
func (m Model) resumeExhaustedPlayback(insertPos int) Model {
	if !m.playback.queueExhausted || m.playback.state != playback.StateIdle {
		return m
	}
	m, _ = m.resumeExtendedQueue(insertPos)
	return m
}

// removeCutOriginals removes the original cut entries from the queue,
// soft-deletes them in the DB, adjusts the playback index, and syncs the
// player. It returns the removed entries and their indices.
func (m *Model) removeCutOriginals(cutIDs []int64) ([]queue.Entry, []int) {
	cutIDSet := make(map[int64]struct{}, len(cutIDs))
	for _, id := range cutIDs {
		cutIDSet[id] = struct{}{}
	}
	// Freshly pasted entries have new IDs, so they won't match cutIDs.
	var kept, removed []queue.Entry
	var removedIndices []int
	for i, e := range m.queue.Entries {
		if _, isCut := cutIDSet[e.ID]; isCut {
			removed = append(removed, e)
			removedIndices = append(removedIndices, i)
			continue
		}
		kept = append(kept, e)
	}
	m.queue.Entries = kept
	if m.batchEnqueuer != nil {
		if err := m.batchEnqueuer.DequeueTracksSoft(cutIDs); err != nil {
			slog.Warn("cut-paste: soft-delete originals failed", "err", err)
		}
		if m.undoer != nil {
			schedulePurge(&m.undo.slot, m.undoer.PurgeDeletedQueueEntries)
		}
	}
	m.adjustPlaybackIndexAfterRemovals(removedIndices)
	m.relocateCurrentIndexByHash()
	if qr, ok := m.player.(playerQueueRemover); ok {
		for i := len(removedIndices) - 1; i >= 0; i-- {
			_ = qr.RemoveFromQueue(removedIndices[i])
		}
	}
	m.syncPlayerQueue()
	m.yankIsCut = false
	return removed, removedIndices
}

// adjustCursorForRemovedEntries shifts the queue cursor to account for entries
// that were removed before it, and ensures scroll visibility.
func (m Model) adjustCursorForRemovedEntries(removedIndices []int) Model {
	if len(removedIndices) > 0 {
		removedBeforeCursor := 0
		for _, ri := range removedIndices {
			if ri < m.queue.Cursor {
				removedBeforeCursor++
			}
		}
		m.queue.Cursor -= removedBeforeCursor
		if m.queue.Cursor < 0 {
			m.queue.Cursor = 0
		}
	}
	maxRows := m.queue.VisibleRows()
	if maxRows > 0 && m.queue.Cursor >= m.queue.ScrollOffset+maxRows {
		m.queue.ScrollOffset = m.queue.Cursor - maxRows + 1
	}
	return m
}

// setupPasteUndo configures the undo slot for a paste or cut-paste operation.
func (m *Model) setupPasteUndo(pastedIDs []int64, pastedEntries []queue.Entry, cutIDs []int64, removedOriginals []queue.Entry, removedOriginalIndices []int, insertPos int) {
	if len(cutIDs) == 0 {
		m.setUndoSlot(undoPasteAction{
			ids:   pastedIDs,
			count: len(pastedEntries),
		}, nil)
		return
	}
	// Convert post-insert indices back to pre-paste positions so that
	// undo re-inserts originals at the correct spots. Pasted entries
	// occupied [insertPos, insertPos+pasteCount), so any original
	// index >= insertPos+pasteCount was shifted right by pasteCount.
	prePasteIndices := make([]int, len(removedOriginalIndices))
	pasteCount := len(pastedEntries)
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
		count:           len(pastedEntries),
	}, cutPurgeCancel)
}
