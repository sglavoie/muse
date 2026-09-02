package tui

import (
	"fmt"
	"log/slog"

	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/player"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
)

// handleTagOpen handles actionTagOpen in the Tags view.
// At TagLevel: drills into tracks for the focused tag.
// At TrackLevel: enqueues the focused track.
func (m Model) handleTagOpen() (Model, tea.Cmd) {
	if m.activeView != viewTags {
		return m, nil
	}
	switch m.tagsView.Level {
	case tagtui.TagLevel:
		tag, ok := m.tagsView.FocusedTag()
		if !ok {
			return m, nil
		}
		tr := m.tagReader
		if tr == nil {
			m.statusBar.Notify("Tag reader not available")
			return m, nil
		}
		return m, func() tea.Msg {
			tracks, err := tr.ReadTracksByTag(tag.ID)
			if err != nil {
				return tagViewTracksLoadedMsg{err: err}
			}
			counts, _ := tr.ReadTagCounts()
			return tagViewTracksLoadedMsg{tag: tag, tracks: tracks, counts: counts}
		}
	case tagtui.TrackLevel:
		cmd := m.tagsView.EnqueueFocusedTrack(m.queueWriter, len(m.queue.entries))
		return m, cmd
	}
	return m, nil
}

// handleTagBack navigates one level up in the Tags view.
func (m Model) handleTagBack() Model {
	if m.activeView != viewTags {
		return m
	}
	m = exitVisualMode(m)
	m.tagsView = m.tagsView.Back()
	return m
}

// handleTagCreate starts the inline create input.
func (m Model) handleTagCreate() (Model, tea.Cmd) {
	if m.activeView != viewTags || m.tagsView.Level != tagtui.TagLevel {
		return m, nil
	}
	if m.tagsView.IsCreating() || m.tagsView.IsRenaming() {
		return m, nil
	}
	m.tagsView = m.tagsView.StartCreate()
	return m, nil
}

// handleTagRename starts the inline rename input for the focused tag.
func (m Model) handleTagRename() (Model, tea.Cmd) {
	if m.activeView != viewTags || m.tagsView.Level != tagtui.TagLevel {
		return m, nil
	}
	if m.tagsView.IsCreating() || m.tagsView.IsRenaming() {
		return m, nil
	}
	tag, ok := m.tagsView.FocusedTag()
	if !ok {
		return m, nil
	}
	m.tagsView = m.tagsView.StartRename(tag.Name)
	return m, nil
}

// handleTagDelete soft-deletes the focused tag and arms undo.
func (m Model) handleTagDelete() (Model, tea.Cmd) {
	if m.activeView != viewTags || m.tagsView.Level != tagtui.TagLevel {
		return m, nil
	}
	tag, ok := m.tagsView.FocusedTag()
	if !ok {
		return m, nil
	}

	// Find the canonical index in the unfiltered tag list so undo can
	// re-insert at the correct position even when a filter is active.
	canonicalIdx := 0
	for i, t := range m.tagsView.Tags {
		if t.ID == tag.ID {
			canonicalIdx = i
			break
		}
	}

	newTags := make([]db.TagRow, 0, len(m.tagsView.Tags)-1)
	for _, t := range m.tagsView.Tags {
		if t.ID != tag.ID {
			newTags = append(newTags, t)
		}
	}
	m.tagsView.Tags = newTags
	m.tagsView = m.tagsView.ClampCursor()

	newGlobalTags := make([]db.TagRow, 0, len(m.tags)-1)
	for _, t := range m.tags {
		if t.ID != tag.ID {
			newGlobalTags = append(newGlobalTags, t)
		}
	}
	m = m.rebuildTagCache(newGlobalTags)

	// If the cross-view tag filter was set to the tag being deleted, clear it
	// so the user is not stranded looking at an empty list with a "[tag: ]"
	// badge whose tag name lookup now returns the empty string.
	if m.activeTagFilter != nil && *m.activeTagFilter == tag.ID {
		m = m.clearTagFilter()
	}

	tw := m.tagWriter
	cancel := m.scheduleSoftDelete(
		func() {
			if tw != nil {
				if err := tw.DeleteTagSoft(tag.ID); err != nil {
					slog.Warn("tag soft-delete failed", "id", tag.ID, "err", err)
				}
			}
		},
		func(before int64) {
			if tw != nil {
				if err := tw.PurgeDeletedTags(before); err != nil {
					slog.Warn("tag purge failed", "err", err)
				}
			}
		},
	)

	m.setUndoSlot(undoDeleteTagAction{tag: tag, idx: canonicalIdx}, cancel)
	player.PlaySound(player.SoundRemoved)
	return m, nil
}

