package tui

import (
	"log/slog"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/db"
)

// handleTagPickerKeyPress intercepts key presses when the tag picker is open.
func (m Model) handleTagPickerKeyPress(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	if m.overlay.tagPicker == nil {
		return m, nil, false
	}
	if msg.String() == "ctrl+c" {
		return m, tea.Quit, true
	}
	updated, cmd := m.overlay.tagPicker.HandleKey(msg.String(), msg)
	m.overlay.tagPicker = &updated
	return m, cmd, true
}

// openTagPicker opens the tag picker overlay for the focused track (single
// mode) or for the current bulk selection. Returns without opening when there
// is no focused track and no bulk selection.
func (m Model) openTagPicker() (Model, tea.Cmd) {
	hashes, isBulk := m.tagPickerHashes()
	if len(hashes) == 0 {
		m.statusBar.notification = newNotification("No track focused")
		return m, nil
	}

	var tp TagPicker
	var cmd tea.Cmd

	if isBulk {
		tp, cmd = NewTagPickerBulk(m.tags, hashes, m.trackTags)
	} else {
		trackTagIDs := m.trackTags[hashes[0]]
		if trackTagIDs == nil {
			trackTagIDs = map[int64]struct{}{}
		}
		tp, cmd = NewTagPicker(m.tags, hashes[0], trackTagIDs)
	}

	m.overlay.tagPicker = &tp
	return m, cmd
}

// handleTagPickerToggle processes TagPickerToggleMsg: writes to DB, updates
// the in-memory cache, updates the picker check state, and arms undo.
func (m Model) handleTagPickerToggle(msg TagPickerToggleMsg) (Model, tea.Cmd) {
	tw := m.tagWriter
	if tw == nil {
		return m, nil
	}

	tagID, hashes, add := msg.TagID, msg.Hashes, msg.Add
	m = m.applyTagToggle(tagID, hashes, add)

	// Update cross-view tag filter when the toggled tag is the active filter.
	if m.activeTagFilter != nil && tagID == *m.activeTagFilter {
		if add {
			for _, h := range hashes {
				if m.library.tagFilterHashes != nil {
					m.library.tagFilterHashes[h] = struct{}{}
				}
				if m.recentlyAdded.tagFilterHashes != nil {
					m.recentlyAdded.tagFilterHashes[h] = struct{}{}
				}
				if m.history.tagFilterHashes != nil {
					m.history.tagFilterHashes[h] = struct{}{}
				}
				if m.queue.tagFilterHashes != nil {
					m.queue.tagFilterHashes[h] = struct{}{}
				}
			}
		} else {
			for _, h := range hashes {
				if m.library.tagFilterHashes != nil {
					delete(m.library.tagFilterHashes, h)
				}
				if m.recentlyAdded.tagFilterHashes != nil {
					delete(m.recentlyAdded.tagFilterHashes, h)
				}
				if m.history.tagFilterHashes != nil {
					delete(m.history.tagFilterHashes, h)
				}
				if m.queue.tagFilterHashes != nil {
					delete(m.queue.tagFilterHashes, h)
				}
			}
		}
		m.library = m.library.recomputeSortedTracks()
		m.library = m.library.rebuildFilterCache()
		m.library = m.library.clampCursor()
		m.recentlyAdded = m.recentlyAdded.recomputeSortedTracks()
		m.recentlyAdded = m.recentlyAdded.rebuildFilterCache()
		m.recentlyAdded = m.recentlyAdded.clampCursor()
		m.history = m.history.recomputeSortedTracks()
		m.history = m.history.rebuildFilterCache()
		m.history = m.history.clampCursor()
		m.clampQueueCursorAndScroll()
	}

	if m.overlay.tagPicker != nil {
		var newState checkState
		if add {
			newState = checkChecked
		} else {
			newState = checkUnchecked
		}
		updated := m.overlay.tagPicker.UpdateItemState(tagID, newState)
		m.overlay.tagPicker = &updated
	}

	var tag db.TagRow
	for _, t := range m.tags {
		if t.ID == tagID {
			tag = t
			break
		}
	}
	m.setUndoSlot(undoTagToggleAction{tagID: tagID, hashes: hashes, wasAdd: add, tag: tag}, nil)

	return m, func() tea.Msg {
		var err error
		if add {
			if len(hashes) == 1 {
				err = tw.AddTrackTag(hashes[0], tagID)
			} else {
				err = tw.AddTrackTagBulk(hashes, tagID)
			}
		} else {
			if len(hashes) == 1 {
				err = tw.RemoveTrackTag(hashes[0], tagID)
			} else {
				err = tw.RemoveTrackTagBulk(hashes, tagID)
			}
		}
		if err != nil {
			slog.Warn("tag toggle: DB write failed", "tagID", tagID, "add", add, "err", err)
		}
		var autoDeleted bool
		if err == nil && !add {
			if deleted, delErr := tw.DeleteTagIfEmpty(tagID); delErr != nil {
				slog.Warn("tag toggle: auto-delete empty tag failed", "tagID", tagID, "err", delErr)
			} else {
				autoDeleted = deleted
			}
		}
		return tagToggledMsg{tagID: tagID, hashes: hashes, wasAdd: add, autoDeleted: autoDeleted, err: err}
	}
}

