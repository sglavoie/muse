package tui

import (
	"fmt"
	"log/slog"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/player"
)

// undoEnqueueAction reverts a single-track enqueue (undoKindEnqueue).
type undoEnqueueAction struct {
	id    int64
	entry QueueEntry
	idx   int
}

func (a undoEnqueueAction) kind() undoKind { return undoKindEnqueue }

func (a undoEnqueueAction) description() string {
	return "Added " + a.entry.Display + " — press u to undo"
}

func (a undoEnqueueAction) perform(m Model) (Model, tea.Cmd) {
	// Find the enqueued entry by its database ID rather than by positional
	// index, because the queue may have been reordered since the enqueue.
	realIdx := -1
	for i, e := range m.queue.entries {
		if e.ID == a.id {
			realIdx = i
			break
		}
	}
	if realIdx >= 0 {
		if m.undoer != nil {
			if err := m.undoer.DequeueTrackSoft(a.id); err != nil {
				slog.Warn("undo enqueue: soft-delete failed", "err", err)
			}
			schedulePurge(&m.undo.slot, m.undoer.PurgeDeletedQueueEntries)
		}
		m.queue.entries = append(m.queue.entries[:realIdx], m.queue.entries[realIdx+1:]...)
		m.clampQueueCursorAndScroll()
		if qr, ok := m.player.(playerQueueRemover); ok {
			_ = qr.RemoveFromQueue(realIdx) // player queue sync during undo, best-effort
		}
		if realIdx < m.playback.currentIndex {
			m.playback.currentIndex--
		} else if realIdx == m.playback.currentIndex {
			m.playback.currentIndex = -1
		}
		if len(m.queue.entries) == 0 || (m.playback.currentHash != "" && !queueContainsHash(m.queue.entries, m.playback.currentHash)) {
			m = m.resetPlaybackState()
		}
	}
	m.statusBar.notification = newNotification("Undone: removed " + a.entry.Display)
	return m, nil
}

// undoDequeueAction reverts a single-track dequeue (undoKindDequeue).
type undoDequeueAction struct {
	id    int64
	entry QueueEntry
	idx   int
}

func (a undoDequeueAction) kind() undoKind { return undoKindDequeue }

func (a undoDequeueAction) description() string {
	return "Track removed — press u to undo"
}

func (a undoDequeueAction) perform(m Model) (Model, tea.Cmd) {
	if m.undoer != nil {
		if err := m.undoer.UndoDequeue(a.id); err != nil {
			slog.Warn("undo dequeue failed", "err", err)
		}
	}
	idx := min(a.idx, len(m.queue.entries))
	entries := make([]QueueEntry, len(m.queue.entries)+1)
	copy(entries, m.queue.entries[:idx])
	entries[idx] = a.entry
	copy(entries[idx+1:], m.queue.entries[idx:])
	m.queue.entries = entries
	if m.playback.currentIndex >= 0 && idx <= m.playback.currentIndex {
		m.playback.currentIndex++
	}
	m.queue.cursor = idx
	n := len(m.queue.filteredEntries())
	if n == 0 {
		m.queue.cursor = 0
		m.queue.scrollOffset = 0
	} else if m.queue.cursor >= n {
		m.queue.cursor = n - 1
	}
	if m.queue.cursor < m.queue.scrollOffset {
		m.queue.scrollOffset = m.queue.cursor
	} else {
		maxRows := m.queue.visibleRows()
		if m.queue.cursor >= m.queue.scrollOffset+maxRows {
			m.queue.scrollOffset = m.queue.cursor - maxRows + 1
		}
	}
	m.syncPlayerQueue()
	m.statusBar.notification = newNotification("Undone: restored removed track")
	return m, nil
}

// undoClearAction reverts a full queue clear (undoKindClear).
type undoClearAction struct {
	ids     []int64
	entries []QueueEntry
	tracks  []player.QueueTrack
}

func (a undoClearAction) kind() undoKind { return undoKindClear }

func (a undoClearAction) description() string {
	return "Queue cleared — press u to undo"
}

func (a undoClearAction) perform(m Model) (Model, tea.Cmd) {
	if m.undoer != nil {
		if err := m.undoer.UndoClearQueue(a.ids); err != nil {
			slog.Warn("undo clear queue failed", "err", err)
		}
	}
	m.queue.entries = a.entries
	if qs, ok := m.player.(playerQueueSetter); ok {
		qs.SetQueue(a.tracks)
	}
	n := len(a.entries)
	m.statusBar.notification = newNotification(fmt.Sprintf("Undone: restored %d %s", n, pluralize(n, "track", "tracks")))
	return m, nil
}

// undoBulkDequeueAction reverts removal of multiple selected queue entries
// (undoKindBulkDequeue).
type undoBulkDequeueAction struct {
	entries []QueueEntry
	indices []int
	ids     []int64
}

func (a undoBulkDequeueAction) kind() undoKind { return undoKindBulkDequeue }

func (a undoBulkDequeueAction) description() string {
	n := len(a.entries)
	return fmt.Sprintf("%d %s removed — press u to undo", n, pluralize(n, "track", "tracks"))
}

