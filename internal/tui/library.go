package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
	libtui "github.com/sglavoie/muse/internal/tui/library"
)

// LibraryView renders the music library panel backed by a sorted LibraryIndex.
type LibraryView struct {
	index          library.LibraryIndex
	width          int
	height         int
	cursor         int // zero-based index of the focused track row
	filterActive   bool
	filterText     string
	fuzzy          library.FuzzySearcher
	selectedHashes map[string]struct{} // bulk-selected track hashes
}

// render returns a string representation of the library panel with proportional
// columnar layout. Column widths resize with the terminal width.
func (v LibraryView) render() string {
	return v.view().Render()
}

// view returns a fully-populated libtui.View from the current LibraryView state.
func (v LibraryView) view() libtui.View {
	return libtui.View{
		Index:          v.index,
		Width:          v.width,
		Height:         v.height,
		Cursor:         v.cursor,
		FilterActive:   v.filterActive,
		FilterText:     v.filterText,
		Fuzzy:          v.fuzzy,
		SelectedHashes: v.selectedHashes,
	}
}

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
	case actionBulkSelectionToggle:
		if t, ok := view.FocusedTrack(); ok {
			return v.ToggleBulkSelection(t.Hash)
		}
		return v
	}
	v.cursor = view.Cursor
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
	for _, t := range v.index.Tracks() {
		if _, ok := v.selectedHashes[t.Hash]; !ok {
			continue
		}
		track := t
		wasEmpty := queueLen == 0
		queueLen++
		cmds = append(cmds, func() tea.Msg {
			id, err := qa.AppendTrack(track)
			return libtui.TrackEnqueuedMsg{Track: track, QueueID: id, Err: err, WasEmpty: wasEmpty}
		})
	}
	v = v.ClearSelection()
	if len(cmds) == 0 {
		return v, nil
	}
	return v, tea.Batch(cmds...)
}

// ActivateFilter sets the filter bar to active.
func (v LibraryView) ActivateFilter() LibraryView {
	v.filterActive = true
	return v
}

// DeactivateFilter clears the filter query, deactivates the filter bar, and
// resets the cursor to 0.
func (v LibraryView) DeactivateFilter() LibraryView {
	v.filterActive = false
	v.filterText = ""
	v.cursor = 0
	return v
}

// AppendFilterChar appends ch to the filter text and resets the cursor to 0.
// It is a no-op when the filter bar is not active.
func (v LibraryView) AppendFilterChar(ch rune) LibraryView {
	if !v.filterActive {
		return v
	}
	v.filterText += string(ch)
	v.cursor = 0
	return v
}

// BackspaceFilter removes the last rune from the filter text and resets the
// cursor to 0. It is a no-op when the filter bar is not active or the text is
// already empty.
func (v LibraryView) BackspaceFilter() LibraryView {
	if !v.filterActive || len(v.filterText) == 0 {
		return v
	}
	runes := []rune(v.filterText)
	v.filterText = string(runes[:len(runes)-1])
	v.cursor = 0
	return v
}

// formatCount formats n with comma separators (e.g. 1234 → "1,234").
func formatCount(n int) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	offset := len(s) % 3
	var result []byte
	for i := range len(s) {
		if i > 0 && (i-offset)%3 == 0 {
			result = append(result, ',')
		}
		result = append(result, s[i])
	}
	return string(result)
}