// handleTagPickerCreate processes TagPickerCreateMsg: creates the tag in the
// DB, refreshes the cache, and re-opens the picker with the new tag.
func (m Model) handleTagPickerCreate(msg TagPickerCreateMsg) (Model, tea.Cmd) {
	tw := m.tagWriter
	tr := m.tagReader
	if tw == nil || tr == nil {
		return m, nil
	}

	name := strings.TrimSpace(msg.Name)
	if name == "" {
		return m, nil
	}

	hashes := msg.Hashes
	return m, func() tea.Msg {
		id, err := tw.CreateTag(name, "")
		if err != nil {
			slog.Warn("tag picker: create tag failed", "name", name, "err", err)
			return tagCreatedMsg{err: err}
		}
		if len(hashes) == 1 {
			if addErr := tw.AddTrackTag(hashes[0], id); addErr != nil {
				slog.Warn("tag picker: assign created tag failed", "id", id, "err", addErr)
				return tagCreatedMsg{id: id, name: name, hashes: hashes, err: addErr}
			}
		} else if len(hashes) > 1 {
			if addErr := tw.AddTrackTagBulk(hashes, id); addErr != nil {
				slog.Warn("tag picker: assign created tag failed (bulk)", "id", id, "err", addErr)
				return tagCreatedMsg{id: id, name: name, hashes: hashes, err: addErr}
			}
		}
		return tagCreatedMsg{id: id, name: name, hashes: hashes}
	}
}

// handleTagCreated processes tagCreatedMsg: updates the cache and re-opens
// the tag picker so the user sees the new tag.
func (m Model) handleTagCreated(msg tagCreatedMsg) (Model, tea.Cmd) {
	if msg.err != nil {
		m.statusBar.notification = newNotification("Create tag failed: " + msg.err.Error())
		return m, nil
	}

	if m.tagReader != nil {
		tags, err := m.tagReader.ReadTags()
		if err == nil {
			m = m.rebuildTagCache(tags)
		}
	}

	// Ensure the new tag is in m.tags even if the DB read above returned
	// stale data (WAL snapshot isolation). Without this, syncTagsViewTags
	// would overwrite the correct m.tagsView.tags on view switch.
	if msg.id != 0 && !m.hasTag(msg.id) {
		m.tags = append(m.tags, db.TagRow{ID: msg.id, Name: msg.name})
		m.tagNames[msg.id] = msg.name
		m.tagColors[msg.id] = ""
	}

	if len(msg.hashes) > 0 && msg.id != 0 {
		m = m.applyTagToggle(msg.id, msg.hashes, true)
		if m.tagsView.tagCounts == nil {
			m.tagsView.tagCounts = make(map[int64]int)
		}
		m.tagsView.tagCounts[msg.id] = len(msg.hashes)
	}

	return m.openTagPicker()
}

// handleTagToggled processes tagToggledMsg: reverts the optimistic update on
// error, removes auto-deleted tags from caches, and is otherwise a no-op on
// success (the cache was already updated optimistically).
func (m Model) handleTagToggled(msg tagToggledMsg) (Model, tea.Cmd) {
	if msg.err != nil {
		m = m.applyTagToggle(msg.tagID, msg.hashes, !msg.wasAdd)
		if m.overlay.tagPicker != nil {
			var prevState checkState
			if msg.wasAdd {
				prevState = checkUnchecked
			} else {
				prevState = checkChecked
			}
			updated := m.overlay.tagPicker.UpdateItemState(msg.tagID, prevState)
			m.overlay.tagPicker = &updated
		}
		m.statusBar.notification = newNotification("Tag toggle failed: " + msg.err.Error())
		return m, nil
	}
	if msg.autoDeleted {
		m = m.removeTagFromCaches(msg.tagID)
		if m.tagWriter != nil {
			schedulePurge(&m.undo.slot, m.tagWriter.PurgeDeletedTags)
		}
	}
	return m, nil
}

// handleTagsRefreshed processes tagsRefreshedMsg: replaces the in-memory
// tag and trackTags caches with the freshly loaded data.
func (m Model) handleTagsRefreshed(msg tagsRefreshedMsg) Model {
	if msg.err != nil {
		slog.Warn("tags refresh failed", "err", msg.err)
		return m
	}
	m = m.rebuildTagCache(msg.tags)
	if msg.trackTags != nil {
		m.trackTags = msg.trackTags
	}
	return m
}
