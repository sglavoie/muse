package tui

import (
	tea "charm.land/bubbletea/v2"
	libtui "github.com/sglavoie/muse/internal/tui/library"
)

// ToggleBulkSelection toggles hash in the selected set. If hash is empty it is
// a no-op. The map is lazily initialised on first toggle.
func (v LibraryView) ToggleBulkSelection(hash string) LibraryView {
	if hash == "" {
		return v
	}
	if v.selectedHashes == nil {
		v.selectedHashes = make(map[string]struct{})
	}
	if _, ok := v.selectedHashes[hash]; ok {
		delete(v.selectedHashes, hash)
	} else {
		v.selectedHashes[hash] = struct{}{}
	}
	return v
}

// ClearFilter deactivates the filter bar, resets cursor/scroll, and rebuilds
// the filter cache. It is a no-op when no filter text is active.
func (v LibraryView) ClearFilter() LibraryView {
	if v.filter.Text == "" {
		return v
	}
	v.filter = v.filter.Deactivate()
	v.cursor = 0
	v.scrollOffset = 0
	return v.rebuildFilterCache()
}

// ClearSelection removes all bulk-selection markers.
func (v LibraryView) ClearSelection() LibraryView {
	v.selectedHashes = nil
	return v
}

// HandleAction executes a pre-resolved library action dispatched by root.Update.
func (v LibraryView) HandleAction(a action) LibraryView {
	view := v.view()
	switch a {
	case actionCursorUp:
		view = view.CursorUp()
	case actionCursorDown:
		view = view.CursorDown()
	case actionHalfPageDown:
		view = view.HalfPageDown()
	case actionHalfPageUp:
		view = view.HalfPageUp()
	case actionFullPageDown:
		view = view.FullPageDown()
	case actionFullPageUp:
		view = view.FullPageUp()
	case actionGoToTop:
		view = view.GoToTop()
	case actionGoToBottom:
		view = view.GoToBottom()
	case actionBulkSelectionToggle:
		if t, ok := view.FocusedTrack(); ok {
			return v.ToggleBulkSelection(t.Hash)
		}
		return v
	}
	v.cursor = view.Cursor
	v.scrollOffset = view.ScrollOffset
	return v
}

// EnqueueFocused dispatches a tea.Cmd that appends the focused track to the
// play queue via qa. It uses the full filter state stored on LibraryView so
// that the correct filtered-result track is selected when a filter is active.
// Returns nil when the library is empty or the cursor is out of range.
func (v LibraryView) EnqueueFocused(qa libtui.QueueAppender, prevQueueLen int) tea.Cmd {
	_, cmd := v.view().EnqueueFocused(qa, prevQueueLen)
	return cmd
}

// PlayNextFocused dispatches a tea.Cmd that inserts the focused track right
// after the currently playing track. insertAfter is the queue index to insert
// at. Returns nil when the library is empty or the cursor is out of range.
func (v LibraryView) PlayNextFocused(qa libtui.QueueAppender, prevQueueLen, insertAfter int) tea.Cmd {
	_, cmd := v.view().PlayNextFocused(qa, prevQueueLen, insertAfter)
	return cmd
}

// EnqueueSelected dispatches a tea.Cmd that appends all bulk-selected tracks to
// the play queue via qa, in index order, then clears the selection. Returns the
// updated view (with selection cleared) and a nil Cmd when no tracks are
// selected. prevQueueLen is used to set WasEmpty on the first enqueued track so
// the root model can trigger immediate playback when the queue was empty.
func (v LibraryView) EnqueueSelected(qa libtui.QueueAppender, prevQueueLen int) (LibraryView, tea.Cmd) {
	if len(v.selectedHashes) == 0 {
		return v, nil
	}
	var cmds []tea.Cmd
	queueLen := prevQueueLen
	tracks := v.baseTracks()
	if v.sortedTracks != nil {
		tracks = v.sortedTracks
	}
	for _, t := range tracks {
		if _, ok := v.selectedHashes[t.Hash]; !ok {
			continue
		}
		track := t
		wasEmpty := queueLen == 0
		queueLen++
		cmds = append(cmds, func() tea.Msg {
			id, err := qa.AppendTrack(track, "")
			return libtui.TrackEnqueuedMsg{Track: track, QueueID: id, Err: err, WasEmpty: wasEmpty, InsertAfter: -1}
		})
	}
	v = v.ClearSelection()
	if len(cmds) == 0 {
		return v, nil
	}
	return v, tea.Batch(cmds...)
}
