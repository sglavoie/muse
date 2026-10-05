package tui

import (
	"fmt"
	"log/slog"

	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/library"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
)

// handleTagFilterAction is called when the user presses ctrl+t.
// If a filter is active, it clears it. Otherwise it opens the tag filter picker.
func (m Model) handleTagFilterAction() (Model, tea.Cmd) {
	if m.activeTagFilter != nil {
		return m.clearTagFilter(), nil
	}
	tags := nonEmptyTags(m.tags, m.tagsView.TagCounts)
	if len(tags) == 0 {
		m.statusBar.Notify("No tags defined")
		return m, nil
	}
	picker := tagtui.NewTagFilterPicker(tags)
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
		return m.markQuitting(), tea.Quit, true
	}
	updated, cmd := m.overlay.tagFilterPicker.HandleKey(msg.String())
	m.overlay.tagFilterPicker = &updated
	return m, cmd, true
}

// handleTagFilterPicked sets activeTagFilter and rebuilds all view caches.
func (m Model) handleTagFilterPicked(msg tagtui.TagFilterPickedMsg) (Model, tea.Cmd) {
	m.overlay.tagFilterPicker = nil
	tagID := msg.Tag.ID
	m.activeTagFilter = &tagID
	m = m.applyTagFilterToViews()
	m.statusBar.Notify("Filtering by tag: " + msg.Tag.Name)
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
	m.statusBar.Notify("Tag filter cleared")
	return m
}

// applyTagFilterToViews rebuilds tagFilterHashes on all track-displaying views
// from m.activeTagFilter and m.trackTags.
func (m Model) applyTagFilterToViews() Model {
	var hashes map[string]struct{}
	if m.activeTagFilter != nil {
		hashes = m.buildTagFilterHashes(*m.activeTagFilter)
	}
	m.library.TagFilterHashes = hashes
	m.library = m.library.RecomputeSortedTracks()
	m.library = m.library.RebuildFilterCache()
	m.library.Cursor = 0
	m.library.ScrollOffset = 0

	m.queue.TagFilterHashes = hashes
	m.queue.Cursor = 0
	m.queue.ScrollOffset = 0

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
	tags := nonEmptyTags(m.tags, m.tagsView.TagCounts)
	if len(tags) == 0 {
		m.statusBar.Notify("No tags defined")
		return m, nil
	}
	picker := tagtui.NewTagFilterPicker(tags)
	m.overlay.tagFilterPicker = &picker
	return m, nil
}

// handleRemoveAllTagsFromTrack removes every tag from a track.
func (m Model) handleRemoveAllTagsFromTrack(msg removeAllTagsFromTrackMsg) (Model, tea.Cmd) {
	hash := msg.hash
	tagIDs := m.trackTags[hash]
	if len(tagIDs) == 0 {
		m.statusBar.Notify("Track has no tags")
		return m, nil
	}

	// Collect IDs to remove.
	ids := make([]int64, 0, len(tagIDs))
	for id := range tagIDs {
		ids = append(ids, id)
	}

	// Optimistic update.
	m.trackTags[hash] = nil
	m.djInvalidate()

	// Update activeTagFilter display if the track is now removed from filter.
	if m.activeTagFilter != nil {
		if m.library.TagFilterHashes != nil {
			delete(m.library.TagFilterHashes, hash)
		}
		if m.queue.TagFilterHashes != nil {
			delete(m.queue.TagFilterHashes, hash)
		}
		// Rebuild sort/filter caches and clamp cursors: the effective
		// track list may have shrunk because baseTracks() re-evaluates
		// tagFilterHashes, invalidating the cached sortedTracks.
		m.library = m.library.RecomputeSortedTracks()
		m.library = m.library.RebuildFilterCache()
		m.library = m.library.ClampCursor()
		m.clampQueueCursorAndScroll()
	}

	tw := m.tagWriter
	if tw == nil {
		return m, nil
	}
	return m, func() tea.Msg {
		// Remove all tag assignments in a single DB call instead of N
		// individual RemoveTrackTag calls through the serialized writer.
		if err := tw.RemoveAllTrackTags(hash); err != nil {
			slog.Warn("remove all tags: DB write failed", "hash", hash, "err", err)
			return allTagsRemovedMsg{hash: hash}
		}
		var autoDeleted []int64
		for _, id := range ids {
			if deleted, err := tw.DeleteTagIfEmpty(id); err != nil {
				slog.Warn("remove all tags: auto-delete empty tag failed", "tagID", id, "err", err)
			} else if deleted {
				autoDeleted = append(autoDeleted, id)
			}
		}
		return allTagsRemovedMsg{hash: hash, autoDeletedTagIDs: autoDeleted}
	}
}

// handleEnqueueByTag enqueues all tracks for a given tag ID from the palette.
func (m Model) handleEnqueueByTag(msg enqueueByTagMsg) (Model, tea.Cmd) {
	tr := m.tagReader
	be := m.batchEnqueuer
	if tr == nil || be == nil {
		m.statusBar.Notify("Enqueue not available")
		return m, nil
	}
	wasEmpty := len(m.queue.Entries) == 0
	tagID, name := msg.tagID, msg.name
	return m, func() tea.Msg {
		rows, err := tr.ReadTracksByTag(tagID)
		if err != nil {
			return BatchEnqueuedMsg{Err: fmt.Errorf("enqueue tag: %w", err)}
		}
		libTracks := make([]library.Track, len(rows))
		for i, r := range rows {
			libTracks[i] = library.Track{
				Hash:        r.Hash,
				Path:        r.Path,
				Artist:      r.Artist,
				Title:       r.Title,
				Album:       r.Album,
				AlbumArtist: r.AlbumArtist,
				Duration:    float64(r.DurationMs) / 1000,
			}
		}
		ids, err := be.AppendTracks(libTracks, name)
		if err != nil {
			return BatchEnqueuedMsg{Err: fmt.Errorf("enqueue tag: %w", err)}
		}
		return BatchEnqueuedMsg{
			Entries:      newQueueEntries(libTracks, ids, name),
			QueueIDs:     ids,
			WasEmpty:     wasEmpty,
			PlaylistName: name,
		}
	}
}
