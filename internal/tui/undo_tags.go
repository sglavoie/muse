package tui

import (
	"fmt"
	"log/slog"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/db"
)

// undoTagToggleAction reverts a tag assignment or removal (undoKindTagToggle).
// wasAdd is true when the original action added the tag; undo removes it.
// wasAdd is false when the original action removed the tag; undo re-adds it.
type undoTagToggleAction struct {
	tagID  int64
	hashes []string
	wasAdd bool
}

func (a undoTagToggleAction) kind() undoKind { return undoKindTagToggle }

func (a undoTagToggleAction) description() string {
	n := len(a.hashes)
	if a.wasAdd {
		return fmt.Sprintf("Tag added to %d %s — press u to undo", n, pluralize(n, "track", "tracks"))
	}
	return fmt.Sprintf("Tag removed from %d %s — press u to undo", n, pluralize(n, "track", "tracks"))
}

func (a undoTagToggleAction) perform(m Model) (Model, tea.Cmd) {
	tw := m.tagWriter
	if tw == nil {
		return m, nil
	}
	tagID, hashes, wasAdd := a.tagID, a.hashes, a.wasAdd
	return m, func() tea.Msg {
		var err error
		if wasAdd {
			err = tw.RemoveTrackTagBulk(hashes, tagID)
		} else {
			err = tw.AddTrackTagBulk(hashes, tagID)
		}
		return tagToggledMsg{tagID: tagID, hashes: hashes, wasAdd: !wasAdd, err: err}
	}
}

// undoDeleteTagAction reverts a tag soft-delete (undoKindDeleteTag).
// Calls UndoDeleteTag to clear deleted_at, then restores the tag in the
// in-memory tag list and cache.
type undoDeleteTagAction struct {
	tag db.TagRow
	idx int
}

func (a undoDeleteTagAction) kind() undoKind { return undoKindDeleteTag }

func (a undoDeleteTagAction) description() string {
	return fmt.Sprintf("Tag %q deleted — press u to undo", a.tag.Name)
}

func (a undoDeleteTagAction) perform(m Model) (Model, tea.Cmd) {
	tw := m.tagWriter
	if tw != nil {
		if err := tw.UndoDeleteTag(a.tag.ID); err != nil {
			slog.Warn("undo delete tag: UndoDeleteTag failed", "id", a.tag.ID, "err", err)
		}
	}
	idx := min(a.idx, len(m.tags))
	restored := make([]db.TagRow, len(m.tags)+1)
	copy(restored, m.tags[:idx])
	restored[idx] = a.tag
	copy(restored[idx+1:], m.tags[idx:])
	m = m.rebuildTagCache(restored)

	tvIdx := min(a.idx, len(m.tagsView.tags))
	tvTags := make([]db.TagRow, len(m.tagsView.tags)+1)
	copy(tvTags, m.tagsView.tags[:tvIdx])
	tvTags[tvIdx] = a.tag
	copy(tvTags[tvIdx+1:], m.tagsView.tags[tvIdx:])
	m.tagsView.tags = tvTags
	m.tagsView.cursor = tvIdx
	m.tagsView = m.tagsView.clampCursor()

	m.statusBar.notification = newNotification(fmt.Sprintf("Undone: restored tag %q", a.tag.Name))
	return m, nil
}
