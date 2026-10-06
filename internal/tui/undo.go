package tui

import (
	"strings"
	"time"
)

// undoAction is the interface implemented by every reversible operation.
// description() returns the "press u to undo" status-bar notification shown
// when the action is created. perform() executes the actual undo logic;
// a returned error preserves the slot for retry within its original window.
type undoAction interface {
	kind() undoKind
	perform(m Model) (Model, error)
	description() string
}

func (m Model) canUndo() bool {
	return m.undo.slot.action != nil && time.Now().Before(m.undo.slot.expiresAt)
}

func (m Model) undoDetail() string {
	description := m.undo.slot.action.description()
	for _, suffix := range []string{" — press u to undo", " · u to undo"} {
		description = strings.TrimSuffix(description, suffix)
	}
	return "Revert: " + description
}

// rearmUndoPurge retains cleanup for the kind of row the failed undo owns.
func (m *Model) rearmUndoPurge() {
	var purge func(int64) error
	switch m.undo.slot.action.kind() {
	case undoKindDeletePlaylist:
		if m.playlistDeleter != nil {
			purge = m.playlistDeleter.PurgeDeletedPlaylists
		}
	case undoKindRemovePlaylistTrack, undoKindBulkRemovePlaylistTracks:
		if m.playlistDeleter != nil {
			purge = m.playlistDeleter.PurgeDeletedPlaylistTracks
		}
	case undoKindDeleteTag:
		if m.tagWriter != nil {
			purge = m.tagWriter.PurgeDeletedTags
		}
	default:
		if m.undoer != nil {
			purge = m.undoer.PurgeDeletedQueueEntries
		}
	}
	if purge != nil {
		schedulePurge(&m.undo.slot, purge)
	}
}
