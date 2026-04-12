package tui

import (
	"fmt"
	"log/slog"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/player"
)

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

func (m Model) handleSelectedQueueDequeueAction() (tea.Model, tea.Cmd, bool) {
	var removedEntries []QueueEntry
	var removedIndices []int
	var removedIDs []int64
	for i, entry := range m.queue.entries {
		if entry.Selected {
			removedEntries = append(removedEntries, entry)
			removedIndices = append(removedIndices, i)
			removedIDs = append(removedIDs, entry.ID)
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

	remaining := make([]QueueEntry, 0, len(m.queue.entries)-len(removedEntries))
	for _, entry := range m.queue.entries {
		if !entry.Selected {
			remaining = append(remaining, entry)
		}
	}
	m.queue.entries = remaining
	m = m.preserveQueueSort()
	m.clampQueueCursorAndScroll()
	m.syncPlayerQueue()
	m.adjustPlaybackIndexAfterRemovals(removedIndices)
	m = m.stopPlaybackIfCurrentMissing()
	m.setUndoSlot(undoBulkDequeueAction{
		entries: removedEntries,
		indices: removedIndices,
		ids:     removedIDs,
	}, cancel)
	player.PlaySound(player.SoundRemoved)
	return m, nil, true
}

func (m Model) queuePasteCmd(insertPos int, entries []QueueEntry) tea.Cmd {
	writer := m.queueWriter
	return func() tea.Msg {
		pastedEntries := make([]QueueEntry, 0, len(entries))
		for _, entry := range entries {
			track := library.Track{
				Path:     entry.Path,
				Hash:     entry.Hash,
				Artist:   entry.Artist,
				Title:    entry.Title,
				Album:    entry.Album,
				Duration: float64(entry.DurationMs) / 1000,
			}
			id, err := writer.AppendTrack(track, entry.SourcePlaylist)
			if err != nil {
				return QueuePastedMsg{Err: err}
			}
			entry.ID = id
			pastedEntries = append(pastedEntries, entry)
		}
		return QueuePastedMsg{Entries: pastedEntries, InsertPos: insertPos}
	}
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
			return m.handleSelectedQueueDequeueAction()
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
		m.queue = m.queue.FocusNowPlaying(m.playback.currentIndex)
		return m, nil, true
	case actionYank:
		filtered := m.queue.filteredEntries()
		if len(filtered) == 0 {
			return m, nil, true
		}
		if m.queue.hasSelection() {
			var buffer []QueueEntry
			for _, entry := range m.queue.entries {
				if entry.Selected {
					buffer = append(buffer, entry)
				}
			}
			m.yankBuffer = buffer
		} else if m.queue.cursor >= 0 && m.queue.cursor < len(filtered) {
			entry := filtered[m.queue.cursor]
			m.yankBuffer = []QueueEntry{m.queue.entries[entry.origIdx]}
		}
		if n := len(m.yankBuffer); n > 0 {
			m.statusBar.notification = newNotification(fmt.Sprintf("Yanked %d track(s)", n))
		}
		return m, nil, true
	case actionPaste:
		if len(m.yankBuffer) == 0 {
			m.statusBar.notification = newNotification("Nothing to paste")
			return m, nil, true
		}
		if m.queue.displayOrder != nil || m.queue.filter.Text != "" {
			m.statusBar.notification = newNotification("Cannot paste while sorted/filtered")
			return m, nil, true
		}
		if m.queueWriter == nil {
			return m, nil, true
		}
		insertPos := min(m.queue.cursor+1, len(m.queue.entries))
		buffer := make([]QueueEntry, len(m.yankBuffer))
		copy(buffer, m.yankBuffer)
		return m, m.queuePasteCmd(insertPos, buffer), true
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
