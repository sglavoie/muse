package tui

import (
	"fmt"
	"log/slog"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
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
		m = m.preserveQueueSort()
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
	m.statusBar.notify("Undone: removed " + a.entry.Display)
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
	m = m.preserveQueueSort()
	if m.playback.currentIndex >= 0 && idx <= m.playback.currentIndex {
		m.playback.currentIndex++
	}
	// QueueView.cursor is a display index; translate the raw insertion
	// position through the active sort's displayOrder so that an undo
	// performed under an active sort lands the cursor on the restored entry
	// instead of an unrelated row at the same raw index.
	m.queue.cursor = m.queue.findEntryDisplayPosition(idx)
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
	m.statusBar.notify("Undone: restored removed track")
	return m, nil
}

// undoClearAction reverts a full queue clear (undoKindClear).
type undoClearAction struct {
	ids        []int64
	entries    []QueueEntry
	tracks     []player.QueueTrack
	savedIndex int    // playback index at the time of clear
	savedHash  string // currently-playing track hash at the time of clear
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
	m.playback.currentIndex = a.savedIndex
	m.playback.currentHash = a.savedHash
	if qs, ok := m.player.(playerQueueSetter); ok {
		qs.SetQueue(a.tracks)
	}
	if is, ok := m.player.(playerIndexSetter); ok {
		is.SetCurrentIndex(a.savedIndex)
	}
	n := len(a.entries)
	m.statusBar.notify(fmt.Sprintf("Undone: restored %d %s", n, pluralize(n, "track", "tracks")))
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
	m = m.preserveQueueSort()
	m.clampQueueCursorAndScroll()
	m.syncPlayerQueue()
	n := len(a.entries)
	m.statusBar.notify(fmt.Sprintf("Undone: restored %d %s", n, pluralize(n, "track", "tracks")))
	return m, nil
}

// softRemoveQueueEntries soft-deletes ids from the DB, removes the matching
// entries from the in-memory queue, adjusts the playback index, syncs the
// player queue, and resets playback state when the current track disappears.
// warnMsg is logged when the DB soft-delete fails.
func (m Model) softRemoveQueueEntries(ids []int64, warnMsg string) Model {
	if m.batchEnqueuer != nil {
		if err := m.batchEnqueuer.DequeueTracksSoft(ids); err != nil {
			slog.Warn(warnMsg, "err", err)
		}
		if m.undoer != nil {
			schedulePurge(&m.undo.slot, m.undoer.PurgeDeletedQueueEntries)
		}
	}
	idSet := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
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
	m = m.preserveQueueSort()
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
	return m
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
	m = m.softRemoveQueueEntries(a.ids, "undo batch enqueue: soft-delete failed")
	n := a.count
	m.statusBar.notify(fmt.Sprintf("Undone: removed %d %s", n, pluralize(n, "track", "tracks")))
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
	m = m.softRemoveQueueEntries(a.ids, "undo paste: soft-delete failed")
	n := a.count
	m.statusBar.notify(fmt.Sprintf("Undone: removed %d pasted %s", n, pluralize(n, "track", "tracks")))
	return m, nil
}

// undoCutPasteAction reverts a cut-paste by removing the pasted copies and
// re-inserting the originals at their previous positions.
type undoCutPasteAction struct {
	pastedIDs       []int64      // fresh DB IDs of pasted copies to remove
	originalEntries []QueueEntry // cut entries to restore
	originalIndices []int        // their original queue positions (before the cut)
	count           int
}

func (a undoCutPasteAction) kind() undoKind { return undoKindCutPaste }

func (a undoCutPasteAction) description() string {
	return fmt.Sprintf("Moved %d %s — press u to undo", a.count, pluralize(a.count, "track", "tracks"))
}

func (a undoCutPasteAction) perform(m Model) (Model, tea.Cmd) {
	// 1. Remove the pasted copies (same logic as undoPasteAction).
	if m.batchEnqueuer != nil {
		if err := m.batchEnqueuer.DequeueTracksSoft(a.pastedIDs); err != nil {
			slog.Warn("undo cut-paste: soft-delete pasted failed", "err", err)
		}
		if m.undoer != nil {
			schedulePurge(&m.undo.slot, m.undoer.PurgeDeletedQueueEntries)
		}
	}
	pastedIDSet := make(map[int64]struct{}, len(a.pastedIDs))
	for _, id := range a.pastedIDs {
		pastedIDSet[id] = struct{}{}
	}
	var kept []QueueEntry
	var removedPlayerIndices []int
	for i, e := range m.queue.entries {
		if _, ok := pastedIDSet[e.ID]; ok {
			removedPlayerIndices = append(removedPlayerIndices, i)
		} else {
			kept = append(kept, e)
		}
	}
	m.queue.entries = kept
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

	// 2. Re-insert the originals at their saved positions.
	// Insert in ascending index order to keep positions correct.
	restoredIDs := make([]int64, len(a.originalEntries))
	for i, idx := range a.originalIndices {
		pos := idx
		if pos > len(m.queue.entries) {
			pos = len(m.queue.entries)
		}
		entry := a.originalEntries[i]
		// Re-append to DB to get a fresh ID.
		if m.queueWriter != nil {
			track := library.Track{
				Path:     entry.Path,
				Hash:     entry.Hash,
				Artist:   entry.Artist,
				Title:    entry.Title,
				Album:    entry.Album,
				Duration: float64(entry.DurationMs) / 1000,
			}
			if id, err := m.queueWriter.AppendTrack(track, entry.SourcePlaylist); err == nil {
				entry.ID = id
				restoredIDs[i] = id
			}
		}
		newEntries := make([]QueueEntry, 0, len(m.queue.entries)+1)
		newEntries = append(newEntries, m.queue.entries[:pos]...)
		newEntries = append(newEntries, entry)
		newEntries = append(newEntries, m.queue.entries[pos:]...)
		m.queue.entries = newEntries
		// Adjust playback index for the insertion.
		if m.playback.currentIndex >= 0 && pos <= m.playback.currentIndex {
			m.playback.currentIndex++
		}
	}

	// Restore cut state so a subsequent paste moves instead of duplicating.
	m.yankIsCut = true
	for i := range m.yankBuffer {
		if i < len(restoredIDs) && restoredIDs[i] != 0 {
			m.yankBuffer[i].ID = restoredIDs[i]
		}
	}

	m = m.preserveQueueSort()
	m.clampQueueCursorAndScroll()
	m.syncPlayerQueue()
	if len(m.queue.entries) == 0 || (m.playback.currentHash != "" && !queueContainsHash(m.queue.entries, m.playback.currentHash)) {
		m = m.resetPlaybackState()
	}
	n := a.count
	m.statusBar.notify(fmt.Sprintf("Undone: restored %d moved %s", n, pluralize(n, "track", "tracks")))
	return m, nil
}
