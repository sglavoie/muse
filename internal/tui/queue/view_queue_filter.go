package queue

import (
	"strings"

	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/layout"
)

// FilteredEntries returns queue entries matching the current filter text,
// in display order (respecting sort). When no filter is active, all entries
// are returned in display order.
func (v View) FilteredEntries() []Entry {
	n := len(v.Entries)
	if n == 0 {
		return nil
	}

	if v.DisplayOrder != nil && !validDisplayOrder(v.DisplayOrder, n) {
		v.DisplayOrder = nil
	}
	ordered := make([]Entry, n)
	for i := range n {
		idx := i
		if v.DisplayOrder != nil {
			idx = v.DisplayOrder[i]
		}
		ordered[i] = v.Entries[idx]
		ordered[i].OrigIdx = idx
	}

	if len(v.TagFilterHashes) > 0 {
		tagFiltered := make([]Entry, 0, len(ordered))
		for _, e := range ordered {
			if _, ok := v.TagFilterHashes[e.Hash]; ok {
				tagFiltered = append(tagFiltered, e)
			}
		}
		ordered = tagFiltered
	}

	if v.Filter.Text == "" {
		return ordered
	}
	fq := library.ParseFilterQuery(v.Filter.Text)
	var result []Entry
	if fq.IsFieldSpecific() {
		for _, e := range ordered {
			titleLow := strings.ToLower(e.Title)
			artistLow := strings.ToLower(e.Artist)
			albumLow := strings.ToLower(e.Album)
			match := true
			if fq.Title != "" && !strings.Contains(titleLow, strings.ToLower(fq.Title)) {
				match = false
			}
			if match && fq.Artist != "" && !strings.Contains(artistLow, strings.ToLower(fq.Artist)) {
				match = false
			}
			if match && fq.Album != "" && !strings.Contains(albumLow, strings.ToLower(fq.Album)) {
				match = false
			}
			if match && fq.General != "" {
				gen := strings.ToLower(fq.General)
				if !strings.Contains(titleLow, gen) && !strings.Contains(artistLow, gen) && !strings.Contains(albumLow, gen) {
					match = false
				}
			}
			if match {
				result = append(result, e)
			}
		}
	} else {
		query := strings.ToLower(fq.General)
		for _, e := range ordered {
			if strings.Contains(strings.ToLower(e.Title), query) ||
				strings.Contains(strings.ToLower(e.Artist), query) ||
				strings.Contains(strings.ToLower(e.Album), query) {
				result = append(result, e)
			}
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
