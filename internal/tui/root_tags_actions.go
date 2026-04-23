package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/db"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
)

// handleTagOpen handles actionTagOpen in the Tags view.
// At TagLevel: drills into tracks for the focused tag.
// At TrackLevel: enqueues the focused track.
func (m Model) handleTagOpen() (Model, tea.Cmd) {
	if m.activeView != viewTags {
		return m, nil
	}
	switch m.tagsView.level {
	case tagtui.TagLevel:
		tag, ok := m.tagsView.focusedTag()
		if !ok {
			return m, nil
		}
		tr := m.tagReader
		if tr == nil {
			m.statusBar.notify("Tag reader not available")
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
		cmd := m.tagsView.enqueueFocusedTrack(m.queueWriter, len(m.queue.entries))
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
	m.tagsView = m.tagsView.back()
	return m
}

// handleTagCreate starts the inline create input.
func (m Model) handleTagCreate() (Model, tea.Cmd) {
	if m.activeView != viewTags || m.tagsView.level != tagtui.TagLevel {
		return m, nil
	}
	if m.tagsView.isCreating() || m.tagsView.isRenaming() {
		return m, nil
	}
	m.tagsView = m.tagsView.startCreate()
	return m, nil
}

// handleTagRename starts the inline rename input for the focused tag.
func (m Model) handleTagRename() (Model, tea.Cmd) {
	if m.activeView != viewTags || m.tagsView.level != tagtui.TagLevel {
		return m, nil
	}
	if m.tagsView.isCreating() || m.tagsView.isRenaming() {
		return m, nil
	}
	tag, ok := m.tagsView.focusedTag()
	if !ok {
		return m, nil
	}
	m.tagsView = m.tagsView.startRename(tag.Name)
	return m, nil
}

// handleTagDelete soft-deletes the focused tag and arms undo.
func (m Model) handleTagDelete() (Model, tea.Cmd) {
	if m.activeView != viewTags || m.tagsView.level != tagtui.TagLevel {
		return m, nil
	}
	tag, ok := m.tagsView.focusedTag()
	if !ok {
		return m, nil
	}
	idx := m.tagsView.cursor

	newTags := make([]db.TagRow, 0, len(m.tagsView.tags)-1)
	for _, t := range m.tagsView.tags {
		if t.ID != tag.ID {
			newTags = append(newTags, t)
		}
	}
	m.tagsView.tags = newTags
	m.tagsView = m.tagsView.clampCursor()

	newGlobalTags := make([]db.TagRow, 0, len(m.tags)-1)
	for _, t := range m.tags {
		if t.ID != tag.ID {
			newGlobalTags = append(newGlobalTags, t)
		}
	}
	m = m.rebuildTagCache(newGlobalTags)

	m.setUndoSlot(undoDeleteTagAction{tag: tag, idx: idx}, nil)

	tw := m.tagWriter
	return m, func() tea.Msg {
		var err error
		if tw != nil {
			err = tw.DeleteTagSoft(tag.ID)
		}
		return tagDeletedMsg{tag: tag, idx: idx, err: err}
	}
}

// handleTagEnqueueAll enqueues all tracks for the focused tag.
func (m Model) handleTagEnqueueAll() (Model, tea.Cmd) {
	if m.activeView != viewTags || m.tagsView.level != tagtui.TagLevel {
		return m, nil
	}
	tag, ok := m.tagsView.focusedTag()
	if !ok {
		return m, nil
	}
	tr := m.tagReader
	be := m.batchEnqueuer
	if tr == nil || be == nil {
		m.statusBar.notify("Enqueue not available")
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
	if !m.tagsView.isCreating() || m.activeView != viewTags {
		return m, nil, false
	}
	switch msg.String() {
	case "enter":
		name := m.tagsView.createInputValue()
		m.tagsView = m.tagsView.cancelCreate()
		if name == "" {
			return m, nil, true
		}
		tw := m.tagWriter
		return m, func() tea.Msg {
			id, err := tw.CreateTag(name, "")
			return tagCreatedMsg{id: id, name: name, err: err}
		}, true
	case "esc", "ctrl+c":
		m.tagsView = m.tagsView.cancelCreate()
		return m, nil, true
	default:
		var cmd tea.Cmd
		m.tagsView, cmd = m.tagsView.updateCreateInput(msg)
		return m, cmd, true
	}
}

// handleTagRenameInput processes enter/esc on the inline rename input.
// Returns (updated model, cmd, consumed).
func (m Model) handleTagRenameInput(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	if !m.tagsView.isRenaming() || m.activeView != viewTags {
		return m, nil, false
	}
	switch msg.String() {
	case "enter":
		name := m.tagsView.renameInputValue()
		tag, _ := m.tagsView.focusedTag()
		m.tagsView = m.tagsView.cancelRename()
		if name == "" || tag.ID == 0 {
			return m, nil, true
		}
		tw := m.tagWriter
		tagID := tag.ID
		return m, func() tea.Msg {
			err := tw.RenameTag(tagID, name)
			return tagRenamedMsg{id: tagID, name: name, err: err}
		}, true
	case "esc", "ctrl+c":
		m.tagsView = m.tagsView.cancelRename()
		return m, nil, true
	default:
		var cmd tea.Cmd
		m.tagsView, cmd = m.tagsView.updateRenameInput(msg)
		return m, cmd, true
	}
}
