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
		return m.markQuitting(), tea.Quit, true
	}
	updated, cmd := m.overlay.tagMergePicker.HandleKey(msg.String())
	m.overlay.tagMergePicker = &updated
	return m, cmd, true
}

// handleTagMerge opens the merge picker for the focused tag.
func (m Model) handleTagMerge() (Model, tea.Cmd) {
	if m.ActiveView != viewTags || m.tagsView.Level != tagtui.TagLevel {
		return m, nil
	}
	if m.tagsView.IsCreating() || m.tagsView.IsRenaming() {
		return m, nil
	}
	tag, ok := m.tagsView.FocusedTag()
	if !ok {
		return m, nil
	}
	if len(m.tags) < 2 {
		m.statusBar.Notify("Need at least 2 tags to merge")
		return m, nil
	}
	picker := tagtui.NewTagMergePicker(tag, m.tags)
	m.overlay.tagMergePicker = &picker
	return m, nil
}

// handleTagMergePicked executes the irreversible merge after user confirmation.
func (m Model) handleTagMergePicked(msg tagtui.TagMergePickedMsg) (Model, tea.Cmd) {
	m.overlay.tagMergePicker = nil
	tw := m.tagWriter
	if tw == nil {
		return m, nil
	}
	src, dest := msg.Src, msg.Dest
	return m, func() tea.Msg {
		err := tw.MergeTags(src.ID, dest.ID)
		return tagMergedMsg{src: src, dest: dest, err: err}
	}
}

// handleTagMerged processes tagMergedMsg: refreshes the tag cache and notifies.
func (m Model) handleTagMerged(msg tagMergedMsg) (Model, tea.Cmd) {
	if msg.err != nil {
		m.statusBar.NotifyError("Merge failed: " + msg.err.Error())
		return m, nil
	}

	// Update the in-memory trackTags cache: remap source → dest so that
	// tag dots and the tag picker reflect the merge immediately.
	for hash, tagIDs := range m.trackTags {
		if _, has := tagIDs[msg.src.ID]; has {
			delete(tagIDs, msg.src.ID)
			tagIDs[msg.dest.ID] = struct{}{}
			m.trackTags[hash] = tagIDs
		}
	}
	// The Auto-DJ stores its filters and its captured seed as tag ids, and the
	// merge has just soft-deleted one of them. Storage remapped its own rows
	// inside the merge transaction; these are the live copies of the same
	// references, and they have to follow or the session goes on naming a tag
	// that no longer exists — until the next whole-session write puts it back on
	// disk.
	m = m.djNoteTagMerged(msg.src.ID, msg.dest.ID)
	// The session row is only half of what the DJ stores. Every saved recipe
	// keeps its own filter references, and a Save-as, Rename, or Update
	// submitted between the merge transaction and this message rewrote one of
	// them from a snapshot frozen before the merge — putting the merged-away id
	// back on disk under a recipe nothing here would write again. Re-pointing
	// the stored references once more closes that window, whichever write
	// happened to land in it.
	m = m.djRepairMergedTagRefs(msg.src.ID, msg.dest.ID)
	if m.overlay.autoDJ != nil {
		// The panel can be open across a merge: the confirmation is answered
		// asynchronously, so the user may have opened it in between. Its draft,
		// its baseline, and the recipe rows it lists are all snapshots with a
		// write path behind them.
		remapped := m.overlay.autoDJ.RemapTag(msg.src.ID, msg.dest.ID)
		m.overlay.autoDJ = &remapped
	}
	// The remap changes which tracks a tag filter selects, and it lands here
	// even when the tag list re-read below fails.
	m.djInvalidate()

	// If the active cross-view tag filter pointed at the merged-away tag,
	// repoint it at the destination so the filter survives the merge and the
	// status bar badge does not show an empty tag name.
	if m.activeTagFilter != nil && *m.activeTagFilter == msg.src.ID {
		destID := msg.dest.ID
		m.activeTagFilter = &destID
		m = m.applyTagFilterToViews()
	}

	if m.tagReader != nil {
		tags, err := m.tagReader.ReadTags()
		if err == nil {
			m = m.rebuildTagCache(tags)
			m.tagsView.Tags = tags
		}
		counts, _ := m.tagReader.ReadTagCounts()
		m.tagsView.TagCounts = counts
	}
	m.tagsView = m.tagsView.ClampCursor()
	m.statusBar.Notify(
		fmt.Sprintf("Merged %q into %q", msg.src.Name, msg.dest.Name))
	return m, nil
}
