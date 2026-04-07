package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	tagtui "github.com/sglavoie/muse/internal/tui/tags"
)

// handleTagMergeKeyPress intercepts key presses when the tag merge picker is open.
func (m Model) handleTagMergeKeyPress(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	if m.overlay.tagMergePicker == nil {
		return m, nil, false
	}
	if msg.String() == "ctrl+c" {
		return m, tea.Quit, true
	}
	updated, cmd := m.overlay.tagMergePicker.HandleKey(msg.String())
	m.overlay.tagMergePicker = &updated
	return m, cmd, true
}

// handleTagMerge opens the merge picker for the focused tag.
func (m Model) handleTagMerge() (Model, tea.Cmd) {
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
	if len(m.tags) < 2 {
		m.statusBar.notification = newNotification("Need at least 2 tags to merge")
		return m, nil
	}
	picker := NewTagMergePicker(tag, m.tags)
	m.overlay.tagMergePicker = &picker
	return m, nil
}

// handleTagMergePicked executes the irreversible merge after user confirmation.
func (m Model) handleTagMergePicked(msg tagMergePickedMsg) (Model, tea.Cmd) {
	m.overlay.tagMergePicker = nil
	tw := m.tagWriter
	if tw == nil {
		return m, nil
	}
	src, dest := msg.src, msg.dest
	return m, func() tea.Msg {
		err := tw.MergeTags(src.ID, dest.ID)
		return tagMergedMsg{src: src, dest: dest, err: err}
	}
}

// handleTagMerged processes tagMergedMsg: refreshes the tag cache and notifies.
func (m Model) handleTagMerged(msg tagMergedMsg) (Model, tea.Cmd) {
	if msg.err != nil {
		m.statusBar.notification = newNotification("Merge failed: " + msg.err.Error())
		return m, nil
	}

	if m.tagReader != nil {
		tags, err := m.tagReader.ReadTags()
		if err == nil {
			m = m.rebuildTagCache(tags)
			m.tagsView.tags = tags
		}
		counts, _ := m.tagReader.ReadTagCounts()
		m.tagsView.tagCounts = counts
	}
	m.tagsView = m.tagsView.clampCursor()
	m.statusBar.notification = newNotification(
		fmt.Sprintf("Merged %q into %q", msg.src.Name, msg.dest.Name))
	return m, nil
}
