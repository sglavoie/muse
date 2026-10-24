package queue

import (
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/layout"
)

// FilteredEntries returns queue entries matching the current filter text, in
// display order (respecting sort and the live playback timeline). When no
// filter is active, all entries are returned in display order.
func (v View) FilteredEntries() []Entry {
	return v.filteredEntries(v.traversalSnapshot())
}

// filteredEntries is FilteredEntries against one already-taken timeline
// snapshot, so a caller that also needs the timeline for styling classifies
// rows against the very order it rendered them in.
func (v View) filteredEntries(t Traversal) []Entry {
	ordered := v.orderedEntries(t)
	if len(ordered) == 0 {
		return nil
	}
	if v.TagFilterHashes != nil {
		ordered = v.tagFiltered(ordered)
	}
	if v.Filter.Text == "" {
		return ordered
	}
	return matchingEntries(ordered, library.ParseFilterQuery(v.Filter.Text))
}

// orderedEntries materialises the queue in display order, stamping each entry
// with its index in the underlying queue. Both filters run over the result, so
// stamping here is what keeps every later transformation — filtering, focus,
// selection, dequeue — keyed to the right underlying row.
func (v View) orderedEntries(t Traversal) []Entry {
	n := len(v.Entries)
	if n == 0 {
		return nil
	}
	order := v.effectiveOrder(t)
	ordered := make([]Entry, n)
	for i := range n {
		idx := i
		if order != nil {
			idx = order[i]
		}
		ordered[i] = v.Entries[idx]
		ordered[i].OrigIdx = idx
	}
	return ordered
}

// tagFiltered keeps only the entries whose track carries an active tag.
func (v View) tagFiltered(ordered []Entry) []Entry {
	kept := make([]Entry, 0, len(ordered))
	for _, e := range ordered {
		if _, ok := v.TagFilterHashes[e.Hash]; ok {
			kept = append(kept, e)
		}
	}
	return kept
}

// matchingEntries keeps the entries satisfying fq, preserving their order.
func matchingEntries(ordered []Entry, fq library.FilterQuery) []Entry {
	var result []Entry
	for _, e := range ordered {
		if fq.MatchesMetadata(e.Title, e.Artist, e.Album, e.AlbumArtist, e.Genre, e.Path) {
			result = append(result, e)
		}
	}
	return result
}

func validDisplayOrder(order []int, n int) bool {
	if len(order) != n {
		return false
	}
	for _, idx := range order {
		if idx < 0 || idx >= n {
			return false
		}
	}
	return true
}

// ScrollToNowPlaying adjusts scrollOffset so the row matching
// currentPlayingIndex sits ~⅓ from the top of the visible window, without
// moving the cursor. Returns v unchanged if currentPlayingIndex is negative
// or the row is not present in the filtered view.
func (v View) ScrollToNowPlaying(currentPlayingIndex int) View {
	if currentPlayingIndex < 0 {
		return v
	}
	filtered := v.FilteredEntries()
	row := -1
	for i, e := range filtered {
		if e.OrigIdx == currentPlayingIndex {
			row = i
			break
		}
	}
	if row < 0 {
		return v
	}
	maxRows := v.VisibleRows()
	target := row - maxRows/3
	maxOffset := len(filtered) - maxRows
	if maxOffset < 0 {
		maxOffset = 0
	}
	if target > maxOffset {
		target = maxOffset
	}
	if target < 0 {
		target = 0
	}
	v.ScrollOffset = target
	return v
}

// ShowsEntry reports whether the queue row at origIdx survives the active
// text and tag filters, i.e. whether FocusNowPlaying can land on it.
func (v View) ShowsEntry(origIdx int) bool {
	if origIdx < 0 {
		return false
	}
	for _, e := range v.FilteredEntries() {
		if e.OrigIdx == origIdx {
			return true
		}
	}
	return false
}

// FocusNowPlaying moves the cursor to the entry whose Hash matches
// currentPlayingIndex and adjusts the scroll offset to center it in the viewport.
// Returns v unchanged if no match is found or index is negative.
func (v View) FocusNowPlaying(currentPlayingIndex int) View {
	if currentPlayingIndex < 0 {
		return v
	}
	filtered := v.FilteredEntries()
	idx := -1
	for i, e := range filtered {
		if e.OrigIdx == currentPlayingIndex {
			idx = i
			break
		}
	}
	if idx < 0 {
		return v
	}
	v.Cursor = idx
	maxRows := v.VisibleRows()
	if v.Cursor >= maxRows {
		v.ScrollOffset = v.Cursor - maxRows/2
	} else {
		v.ScrollOffset = 0
	}
	maxOffset := len(filtered) - maxRows
	if maxOffset < 0 {
		maxOffset = 0
	}
	if v.ScrollOffset > maxOffset {
		v.ScrollOffset = maxOffset
	}
	return v
}

// MoveQueueCursor shifts the cursor by delta rows, clamping to [0, len-1],
// and adjusts the scroll offset so the cursor stays visible.
func (v View) MoveQueueCursor(delta int) View {
	n := len(v.FilteredEntries())
	if n == 0 {
		return v
	}
	v.Cursor += delta
	v.Cursor = max(0, min(v.Cursor, n-1))
	maxRows := v.VisibleRows()
	if v.Cursor < v.ScrollOffset {
		v.ScrollOffset = v.Cursor
	} else if v.Cursor >= v.ScrollOffset+maxRows {
		v.ScrollOffset = v.Cursor - maxRows + 1
	}
	return v
}

// VisibleRows returns the maximum number of queue entries that fit on screen.
// Reserve 3 lines for the state strip, column header, and bottom margin,
// plus 1 extra when the save-as-playlist input is active.
func (v View) VisibleRows() int {
	reserve := 3
	if v.SavingAsPlaylist {
		reserve++
	}
	return layout.VisibleRows(v.Height, reserve)
}
