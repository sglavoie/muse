package tui

import (
	"log/slog"

	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/library"
)

// handleTagFilterAction is called when the user presses ctrl+t.
// If a filter is active, it clears it. Otherwise it opens the tag filter picker.
func (m Model) handleTagFilterAction() (Model, tea.Cmd) {
	if m.activeTagFilter != nil {
		return m.clearTagFilter(), nil
	}
	if len(m.tags) == 0 {
		m.statusBar.notification = newNotification("No tags defined")
		return m, nil
	}
	picker := NewTagFilterPicker(m.tags)
	m.overlay.tagFilterPicker = &picker
	return m, nil
}

// handleTagFilterPickerKeyPress intercepts key presses when the tag filter
// picker is open.
func (m Model) handleTagFilterPickerKeyPress(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	if m.overlay.tagFilterPicker == nil {
		return m, nil, false
	}
	if msg.String() == "ctrl+c" {
		return m, tea.Quit, true
	}
	updated, cmd := m.overlay.tagFilterPicker.HandleKey(msg.String())
	m.overlay.tagFilterPicker = &updated
	return m, cmd, true
}

// handleTagFilterPicked sets activeTagFilter and rebuilds all view caches.
func (m Model) handleTagFilterPicked(msg tagFilterPickedMsg) (Model, tea.Cmd) {
	m.overlay.tagFilterPicker = nil
	tagID := msg.tag.ID
	m.activeTagFilter = &tagID
	m = m.applyTagFilterToViews()
	m.statusBar.notification = newNotification("Filtering by tag: " + msg.tag.Name)
	return m, nil
}

// handleTagFilterDismissed closes the picker without changing the active filter.
func (m Model) handleTagFilterDismissed() Model {
	m.overlay.tagFilterPicker = nil
	return m
}

// clearTagFilter clears activeTagFilter and rebuilds all view caches.
func (m Model) clearTagFilter() Model {
	m.activeTagFilter = nil
	m = m.applyTagFilterToViews()
	m.statusBar.notification = newNotification("Tag filter cleared")
	return m
}

// applyTagFilterToViews rebuilds tagFilterHashes on all track-displaying views
// from m.activeTagFilter and m.trackTags.
func (m Model) applyTagFilterToViews() Model {
	var hashes map[string]struct{}
	if m.activeTagFilter != nil {
		hashes = m.buildTagFilterHashes(*m.activeTagFilter)
	}
	m.library.tagFilterHashes = hashes
	m.library = m.library.recomputeSortedTracks()
	m.library = m.library.rebuildFilterCache()
	m.library.cursor = 0
	m.library.scrollOffset = 0

	m.recentlyAdded.tagFilterHashes = hashes
	m.recentlyAdded = m.recentlyAdded.recomputeSortedTracks()
	m.recentlyAdded = m.recentlyAdded.rebuildFilterCache()
	m.recentlyAdded.cursor = 0
	m.recentlyAdded.scrollOffset = 0

	m.history.tagFilterHashes = hashes
	m.history = m.history.recomputeSortedTracks()
	m.history = m.history.rebuildFilterCache()
	m.history.cursor = 0
	m.history.scrollOffset = 0

	m.queue.tagFilterHashes = hashes
	m.queue.cursor = 0
	m.queue.scrollOffset = 0

	return m
}

// buildTagFilterHashes returns the set of track hashes that carry tagID.
func (m Model) buildTagFilterHashes(tagID int64) map[string]struct{} {
	result := make(map[string]struct{})
	for hash, tagIDs := range m.trackTags {
		if _, ok := tagIDs[tagID]; ok {
			result[hash] = struct{}{}
		}
	}
	return result
}

// handleOpenTagFilterPicker opens the tag filter picker (used from palette).
func (m Model) handleOpenTagFilterPicker() (Model, tea.Cmd) {
	if len(m.tags) == 0 {
		m.statusBar.notification = newNotification("No tags defined")
		return m, nil
	}
	picker := NewTagFilterPicker(m.tags)
	m.overlay.tagFilterPicker = &picker
	return m, nil
}

// handleRemoveAllTagsFromTrack removes every tag from a track.
func (m Model) handleRemoveAllTagsFromTrack(msg removeAllTagsFromTrackMsg) (Model, tea.Cmd) {
	hash := msg.hash
	tagIDs := m.trackTags[hash]
	if len(tagIDs) == 0 {
		m.statusBar.notification = newNotification("Track has no tags")
		return m, nil
	}

	// Collect IDs to remove.
	ids := make([]int64, 0, len(tagIDs))
	for id := range tagIDs {
		ids = append(ids, id)
	}

	// Optimistic update.
	m.trackTags[hash] = nil

	// Update activeTagFilter display if the track is now removed from filter.
	if m.activeTagFilter != nil {
		if m.library.tagFilterHashes != nil {
			delete(m.library.tagFilterHashes, hash)
		}
		if m.queue.tagFilterHashes != nil {
			delete(m.queue.tagFilterHashes, hash)
		}
	}

	tw := m.tagWriter
	if tw == nil {
		return m, nil
	}
	return m, func() tea.Msg {
		for _, id := range ids {
			if err := tw.RemoveTrackTag(hash, id); err != nil {
				slog.Warn("remove all tags: DB write failed", "hash", hash, "tagID", id, "err", err)
			}
		}
		return TransientNotifyMsg("All tags removed from track")
	}
}

// handleEnqueueByTag enqueues all tracks for a given tag ID from the palette.
func (m Model) handleEnqueueByTag(msg enqueueByTagMsg) (Model, tea.Cmd) {
	tr := m.tagReader
	be := m.batchEnqueuer
	if tr == nil || be == nil {
		m.statusBar.notification = newNotification("Enqueue not available")
		return m, nil
	}
	wasEmpty := len(m.queue.entries) == 0
	tagID, name := msg.tagID, msg.name
	return m, func() tea.Msg {
		rows, err := tr.ReadTracksByTag(tagID)
		if err != nil {
			return TransientNotifyMsg("Enqueue failed: " + err.Error())
		}
		libTracks := make([]library.Track, len(rows))
		for i, r := range rows {
			libTracks[i] = library.Track{
				Hash:     r.Hash,
				Path:     r.Path,
				Artist:   r.Artist,
				Title:    r.Title,
				Album:    r.Album,
				Duration: float64(r.DurationMs) / 1000,
			}
		}
		ids, err := be.AppendTracks(libTracks, name)
		if err != nil {
			return TransientNotifyMsg("Enqueue failed: " + err.Error())
		}
		return tagEnqueueAllMsg{tagName: name, count: len(libTracks), ids: ids, wasEmpty: wasEmpty}
	}
}
