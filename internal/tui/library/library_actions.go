package library

import (
	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/tui/keys"
)

// ToggleBulkSelection toggles hash in the selected set. If hash is empty it is
// a no-op. The map is lazily initialised on first toggle.
func (v View) ToggleBulkSelection(hash string) View {
	if hash == "" {
		return v
	}
	if v.SelectedHashes == nil {
		v.SelectedHashes = make(map[string]struct{})
	}
	if _, ok := v.SelectedHashes[hash]; ok {
		delete(v.SelectedHashes, hash)
	} else {
		v.SelectedHashes[hash] = struct{}{}
	}
	return v
}

// ClearFilter deactivates the filter bar, resets cursor/scroll, and rebuilds
// the filter cache. It is a no-op when no filter text is active.
func (v View) ClearFilter() View {
	if v.Filter.Text == "" {
		return v
	}
	v.Filter = v.Filter.Deactivate()
	v.Cursor = 0
	v.ScrollOffset = 0
	return v.RebuildFilterCache()
}

// ClearSelection removes all bulk-selection markers.
func (v View) ClearSelection() View {
	v.SelectedHashes = nil
	return v
}

// HandleAction executes a pre-resolved library action dispatched by root.Update.
func (v View) HandleAction(a keys.Action) View {
	view := v.Panel()
	switch a {
	case keys.ActionCursorUp:
		view = view.CursorUp()
	case keys.ActionCursorDown:
		view = view.CursorDown()
	case keys.ActionHalfPageDown:
		view = view.HalfPageDown()
	case keys.ActionHalfPageUp:
		view = view.HalfPageUp()
	case keys.ActionFullPageDown:
		view = view.FullPageDown()
	case keys.ActionFullPageUp:
		view = view.FullPageUp()
	case keys.ActionGoToTop:
		view = view.GoToTop()
	case keys.ActionGoToBottom:
		view = view.GoToBottom()
	case keys.ActionBulkSelectionToggle:
		if t, ok := view.FocusedTrack(); ok {
			return v.ToggleBulkSelection(t.Hash)
		}
		return v
	}
	v.Cursor = view.Cursor
	v.ScrollOffset = view.ScrollOffset
	return v
}

// EnqueueFocused dispatches a tea.Cmd that appends the focused track to the
// play queue via qa. It uses the full filter state stored on View so
// that the correct filtered-result track is selected when a filter is active.
// Returns nil when the library is empty or the cursor is out of range.
func (v View) EnqueueFocused(qa QueueAppender, prevQueueLen int) tea.Cmd {
	_, cmd := v.Panel().EnqueueFocused(qa, prevQueueLen)
	return cmd
}

// PlayNextFocused dispatches a tea.Cmd that inserts the focused track right
// after the currently playing track. insertAfter is the queue index to insert
// at. Returns nil when the library is empty or the cursor is out of range.
func (v View) PlayNextFocused(qa QueueAppender, prevQueueLen, insertAfter int) tea.Cmd {
	_, cmd := v.Panel().PlayNextFocused(qa, prevQueueLen, insertAfter)
	return cmd
}
