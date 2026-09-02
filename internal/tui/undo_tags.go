package tui

import (
	"fmt"
	"log/slog"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/tui/text"
)

// undoTagToggleAction reverts a tag assignment or removal (undoKindTagToggle).
// wasAdd is true when the original action added the tag; undo removes it.
// wasAdd is false when the original action removed the tag; undo re-adds it.
// tag stores the TagRow for cache restoration when the tag was auto-deleted.
type undoTagToggleAction struct {
	tagID  int64
	hashes []string
	wasAdd bool
	tag    db.TagRow // stored for cache restoration on undo of auto-deleted tag
}

func (a undoTagToggleAction) kind() undoKind { return undoKindTagToggle }

func (a undoTagToggleAction) description() string {
	n := len(a.hashes)
	if a.wasAdd {
		return fmt.Sprintf("Tag added to %d %s — press u to undo", n, text.Pluralize(n, "track", "tracks"))
	}
	return fmt.Sprintf("Tag removed from %d %s — press u to undo", n, text.Pluralize(n, "track", "tracks"))
}

func (a undoTagToggleAction) perform(m Model) (Model, tea.Cmd) {
	tw := m.tagWriter
	if tw == nil {
		return m, nil
	}

	if a.wasAdd {
		// Undo add → remove.
		if err := tw.RemoveTrackTagBulk(a.hashes, a.tagID); err != nil {
			slog.Warn("undo tag toggle: remove failed", "tagID", a.tagID, "err", err)
		}
		m = m.applyTagToggle(a.tagID, a.hashes, false)
	} else {
		// Undo remove → re-add. Restore soft-deleted tag first (harmless
		// no-op when the tag was not actually auto-deleted).
		if err := tw.UndoDeleteTag(a.tagID); err != nil {
			slog.Warn("undo tag toggle: restore tag failed", "tagID", a.tagID, "err", err)
		}
		if err := tw.AddTrackTagBulk(a.hashes, a.tagID); err != nil {
			slog.Warn("undo tag toggle: re-add failed", "tagID", a.tagID, "err", err)
		}
		m = m.applyTagToggle(a.tagID, a.hashes, true)
		// If the tag was removed from the cache (auto-deleted), restore it.
		if _, found := m.tagNames[a.tagID]; !found {
			restored := make([]db.TagRow, len(m.tags)+1)
			copy(restored, m.tags)
			restored[len(m.tags)] = a.tag
			m = m.rebuildTagCache(restored)
			m.tagsView.Tags = m.tags
		}
	}

	m.statusBar.Notify("Undone: tag toggle")
	return m, nil
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

	tvIdx := min(a.idx, len(m.tagsView.Tags))
	tvTags := make([]db.TagRow, len(m.tagsView.Tags)+1)
	copy(tvTags, m.tagsView.Tags[:tvIdx])
	tvTags[tvIdx] = a.tag
	copy(tvTags[tvIdx+1:], m.tagsView.Tags[tvIdx:])
	m.tagsView.Tags = tvTags
	m.tagsView.Cursor = tvIdx
	m.tagsView = m.tagsView.ClampCursor()

	m.statusBar.Notify(fmt.Sprintf("Undone: restored tag %q", a.tag.Name))
	return m, nil
}
