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
	timeline := v.traversalSnapshot()
	filtered := v.filteredEntries(timeline)
	if move, ok := queueMoves[a]; ok {
		return move(v, len(filtered)), nil
	}
	return v.handleEntryAction(a, filtered, timeline)
}

// queueMoves maps a cursor-movement action to the function performing it, where
// n is the number of entries the filter currently lets through. Actions absent
// from the table do not move the cursor.
var queueMoves = map[keys.Action]func(v View, n int) View{
	keys.ActionCursorUp:     func(v View, _ int) View { return v.cursorUp() },
	keys.ActionCursorDown:   func(v View, n int) View { return v.cursorDown(n) },
	keys.ActionHalfPageDown: func(v View, _ int) View { return v.MoveQueueCursor(v.VisibleRows() / 2) },
	keys.ActionHalfPageUp:   func(v View, _ int) View { return v.MoveQueueCursor(-v.VisibleRows() / 2) },
	keys.ActionFullPageDown: func(v View, _ int) View { return v.MoveQueueCursor(v.VisibleRows()) },
	keys.ActionFullPageUp:   func(v View, _ int) View { return v.MoveQueueCursor(-v.VisibleRows()) },
	keys.ActionGoToTop:      func(v View, _ int) View { v.Cursor, v.ScrollOffset = 0, 0; return v },
	keys.ActionGoToBottom:   func(v View, n int) View { return v.goToBottom(n) },
}

// cursorUp moves the cursor one row up, dragging the scroll window along when
// the cursor would leave through the top.
func (v View) cursorUp() View {
	if v.Cursor > 0 {
		v.Cursor--
	}
	if v.Cursor < v.ScrollOffset {
		v.ScrollOffset = v.Cursor
	}
	return v
}

// cursorDown moves the cursor one row down, scrolling when it would leave
// through the bottom.
func (v View) cursorDown(n int) View {
	if v.Cursor < n-1 {
		v.Cursor++
	}
	if maxRows := v.VisibleRows(); v.Cursor >= v.ScrollOffset+maxRows {
		v.ScrollOffset = v.Cursor - maxRows + 1
	}
	return v
}

// goToBottom jumps to the last entry, scrolling only when it lies past the
// first screenful.
func (v View) goToBottom(n int) View {
	if n <= 0 {
		return v
	}
	v.Cursor = n - 1
	if maxRows := v.VisibleRows(); v.Cursor >= maxRows {
		v.ScrollOffset = v.Cursor - maxRows + 1
	}
	return v
}

// handleEntryAction dispatches the actions that act on the focused entry or on
// the queue as a whole. None of them move the cursor.
func (v View) handleEntryAction(a keys.Action, filtered []Entry, timeline Traversal) (View, tea.Cmd) {
	switch a {
	case keys.ActionClearQueue:
		return v, func() tea.Msg { return ClearQueueMsg{} }
	case keys.ActionMoveTrackUp:
		return v, v.moveTrackUpCmd(timeline)
	case keys.ActionMoveTrackDown:
		return v, v.moveTrackDownCmd(len(filtered), timeline)
	}
	e, ok := v.focusedFilteredEntry(filtered)
	if !ok {
		return v, nil
	}
	switch a {
	case keys.ActionSelect:
		return v, selectEntryCmd(e)
	case keys.ActionDequeue:
		return v, func() tea.Msg { return DequeueTrackMsg{Index: e.OrigIdx} }
	case keys.ActionBulkSelectionToggle:
		return v, func() tea.Msg { return BulkSelectionToggleMsg{Index: e.OrigIdx} }
	}
	return v, nil
}

// focusedFilteredEntry returns the entry under the cursor within the filtered
// view, or false when the cursor sits outside it.
func (v View) focusedFilteredEntry(filtered []Entry) (Entry, bool) {
	if v.Cursor < 0 || v.Cursor >= len(filtered) {
		return Entry{}, false
	}
	return filtered[v.Cursor], true
}

// selectEntryCmd plays the entry, or reports it missing.
//
// Pre-flight stat: the fsnotify watcher is debounced, so a file the user
// deleted moments ago may still appear playable here. Short-circuiting to the
// removal path avoids calling the player (and mpv IPC) synchronously from the
// Update goroutine, which would freeze the UI.
func selectEntryCmd(e Entry) tea.Cmd {
	idx := e.OrigIdx
	if e.Missing || !FileExists(e.Path) {
		return func() tea.Msg {
			return playback.MissingTrackSelectedMsg{Index: idx, Path: e.Path, Hash: e.Hash}
		}
	}
	return func() tea.Msg { return playback.PlayQueueIndexMsg{Index: idx} }
}

// ReorderBlocked reports whether the view is sorted, filtered, or ordered by
// the live playback timeline. Display positions then no longer match queue
// positions, so any edit keyed to a position — a move, a paste — would land on
// the wrong track.
func (v View) ReorderBlocked() bool {
	return v.reorderBlocked(v.traversalSnapshot())
}

func (v View) reorderBlocked(timeline Traversal) bool {
	return v.DisplayOrder != nil || v.timelineActive(timeline) ||
		v.Filter.Text != "" || len(v.TagFilterHashes) > 0
}

func (v View) moveTrackUpCmd(timeline Traversal) tea.Cmd {
	if v.reorderBlocked(timeline) {
		return reorderRefusedCmd()
	}
	if v.Cursor <= 0 {
		return nil
	}
	idx := v.Cursor
	return func() tea.Msg { return MoveTrackUpMsg{Index: idx} }
}

func (v View) moveTrackDownCmd(n int, timeline Traversal) tea.Cmd {
	if v.reorderBlocked(timeline) {
		return reorderRefusedCmd()
	}
	if v.Cursor >= n-1 {
		return nil
	}
	idx := v.Cursor
	return func() tea.Msg { return MoveTrackDownMsg{Index: idx} }
}

func reorderRefusedCmd() tea.Cmd {
	return func() tea.Msg { return statusbar.TransientNotifyMsg("Cannot reorder while sorted/filtered") }
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
