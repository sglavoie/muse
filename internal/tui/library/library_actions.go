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

// panelMoves maps a cursor-movement action to the Panel method performing it.
// Actions absent from the table leave the cursor untouched.
var panelMoves = map[keys.Action]func(Panel) Panel{
	keys.ActionCursorUp:     Panel.CursorUp,
	keys.ActionCursorDown:   Panel.CursorDown,
	keys.ActionHalfPageDown: Panel.HalfPageDown,
	keys.ActionHalfPageUp:   Panel.HalfPageUp,
	keys.ActionFullPageDown: Panel.FullPageDown,
	keys.ActionFullPageUp:   Panel.FullPageUp,
	keys.ActionGoToTop:      Panel.GoToTop,
	keys.ActionGoToBottom:   Panel.GoToBottom,
}

// HandleAction executes a pre-resolved library action dispatched by root.Update.
func (v View) HandleAction(a keys.Action) View {
	panel := v.Panel()
	if a == keys.ActionBulkSelectionToggle {
		if t, ok := panel.FocusedTrack(); ok {
			return v.ToggleBulkSelection(t.Hash)
		}
		return v
	}
	move, ok := panelMoves[a]
	if !ok {
		return v
	}
	panel = move(panel)
	v.Cursor = panel.Cursor
	v.ScrollOffset = panel.ScrollOffset
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
