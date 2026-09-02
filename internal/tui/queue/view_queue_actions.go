package queue

import (
	"os"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/tui/keys"
	"github.com/sglavoie/muse/internal/tui/playback"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
	"github.com/sglavoie/muse/internal/tui/statusbar"
)

// HandleAction executes a pre-resolved queue action dispatched by root.Update.
// Root resolves the raw key to an action via the keybinding registry and calls
// this method, keeping key-to-action mapping in one authoritative location.
func (v View) HandleAction(a keys.Action) (View, tea.Cmd) {
	filtered := v.FilteredEntries()
	n := len(filtered)

	switch a {
	case keys.ActionCursorUp:
		if v.Cursor > 0 {
			v.Cursor--
		}
		if v.Cursor < v.ScrollOffset {
			v.ScrollOffset = v.Cursor
		}
	case keys.ActionCursorDown:
		if v.Cursor < n-1 {
			v.Cursor++
		}
		maxRows := v.VisibleRows()
		if v.Cursor >= v.ScrollOffset+maxRows {
			v.ScrollOffset = v.Cursor - maxRows + 1
		}
	case keys.ActionHalfPageDown:
		v = v.MoveQueueCursor(v.VisibleRows() / 2)
	case keys.ActionHalfPageUp:
		v = v.MoveQueueCursor(-v.VisibleRows() / 2)
	case keys.ActionFullPageDown:
		v = v.MoveQueueCursor(v.VisibleRows())
	case keys.ActionFullPageUp:
		v = v.MoveQueueCursor(-v.VisibleRows())
	case keys.ActionGoToTop:
		v.Cursor = 0
		v.ScrollOffset = 0
	case keys.ActionGoToBottom:
		if n > 0 {
			v.Cursor = n - 1
			maxRows := v.VisibleRows()
			if v.Cursor >= maxRows {
				v.ScrollOffset = v.Cursor - maxRows + 1
			}
		}
	case keys.ActionSelect:
		if v.Cursor >= 0 && v.Cursor < n {
			e := filtered[v.Cursor]
			idx := e.OrigIdx
			// Pre-flight stat: the fsnotify watcher is debounced, so a file
			// the user deleted moments ago may still appear playable here.
			// Short-circuiting to the removal path avoids calling the player
			// (and mpv IPC) synchronously from the Update goroutine, which
			// would freeze the UI.
			if e.Missing || !FileExists(e.Path) {
				return v, func() tea.Msg {
					return playback.MissingTrackSelectedMsg{Index: idx, Path: e.Path, Hash: e.Hash}
				}
			}
			return v, func() tea.Msg { return playback.PlayQueueIndexMsg{Index: idx} }
		}
	case keys.ActionDequeue:
		if v.Cursor >= 0 && v.Cursor < n {
			e := filtered[v.Cursor]
			idx := e.OrigIdx
			return v, func() tea.Msg { return DequeueTrackMsg{Index: idx} }
		}
	case keys.ActionBulkSelectionToggle:
		if v.Cursor >= 0 && v.Cursor < n {
			e := filtered[v.Cursor]
			idx := e.OrigIdx
			return v, func() tea.Msg { return BulkSelectionToggleMsg{Index: idx} }
		}
	case keys.ActionClearQueue:
		return v, func() tea.Msg { return ClearQueueMsg{} }
	case keys.ActionMoveTrackUp:
		if v.DisplayOrder != nil || v.Filter.Text != "" || len(v.TagFilterHashes) > 0 {
			return v, func() tea.Msg { return statusbar.TransientNotifyMsg("Cannot reorder while sorted/filtered") }
		}
		if v.Cursor > 0 {
			idx := v.Cursor
			return v, func() tea.Msg { return MoveTrackUpMsg{Index: idx} }
		}
	case keys.ActionMoveTrackDown:
		if v.DisplayOrder != nil || v.Filter.Text != "" || len(v.TagFilterHashes) > 0 {
			return v, func() tea.Msg { return statusbar.TransientNotifyMsg("Cannot reorder while sorted/filtered") }
		}
		if v.Cursor < n-1 {
			idx := v.Cursor
			return v, func() tea.Msg { return MoveTrackDownMsg{Index: idx} }
		}
	}
	return v, nil
}

