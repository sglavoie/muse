package tui

import (
	"os"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/tui/keys"
	"github.com/sglavoie/muse/internal/tui/playback"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
)

// HandleAction executes a pre-resolved queue action dispatched by root.Update.
// Root resolves the raw key to an action via the keybinding registry and calls
// this method, keeping key-to-action mapping in one authoritative location.
func (v QueueView) HandleAction(a keys.Action) (QueueView, tea.Cmd) {
	filtered := v.filteredEntries()
	n := len(filtered)

	switch a {
	case keys.ActionCursorUp:
		if v.cursor > 0 {
			v.cursor--
		}
		if v.cursor < v.scrollOffset {
			v.scrollOffset = v.cursor
		}
	case keys.ActionCursorDown:
		if v.cursor < n-1 {
			v.cursor++
		}
		maxRows := v.visibleRows()
		if v.cursor >= v.scrollOffset+maxRows {
			v.scrollOffset = v.cursor - maxRows + 1
		}
	case keys.ActionHalfPageDown:
		v = v.moveQueueCursor(v.visibleRows() / 2)
	case keys.ActionHalfPageUp:
		v = v.moveQueueCursor(-v.visibleRows() / 2)
	case keys.ActionFullPageDown:
		v = v.moveQueueCursor(v.visibleRows())
	case keys.ActionFullPageUp:
		v = v.moveQueueCursor(-v.visibleRows())
	case keys.ActionGoToTop:
		v.cursor = 0
		v.scrollOffset = 0
	case keys.ActionGoToBottom:
		if n > 0 {
			v.cursor = n - 1
			maxRows := v.visibleRows()
			if v.cursor >= maxRows {
				v.scrollOffset = v.cursor - maxRows + 1
			}
		}
	case keys.ActionSelect:
		if v.cursor >= 0 && v.cursor < n {
			e := filtered[v.cursor]
			idx := e.origIdx
			// Pre-flight stat: the fsnotify watcher is debounced, so a file
			// the user deleted moments ago may still appear playable here.
			// Short-circuiting to the removal path avoids calling the player
			// (and mpv IPC) synchronously from the Update goroutine, which
			// would freeze the UI.
			if e.Missing || !fileExists(e.Path) {
				return v, func() tea.Msg {
					return playback.MissingTrackSelectedMsg{Index: idx, Path: e.Path, Hash: e.Hash}
				}
			}
			return v, func() tea.Msg { return playback.PlayQueueIndexMsg{Index: idx} }
		}
	case keys.ActionDequeue:
		if v.cursor >= 0 && v.cursor < n {
			e := filtered[v.cursor]
			idx := e.origIdx
			return v, func() tea.Msg { return DequeueTrackMsg{Index: idx} }
		}
	case keys.ActionBulkSelectionToggle:
		if v.cursor >= 0 && v.cursor < n {
			e := filtered[v.cursor]
			idx := e.origIdx
			return v, func() tea.Msg { return BulkSelectionToggleMsg{Index: idx} }
		}
	case keys.ActionClearQueue:
		return v, func() tea.Msg { return ClearQueueMsg{} }
	case keys.ActionMoveTrackUp:
		if v.displayOrder != nil || v.filter.Text != "" || len(v.tagFilterHashes) > 0 {
			return v, func() tea.Msg { return TransientNotifyMsg("Cannot reorder while sorted/filtered") }
		}
		if v.cursor > 0 {
			idx := v.cursor
			return v, func() tea.Msg { return MoveTrackUpMsg{Index: idx} }
		}
	case keys.ActionMoveTrackDown:
		if v.displayOrder != nil || v.filter.Text != "" || len(v.tagFilterHashes) > 0 {
			return v, func() tea.Msg { return TransientNotifyMsg("Cannot reorder while sorted/filtered") }
		}
		if v.cursor < n-1 {
			idx := v.cursor
			return v, func() tea.Msg { return MoveTrackDownMsg{Index: idx} }
		}
	}
	return v, nil
}

// fileExists reports whether the given path can be stat'd. It returns
// false for any error, including permission-denied: the caller treats a
// non-stat'able path as "cannot play" regardless of the underlying cause.
// Declared as a var so tests that exercise actionSelect with synthetic
// paths can stub the check without creating real temp files.
var fileExists = func(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// markMissing marks all entries whose Path matches as missing.
func (v *QueueView) markMissing(path string) {
	for i := range v.entries {
		if v.entries[i].Path == path {
			v.entries[i].Missing = true
		}
	}
}

// markMissingByHash marks all entries with the given content hash as missing.
// Used when a scan discovers different content at a queued entry's path: the
// old hash's file is gone (overwritten in place), so its queue instances must
// not play the replacement content under stale metadata. Marking by hash — not
// path — keeps a same-content copy queued under a different path untouched.
func (v *QueueView) markMissingByHash(hash string) {
	if hash == "" {
		return
	}
	for i := range v.entries {
		if v.entries[i].Hash == hash {
			v.entries[i].Missing = true
		}
	}
}

// isSavingAsPlaylist reports whether the inline save-as-playlist input is active.
func (v QueueView) isSavingAsPlaylist() bool {
	return v.savingAsPlaylist
}

// startSaveAsPlaylist activates the inline save-as-playlist input with an empty buffer.
// When fromSelection is true the prompt says "Save selection as playlist" and
// only bulk-selected entries will be persisted.
func (v QueueView) startSaveAsPlaylist(fromSelection bool) QueueView {
	v.savingAsPlaylist = true
	v.savingFromSelection = fromSelection
	v.saveInput = pltui.NewInlineInput("")
	return v
}

// cancelSaveAsPlaylist dismisses the inline save-as-playlist input without persisting.
func (v QueueView) cancelSaveAsPlaylist() QueueView {
	v.savingAsPlaylist = false
	v.savingFromSelection = false
	v.saveInput = textinput.Model{}
	return v
}

// hasSelection reports whether any queue entries are bulk-selected.
func (v QueueView) hasSelection() bool {
	for _, e := range v.entries {
		if e.Selected {
			return true
		}
	}
	return false
}

// selectedHashes returns the hashes of all bulk-selected entries, preserving queue order.
func (v QueueView) selectedHashes() []string {
	var hashes []string
	for _, e := range v.entries {
		if e.Selected {
			hashes = append(hashes, e.Hash)
		}
	}
	return hashes
}

// updateSaveInput forwards a key message to the Bubbles textinput model.
func (v QueueView) updateSaveInput(msg tea.KeyPressMsg) (QueueView, tea.Cmd) {
	var cmd tea.Cmd
	v.saveInput, cmd = v.saveInput.Update(msg)
	return v, cmd
}

// updateSaveInputMsg forwards a generic tea.Msg to the textinput, enabling
// paste and other multi-step commands.
func (v QueueView) updateSaveInputMsg(msg tea.Msg) (QueueView, tea.Cmd) {
	var cmd tea.Cmd
	v.saveInput, cmd = v.saveInput.Update(msg)
	return v, cmd
}

// saveInputValue returns the current save input buffer, trimmed of
// leading/trailing whitespace to prevent names like "  hello  ".
func (v QueueView) saveInputValue() string {
	return strings.TrimSpace(v.saveInput.Value())
}

// saveInputIsEmpty reports whether the trimmed save input buffer is empty.
func (v QueueView) saveInputIsEmpty() bool {
	return strings.TrimSpace(v.saveInput.Value()) == ""
}