// handleTagEnqueueAll enqueues all tracks for the focused tag.
func (m Model) handleTagEnqueueAll() (Model, tea.Cmd) {
	if m.activeView != viewTags || m.tagsView.Level != tagtui.TagLevel {
		return m, nil
	}
	tag, ok := m.tagsView.FocusedTag()
	if !ok {
		return m, nil
	}
	tr := m.tagReader
	be := m.batchEnqueuer
	if tr == nil || be == nil {
		m.statusBar.Notify("Enqueue not available")
		return m, nil
	}
	wasEmpty := len(m.queue.entries) == 0
	tagName := tag.Name
	tagID := tag.ID
	return m, func() tea.Msg {
		rows, err := tr.ReadTracksByTag(tagID)
		if err != nil {
			return BatchEnqueuedMsg{Err: fmt.Errorf("enqueue tag: %w", err)}
		}
		libTracks := playlistRowsToLibTracks(rows)
		ids, err := be.AppendTracks(libTracks, tagName)
		if err != nil {
			return BatchEnqueuedMsg{Err: fmt.Errorf("enqueue tag: %w", err)}
		}
		return BatchEnqueuedMsg{
			Entries:      newQueueEntries(libTracks, ids, tagName),
			QueueIDs:     ids,
			WasEmpty:     wasEmpty,
			PlaylistName: tagName,
		}
	}
}

// handleTagCreateInputKeyPress is the keyPressHandler signature wrapper for
// the tag create input (used in orderedKeyPressHandlers).
func (m Model) handleTagCreateInputKeyPress(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	return m.handleTagCreateInput(msg)
}

// handleTagRenameInputKeyPress is the keyPressHandler signature wrapper for
// the tag rename input (used in orderedKeyPressHandlers).
func (m Model) handleTagRenameInputKeyPress(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	return m.handleTagRenameInput(msg)
}

// handleTagCreateInput processes enter/esc on the inline create input.
// Returns (updated model, cmd, consumed).
func (m Model) handleTagCreateInput(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	if !m.tagsView.IsCreating() || m.activeView != viewTags {
		return m, nil, false
	}
	switch msg.String() {
	case "enter":
		name := m.tagsView.CreateInputValue()
		m.tagsView = m.tagsView.CancelCreate()
		if name == "" {
			return m, nil, true
		}
		tw := m.tagWriter
		if tw == nil {
			m.statusBar.Notify("Tag writer not available")
			return m, nil, true
		}
		return m, func() tea.Msg {
			id, err := tw.CreateTag(name, "")
			return tagCreatedMsg{id: id, name: name, err: err}
		}, true
	case "esc", "ctrl+c":
		m.tagsView = m.tagsView.CancelCreate()
		return m, nil, true
	default:
		var cmd tea.Cmd
		m.tagsView, cmd = m.tagsView.UpdateCreateInput(msg)
		return m, cmd, true
	}
}

// handleTagRenameInput processes enter/esc on the inline rename input.
// Returns (updated model, cmd, consumed).
func (m Model) handleTagRenameInput(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	if !m.tagsView.IsRenaming() || m.activeView != viewTags {
		return m, nil, false
	}
	switch msg.String() {
	case "enter":
		name := m.tagsView.RenameInputValue()
		tag, _ := m.tagsView.FocusedTag()
		m.tagsView = m.tagsView.CancelRename()
		if name == "" || tag.ID == 0 {
			return m, nil, true
		}
		tw := m.tagWriter
		if tw == nil {
			m.statusBar.Notify("Tag writer not available")
			return m, nil, true
		}
		tagID := tag.ID
		return m, func() tea.Msg {
			err := tw.RenameTag(tagID, name)
			return tagRenamedMsg{id: tagID, name: name, err: err}
		}, true
	case "esc", "ctrl+c":
		m.tagsView = m.tagsView.CancelRename()
		return m, nil, true
	default:
		var cmd tea.Cmd
		m.tagsView, cmd = m.tagsView.UpdateRenameInput(msg)
		return m, cmd, true
	}
}