func (a undoBulkDequeueAction) perform(m Model) (Model, tea.Cmd) {
	if m.undoer != nil {
		for _, id := range a.ids {
			if err := m.undoer.UndoDequeue(id); err != nil {
				slog.Warn("undo bulk dequeue failed", "err", err)
			}
		}
	}
	for i := range a.indices {
		idx := min(a.indices[i], len(m.queue.entries))
		entries := make([]QueueEntry, len(m.queue.entries)+1)
		copy(entries, m.queue.entries[:idx])
		entries[idx] = a.entries[i]
		copy(entries[idx+1:], m.queue.entries[idx:])
		m.queue.entries = entries
		if m.playback.currentIndex >= 0 && idx <= m.playback.currentIndex {
			m.playback.currentIndex++
		}
	}
	m.clampQueueCursorAndScroll()
	m.syncPlayerQueue()
	n := len(a.entries)
	m.statusBar.notification = newNotification(fmt.Sprintf("Undone: restored %d %s", n, pluralize(n, "track", "tracks")))
	return m, nil
}

// undoBatchEnqueueAction reverts a batch enqueue from a playlist
// (undoKindBatchEnqueue).
type undoBatchEnqueueAction struct {
	ids          []int64
	count        int
	startIdx     int
	playlistName string
}

func (a undoBatchEnqueueAction) kind() undoKind { return undoKindBatchEnqueue }

func (a undoBatchEnqueueAction) description() string {
	return fmt.Sprintf("Added %d %s from %s — press u to undo",
		a.count, pluralize(a.count, "track", "tracks"), a.playlistName)
}

func (a undoBatchEnqueueAction) perform(m Model) (Model, tea.Cmd) {
	if m.batchEnqueuer != nil {
		if err := m.batchEnqueuer.DequeueTracksSoft(a.ids); err != nil {
			slog.Warn("undo batch enqueue: soft-delete failed", "err", err)
		}
		if m.undoer != nil {
			schedulePurge(&m.undo.slot, m.undoer.PurgeDeletedQueueEntries)
		}
	}
	idSet := make(map[int64]struct{}, len(a.ids))
	for _, id := range a.ids {
		idSet[id] = struct{}{}
	}
	var kept []QueueEntry
	var removedPlayerIndices []int
	for i, e := range m.queue.entries {
		if _, ok := idSet[e.ID]; ok {
			removedPlayerIndices = append(removedPlayerIndices, i)
		} else {
			kept = append(kept, e)
		}
	}
	m.queue.entries = kept
	m.clampQueueCursorAndScroll()
	if m.playback.currentIndex >= 0 {
		removedBefore := 0
		removedCurrent := false
		for _, ri := range removedPlayerIndices {
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
		for i := len(removedPlayerIndices) - 1; i >= 0; i-- {
			_ = qr.RemoveFromQueue(removedPlayerIndices[i])
		}
	}
	if len(m.queue.entries) == 0 || (m.playback.currentHash != "" && !queueContainsHash(m.queue.entries, m.playback.currentHash)) {
		m = m.resetPlaybackState()
	}
	n := a.count
	m.statusBar.notification = newNotification(fmt.Sprintf("Undone: removed %d %s", n, pluralize(n, "track", "tracks")))
	return m, nil
}

// undoPasteAction reverts a queue paste by removing the pasted entries
// (undoKindPaste). It mirrors undoBatchEnqueueAction: soft-deletes the pasted
// IDs and removes matching entries from the in-memory queue.
type undoPasteAction struct {
	ids   []int64
	count int
}

func (a undoPasteAction) kind() undoKind { return undoKindPaste }

func (a undoPasteAction) description() string {
	return fmt.Sprintf("Pasted %d %s — press u to undo", a.count, pluralize(a.count, "track", "tracks"))
}

func (a undoPasteAction) perform(m Model) (Model, tea.Cmd) {
	if m.batchEnqueuer != nil {
		if err := m.batchEnqueuer.DequeueTracksSoft(a.ids); err != nil {
			slog.Warn("undo paste: soft-delete failed", "err", err)
		}
		if m.undoer != nil {
			schedulePurge(&m.undo.slot, m.undoer.PurgeDeletedQueueEntries)
		}
	}
	idSet := make(map[int64]struct{}, len(a.ids))
	for _, id := range a.ids {
		idSet[id] = struct{}{}
	}
	var kept []QueueEntry
	var removedPlayerIndices []int
	for i, e := range m.queue.entries {
		if _, ok := idSet[e.ID]; ok {
			removedPlayerIndices = append(removedPlayerIndices, i)
		} else {
			kept = append(kept, e)
		}
	}
	m.queue.entries = kept
	m.clampQueueCursorAndScroll()
	if m.playback.currentIndex >= 0 {
		removedBefore := 0
		removedCurrent := false
		for _, ri := range removedPlayerIndices {
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
		for i := len(removedPlayerIndices) - 1; i >= 0; i-- {
			_ = qr.RemoveFromQueue(removedPlayerIndices[i])
		}
	}
	if len(m.queue.entries) == 0 || (m.playback.currentHash != "" && !queueContainsHash(m.queue.entries, m.playback.currentHash)) {
		m = m.resetPlaybackState()
	}
	n := a.count
	m.statusBar.notification = newNotification(fmt.Sprintf("Undone: removed %d pasted %s", n, pluralize(n, "track", "tracks")))
	return m, nil
}