// FileExists reports whether the given path can be stat'd. It returns
// false for any error, including permission-denied: the caller treats a
// non-stat'able path as "cannot play" regardless of the underlying cause.
// Declared as a var so tests that exercise actionSelect with synthetic
// paths can stub the check without creating real temp files.
var FileExists = func(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// MarkMissing marks all entries whose Path matches as missing.
func (v *View) MarkMissing(path string) {
	for i := range v.Entries {
		if v.Entries[i].Path == path {
			v.Entries[i].Missing = true
		}
	}
}

// MarkMissingByHash marks all entries with the given content hash as missing.
// Used when a scan discovers different content at a queued entry's path: the
// old hash's file is gone (overwritten in place), so its queue instances must
// not play the replacement content under stale metadata. Marking by hash — not
// path — keeps a same-content copy queued under a different path untouched.
func (v *View) MarkMissingByHash(hash string) {
	if hash == "" {
		return
	}
	for i := range v.Entries {
		if v.Entries[i].Hash == hash {
			v.Entries[i].Missing = true
		}
	}
}

// IsSavingAsPlaylist reports whether the inline save-as-playlist input is active.
func (v View) IsSavingAsPlaylist() bool {
	return v.SavingAsPlaylist
}

// StartSaveAsPlaylist activates the inline save-as-playlist input with an empty buffer.
// When fromSelection is true the prompt says "Save selection as playlist" and
// only bulk-selected entries will be persisted.
func (v View) StartSaveAsPlaylist(fromSelection bool) View {
	v.SavingAsPlaylist = true
	v.SavingFromSelection = fromSelection
	v.SaveInput = pltui.NewInlineInput("")
	return v
}

// CancelSaveAsPlaylist dismisses the inline save-as-playlist input without persisting.
func (v View) CancelSaveAsPlaylist() View {
	v.SavingAsPlaylist = false
	v.SavingFromSelection = false
	v.SaveInput = textinput.Model{}
	return v
}

// HasSelection reports whether any queue entries are bulk-selected.
func (v View) HasSelection() bool {
	for _, e := range v.Entries {
		if e.Selected {
			return true
		}
	}
	return false
}

// SelectedHashes returns the hashes of all bulk-selected entries, preserving queue order.
func (v View) SelectedHashes() []string {
	var hashes []string
	for _, e := range v.Entries {
		if e.Selected {
			hashes = append(hashes, e.Hash)
		}
	}
	return hashes
}

// UpdateSaveInput forwards a key message to the Bubbles textinput model.
func (v View) UpdateSaveInput(msg tea.KeyPressMsg) (View, tea.Cmd) {
	var cmd tea.Cmd
	v.SaveInput, cmd = v.SaveInput.Update(msg)
	return v, cmd
}

// UpdateSaveInputMsg forwards a generic tea.Msg to the textinput, enabling
// paste and other multi-step commands.
func (v View) UpdateSaveInputMsg(msg tea.Msg) (View, tea.Cmd) {
	var cmd tea.Cmd
	v.SaveInput, cmd = v.SaveInput.Update(msg)
	return v, cmd
}

// SaveInputValue returns the current save input buffer, trimmed of
// leading/trailing whitespace to prevent names like "  hello  ".
func (v View) SaveInputValue() string {
	return strings.TrimSpace(v.SaveInput.Value())
}

// SaveInputIsEmpty reports whether the trimmed save input buffer is empty.
func (v View) SaveInputIsEmpty() bool {
	return strings.TrimSpace(v.SaveInput.Value()) == ""
}
