package tui

import (
	"fmt"
	"log/slog"
	"math/rand/v2"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/player"
)

// handleMoveTrackUpMsg swaps a queue entry with the one above it and persists
// the reorder.
func (m Model) handleMoveTrackUpMsg(msg MoveTrackUpMsg) (Model, tea.Cmd) {
	return m.shiftQueueTrack(msg.Index, -1)
}

// handleMoveTrackDownMsg swaps a queue entry with the one below it and persists
// the reorder.
func (m Model) handleMoveTrackDownMsg(msg MoveTrackDownMsg) (Model, tea.Cmd) {
	return m.shiftQueueTrack(msg.Index, +1)
}

// shiftQueueTrack moves the queue entry at idx one position in the given
// direction (-1 = up, +1 = down) and persists the swap asynchronously.
func (m Model) shiftQueueTrack(idx, dir int) (Model, tea.Cmd) {
	n := len(m.queue.entries)
	if dir < 0 {
		if idx <= 0 || idx >= n {
			return m, nil
		}
	} else {
		if idx < 0 || idx >= n-1 {
			return m, nil
		}
	}

	m = exitVisualMode(m)
	entry := m.queue.entries[idx]
	neighbor := idx + dir
	m.queue.entries[idx], m.queue.entries[neighbor] = m.queue.entries[neighbor], m.queue.entries[idx]
	m.queue.cursor = neighbor

	if dir < 0 {
		if m.queue.cursor < m.queue.scrollOffset {
			m.queue.scrollOffset = m.queue.cursor
		}
	} else {
		maxRows := m.queue.visibleRows()
		if maxRows > 0 && m.queue.cursor >= m.queue.scrollOffset+maxRows {
			m.queue.scrollOffset = m.queue.cursor - maxRows + 1
		}
	}

	m = m.resetQueueSort()
	slog.Debug("track reordered", "hash", entry.Hash, "from", idx, "to", neighbor)
	switch m.playback.currentIndex {
	case idx:
		m.playback.currentIndex = neighbor
	case neighbor:
		m.playback.currentIndex = idx
	}
	m.queueDirty = true
	m.syncPlayerQueue()
	if m.reorderer != nil {
		reorderer := m.reorderer
		id := entry.ID
		return m, func() tea.Msg {
			if err := reorderer.ReorderQueueTrack(id, dir); err != nil {
				slog.Error("reorder queue track failed", "id", id, "direction", dir, "err", err)
			}
			return nil
		}
	}
	return m, nil
}

// handleQueueShuffleMsg applies a Fisher-Yates shuffle to the in-memory queue,
// updates the current-playing index, syncs the player queue, and persists the
// new order to the database when a queueShuffler is available.
func (m Model) handleQueueShuffleMsg() (Model, tea.Cmd) {
	if len(m.queue.entries) <= 1 {
		m.statusBar.Notify("Queue too small to shuffle")
		return m, nil
	}

	// Capture the currently-playing entry's database ID before shuffling.
	// Using the ID (unique per queue row) instead of the hash avoids
	// misidentifying the entry when the queue contains duplicate tracks.
	var currentID int64
	if m.playback.currentIndex >= 0 && m.playback.currentIndex < len(m.queue.entries) {
		currentID = m.queue.entries[m.playback.currentIndex].ID
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
	if currentID != 0 {
		for i, entry := range m.queue.entries {
			if entry.ID == currentID {
				m.playback.currentIndex = i
				break
			}
		}
	}

	m = m.resetQueueSort()
	m.syncPlayerQueue()
	m.statusBar.Notify(fmt.Sprintf("Queue shuffled (%d tracks)", len(m.queue.entries)))

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
		m.statusBar.Notify("No duplicates found")
		return m, nil
	}

	m.queue.entries = keep
	for i := range m.queue.entries {
		m.queue.entries[i].Selected = false
	}
	m = exitVisualMode(m)
	m = m.preserveQueueSort()
	m.clampQueueCursorAndScroll()

	// Re-locate the currently-playing entry by hash rather than using
	// index arithmetic.  Dedup keeps the first occurrence of each hash,
	// so if playback was on a later duplicate that got removed the old
	// index is stale.  Searching by hash mirrors handleQueueShuffleMsg.
	if m.playback.currentHash != "" {
		m.playback.currentIndex = -1
		for i, entry := range m.queue.entries {
			if entry.Hash == m.playback.currentHash {
				m.playback.currentIndex = i
				break
			}
		}
	}
	m = m.stopPlaybackIfCurrentMissing()
	m.syncPlayerQueue()

	batchDel := m.batchEnqueuer
	undoer := m.undoer
	cancel := m.scheduleSoftDelete(
		func() {
			if batchDel != nil {
				if err := batchDel.DequeueTracksSoft(removedIDs); err != nil {
					slog.Warn("dedup soft-delete failed", "err", err)
				}
			} else if undoer != nil {
				for _, id := range removedIDs {
					if err := undoer.DequeueTrackSoft(id); err != nil {
						slog.Warn("dedup soft-delete failed", "err", err)
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
