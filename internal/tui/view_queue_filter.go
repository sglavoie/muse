package tui

import (
	"strings"

	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/layout"
)

// filteredEntries returns queue entries matching the current filter text,
// in display order (respecting sort). When no filter is active, all entries
// are returned in display order.
func (v QueueView) filteredEntries() []QueueEntry {
	n := len(v.entries)
	if n == 0 {
		return nil
	}

	if v.displayOrder != nil && len(v.displayOrder) != n {
		v.displayOrder = nil
	}
	ordered := make([]QueueEntry, n)
	for i := range n {
		idx := i
		if v.displayOrder != nil {
			idx = v.displayOrder[i]
		}
		ordered[i] = v.entries[idx]
		ordered[i].origIdx = idx
	}

	if len(v.tagFilterHashes) > 0 {
		tagFiltered := make([]QueueEntry, 0, len(ordered))
		for _, e := range ordered {
			if _, ok := v.tagFilterHashes[e.Hash]; ok {
				tagFiltered = append(tagFiltered, e)
			}
		}
		ordered = tagFiltered
	}

	if v.filter.Text == "" {
		return ordered
	}
	fq := library.ParseFilterQuery(v.filter.Text)
	var result []QueueEntry
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

// FocusNowPlaying moves the cursor to the entry whose Hash matches
// currentPlayingIndex and adjusts the scroll offset to center it in the viewport.
// Returns v unchanged if no match is found or index is negative.
func (v QueueView) FocusNowPlaying(currentPlayingIndex int) QueueView {
	if currentPlayingIndex < 0 {
		return v
	}
	filtered := v.filteredEntries()
	idx := -1
	for i, e := range filtered {
		if e.origIdx == currentPlayingIndex {
			idx = i
			break
		}
	}
	if idx < 0 {
		return v
	}
	v.cursor = idx
	maxRows := v.visibleRows()
	if v.cursor >= maxRows {
		v.scrollOffset = v.cursor - maxRows/2
	} else {
		v.scrollOffset = 0
	}
	return v
}

// moveQueueCursor shifts the cursor by delta rows, clamping to [0, len-1],
// and adjusts the scroll offset so the cursor stays visible.
func (v QueueView) moveQueueCursor(delta int) QueueView {
	n := len(v.filteredEntries())
	if n == 0 {
		return v
	}
	v.cursor += delta
	v.cursor = max(0, min(v.cursor, n-1))
	maxRows := v.visibleRows()
	if v.cursor < v.scrollOffset {
		v.scrollOffset = v.cursor
	} else if v.cursor >= v.scrollOffset+maxRows {
		v.scrollOffset = v.cursor - maxRows + 1
	}
	return v
}

// visibleRows returns the maximum number of queue entries that fit on screen.
// Reserve 2 lines for the column header and bottom margin, plus 1 extra when
// the filter bar is visible (active or committed with non-empty text), and
// 1 extra when the save-as-playlist input is active.
func (v QueueView) visibleRows() int {
	reserve := 2
	if v.filter.Active || v.filter.Text != "" {
		reserve++
	}
	if v.savingAsPlaylist {
		reserve++
	}
	return layout.VisibleRows(v.height, reserve)
}
