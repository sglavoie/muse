package tui

import (
	"fmt"
	"log/slog"
	"math/rand/v2"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/player"
	"github.com/sglavoie/muse/internal/tui/queue"
)

// handleMoveTrackUpMsg swaps a queue entry with the one above it and persists
// the reorder.
func (m Model) handleMoveTrackUpMsg(msg queue.MoveTrackUpMsg) (Model, tea.Cmd) {
	return m.shiftQueueTrack(msg.Index, -1)
}

// handleMoveTrackDownMsg swaps a queue entry with the one below it and persists
// the reorder.
func (m Model) handleMoveTrackDownMsg(msg queue.MoveTrackDownMsg) (Model, tea.Cmd) {
	return m.shiftQueueTrack(msg.Index, +1)
}

// shiftQueueTrack moves the queue entry at idx one position in the given
// direction (-1 = up, +1 = down) and persists the swap asynchronously.
func (m Model) shiftQueueTrack(idx, dir int) (Model, tea.Cmd) {
	n := len(m.queue.Entries)
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
	entry := m.queue.Entries[idx]
	neighbor := idx + dir
	m.queue.Entries[idx], m.queue.Entries[neighbor] = m.queue.Entries[neighbor], m.queue.Entries[idx]
	m.queue.Cursor = neighbor

	if dir < 0 {
		if m.queue.Cursor < m.queue.ScrollOffset {
			m.queue.ScrollOffset = m.queue.Cursor
		}
	} else {
		maxRows := m.queue.VisibleRows()
		if maxRows > 0 && m.queue.Cursor >= m.queue.ScrollOffset+maxRows {
			m.queue.ScrollOffset = m.queue.Cursor - maxRows + 1
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
	if len(m.queue.Entries) <= 1 {
		m.statusBar.Notify("Queue too small to shuffle")
		return m, nil
	}

	// Capture the currently-playing entry's database ID before shuffling.
	// Using the ID (unique per queue row) instead of the hash avoids
	// misidentifying the entry when the queue contains duplicate tracks.
	var currentID int64
	if m.playback.currentIndex >= 0 && m.playback.currentIndex < len(m.queue.Entries) {
		currentID = m.queue.Entries[m.playback.currentIndex].ID
	}

	entries := make([]queue.Entry, len(m.queue.Entries))
	copy(entries, m.queue.Entries)
	for i := len(entries) - 1; i > 0; i-- {
		j := rand.IntN(i + 1) //nolint:gosec // non-security random for queue shuffle
		entries[i], entries[j] = entries[j], entries[i]
	}
	m.queue.Entries = entries
	for i := range m.queue.Entries {
		m.queue.Entries[i].Selected = false
	}
	m = exitVisualMode(m)

	m.playback.currentIndex = -1
	if currentID != 0 {
		for i, entry := range m.queue.Entries {
			if entry.ID == currentID {
				m.playback.currentIndex = i
				break
			}
		}
	}

	m = m.resetQueueSort()
	m.syncPlayerQueue()
	m.statusBar.Notify(fmt.Sprintf("Queue shuffled (%d tracks)", len(m.queue.Entries)))

	if shuffler, ok := m.reorderer.(queueShuffler); ok {
		ids := make([]int64, len(m.queue.Entries))
		for i, entry := range m.queue.Entries {
			ids[i] = entry.ID
		}
		return m, func() tea.Msg { _ = shuffler.SetQueueOrder(ids); return nil }
	}
	return m, nil
}

// queueDuplicateCount uses the same hash identity as handleQueueDedupMsg.
func (m Model) queueDuplicateCount() int {
	seen := make(map[string]struct{}, len(m.queue.Entries))
	for _, entry := range m.queue.Entries {
		seen[entry.Hash] = struct{}{}
	}
	return len(m.queue.Entries) - len(seen)
}

// handleQueueDedupMsg removes duplicate queue entries, keeping the first
// occurrence of each track hash. Removed entries are soft-deleted with undo
// support.
func (m Model) handleQueueDedupMsg() (Model, tea.Cmd) {
	seen := make(map[string]struct{})
	var keep []queue.Entry
	var removedEntries []queue.Entry
	var removedIndices []int
	var removedIDs []int64

	for i, entry := range m.queue.Entries {
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

	m.queue.Entries = keep
	for i := range m.queue.Entries {
		m.queue.Entries[i].Selected = false
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
		for i, entry := range m.queue.Entries {
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
