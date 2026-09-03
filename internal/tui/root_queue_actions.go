package tui

import (
	"fmt"
	"log/slog"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/player"
	"github.com/sglavoie/muse/internal/tui/keys"
	"github.com/sglavoie/muse/internal/tui/queue"
)

// handleUndo processes the UndoMsg by reversing the most recent destructive
// action stored in the undo slot, if one exists and has not expired.
func (m Model) handleUndo() Model {
	if m.undo.slot.action == nil || !time.Now().Before(m.undo.slot.expiresAt) {
		m.statusBar.Notify("Nothing to undo")
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
	m = exitVisualMode(m)
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
	var removedEntries []queue.Entry
	var removedIndices []int
	var removedIDs []int64
	for i, entry := range m.queue.Entries {
		if entry.Selected {
			removedEntries = append(removedEntries, entry)
			removedIndices = append(removedIndices, i)
			removedIDs = append(removedIDs, entry.ID)
		}
	}

	batchDel := m.batchEnqueuer
	undoer := m.undoer
	cancel := m.scheduleSoftDelete(
		func() {
			if batchDel != nil {
				if err := batchDel.DequeueTracksSoft(removedIDs); err != nil {
					slog.Warn("bulk dequeue soft-delete failed", "err", err)
				}
			} else if undoer != nil {
				for _, id := range removedIDs {
					if err := undoer.DequeueTrackSoft(id); err != nil {
						slog.Warn("bulk dequeue soft-delete failed", "err", err)
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

	remaining := make([]queue.Entry, 0, len(m.queue.Entries)-len(removedEntries))
	for _, entry := range m.queue.Entries {
		if !entry.Selected {
			remaining = append(remaining, entry)
		}
	}
	m.queue.Entries = remaining
	m = exitVisualMode(m)
	m = m.preserveQueueSort()
	m.clampQueueCursorAndScroll()
	m.adjustPlaybackIndexAfterRemovals(removedIndices)
	m.relocateCurrentIndexByHash()
	m.syncPlayerQueue()
	m = m.stopPlaybackIfCurrentMissing()
	m.setUndoSlot(undoBulkDequeueAction{
		entries: removedEntries,
		indices: removedIndices,
		ids:     removedIDs,
	}, cancel)
	player.PlaySound(player.SoundRemoved)
	return m, nil, true
}

func (m Model) queuePasteCmd(insertPos int, entries []queue.Entry, cutIDs []int64) tea.Cmd {
	writer := m.queueWriter
	return func() tea.Msg {
		pastedEntries := make([]queue.Entry, 0, len(entries))
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
		return QueuePastedMsg{Entries: pastedEntries, InsertPos: insertPos, CutIDs: cutIDs}
	}
}

// handleQueueKeyAction handles queue-related key actions (select, dequeue,
// clear, move, save-as-playlist, undo). It returns true in the third value
// when the action was handled.
func (m Model) handleQueueKeyAction(action keys.Action) (tea.Model, tea.Cmd, bool) {
	switch action {
	case keys.ActionUndo:
		return m, func() tea.Msg { return UndoMsg{} }, true
	case keys.ActionDequeue:
		if m.queue.HasSelection() {
			return m.handleSelectedQueueDequeueAction()
		}
		var cmd tea.Cmd
		m.queue, cmd = m.queue.HandleAction(action)
		return m, cmd, true
	case keys.ActionClearQueue:
		if len(m.queue.Entries) == 0 {
			return m, nil, true
		}
		var cmd tea.Cmd
		m.queue, cmd = m.queue.HandleAction(action)
		return m, cmd, true
	case keys.ActionSelect:
		var cmd tea.Cmd
		m.queue, cmd = m.queue.HandleAction(action)
		return m, cmd, true
	case keys.ActionMoveTrackUp, keys.ActionMoveTrackDown:
		if m.ActiveView != viewQueue {
			return m, nil, false
		}
		var cmd tea.Cmd
		m.queue, cmd = m.queue.HandleAction(action)
		return m, cmd, true
	case keys.ActionFocusNowPlaying:
		if m.ActiveView != viewQueue {
			m = m.switchToView(viewQueue)
		}
		m.queue = m.queue.FocusNowPlaying(m.playback.currentIndex)
		return m, nil, true
	case keys.ActionYank:
		return m.yankOrCutQueueTracks(false)
	case keys.ActionCut:
		return m.yankOrCutQueueTracks(true)
	case keys.ActionPaste:
		if len(m.yankBuffer) == 0 {
			m.statusBar.Notify("Nothing to paste")
			return m, nil, true
		}
		if m.queue.ReorderBlocked() {
			m.statusBar.Notify("Cannot paste while sorted/filtered")
			return m, nil, true
		}
		if m.queueWriter == nil {
			return m, nil, true
		}
		insertPos := min(m.queue.Cursor+1, len(m.queue.Entries))
		buffer := make([]queue.Entry, len(m.yankBuffer))
		copy(buffer, m.yankBuffer)
		var cutIDs []int64
		if m.yankIsCut {
			cutIDs = make([]int64, len(m.yankBuffer))
			for i, e := range m.yankBuffer {
				cutIDs[i] = e.ID
			}
		}
		return m, m.queuePasteCmd(insertPos, buffer, cutIDs), true
	case keys.ActionSaveQueueAsPlaylist:
		if len(m.queue.Entries) == 0 {
			m.statusBar.Notify("Queue is empty — nothing to save")
		} else {
			fromSelection := m.queue.HasSelection()
			m.queue = m.queue.StartSaveAsPlaylist(fromSelection)
		}
		return m, nil, true
	}
	return m, nil, false
}

// yankOrCutQueueTracks fills yankBuffer from the current selection or cursor
// and clears all Selected flags. isCut=true marks the buffer as a cut (move)
// rather than a copy.
func (m Model) yankOrCutQueueTracks(isCut bool) (tea.Model, tea.Cmd, bool) {
	filtered := m.queue.FilteredEntries()
	if len(filtered) == 0 {
		return m, nil, true
	}
	if m.queue.HasSelection() {
		var buffer []queue.Entry
		for _, entry := range m.queue.Entries {
			if entry.Selected {
				buffer = append(buffer, entry)
			}
		}
		m.yankBuffer = buffer
	} else if m.queue.Cursor >= 0 && m.queue.Cursor < len(filtered) {
		entry := filtered[m.queue.Cursor]
		m.yankBuffer = []queue.Entry{m.queue.Entries[entry.OrigIdx]}
	}
	m.yankIsCut = isCut
	for i := range m.yankBuffer {
		m.yankBuffer[i].Selected = false
	}
	for i := range m.queue.Entries {
		m.queue.Entries[i].Selected = false
	}
	m = exitVisualMode(m)
	if n := len(m.yankBuffer); n > 0 {
		verb := "Yanked"
		if isCut {
			verb = "Cut"
		}
		m.statusBar.Notify(fmt.Sprintf("%s %d track(s)", verb, n))
	}
	return m, nil, true
}
