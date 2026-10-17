package tui

import (
	tea "charm.land/bubbletea/v2"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
)

// updateTagMsg routes tag updates without mixing picker, list, and filter state.
func (m Model) updateTagMsg(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	if next, cmd, ok := m.updateTagPickerMsg(msg); ok {
		return next, cmd, true
	}
	if next, cmd, ok := m.updateTagListMsg(msg); ok {
		return next, cmd, true
	}
	return m.updateTagFilterMsg(msg)
}

func (m Model) updateTagPickerMsg(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case tagtui.TagPickerDismissedMsg:
		m.overlay.tagPicker = nil

	case tagtui.TagPickerToggleMsg:
		next, cmd := m.handleTagPickerToggle(msg)
		return next, cmd, true

	case tagtui.TagPickerCreateMsg:
		next, cmd := m.handleTagPickerCreate(msg)
		return next, cmd, true

	case tagCreatedMsg:
		m = m.refreshTagsViewAfterCreate(msg)
		next, cmd := m.handleTagCreated(msg)
		return next, cmd, true

	case tagToggledMsg:
		next, cmd := m.handleTagToggled(msg)
		return next, cmd, true

	case emptyTagsPurgedMsg:
		m = m.removeTagsFromCaches(msg.tagIDs)

	case allTagsRemovedMsg:
		m = m.removeTagsFromCaches(msg.autoDeletedTagIDs)
		// Leave automatic tombstones for startup; a retained undo may need them.
		m.statusBar.Notify("All tags removed from track")

	default:
		return m, nil, false
	}
	return m, nil, true
}
func (m Model) updateTagListMsg(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case tagsRefreshedMsg:
		m = m.handleTagsRefreshed(msg)

	case tagViewTracksLoadedMsg:
		m = m.handleTagViewTracksLoaded(msg)

	case tagViewCountsLoadedMsg:
		m = m.handleTagViewCountsLoaded(msg)

	case tagRenamedMsg:
		m = m.handleTagRenamed(msg)

	case tagDeletedMsg:
		m = m.handleTagDeleted(msg)

	case tagtui.TagMergeDismissedMsg:
		m.overlay.tagMergePicker = nil

	case tagtui.TagMergePickedMsg:
		next, cmd := m.handleTagMergePicked(msg)
		return next, cmd, true

	case tagMergedMsg:
		next, cmd := m.handleTagMerged(msg)
		return next, cmd, true

	default:
		return m, nil, false
	}
	return m, nil, true
}
func (m Model) updateTagFilterMsg(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case tagtui.TagFilterPickedMsg:
		next, cmd := m.handleTagFilterPicked(msg)
		return next, cmd, true

	case tagtui.TagFilterDismissedMsg:
		m = m.handleTagFilterDismissed()

	case openTagFilterPickerMsg:
		next, cmd := m.handleOpenTagFilterPicker()
		return next, cmd, true

	case clearTagFilterMsg:
		m = m.clearTagFilter()

	case removeAllTagsFromTrackMsg:
		next, cmd := m.handleRemoveAllTagsFromTrack(msg)
		return next, cmd, true

	case enqueueByTagMsg:
		next, cmd := m.handleEnqueueByTag(msg)
		return next, cmd, true

	default:
		return m, nil, false
	}
	return m, nil, true
}

func (m Model) removeTagsFromCaches(ids []int64) Model {
	for _, id := range ids {
		m = m.removeTagFromCaches(id)
	}
	return m
}
