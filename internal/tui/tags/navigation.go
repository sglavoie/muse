package tags

import (
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/tui/layout"
)

// visibleRows returns the maximum number of data rows that fit on screen.
func (v View) visibleRows() int {
	var reserve int
	switch v.Level {
	case TagLevel:
		if v.FilterActive || v.FilterText != "" {
			reserve = 1
		}
	default: // TrackLevel
		reserve = 3 // heading + separator + column header
	}
	if v.SortIndicator != "" {
		reserve++
	}
	return layout.VisibleRows(v.Height, reserve)
}

// itemCount returns the number of items in the current level.
func (v View) itemCount() int {
	if v.Level == TrackLevel {
		return len(v.EffectiveTracks())
	}
	return len(v.EffectiveTags())
}

// visibleWindow returns the clamped [start,end) indexes for the visible slice
// window. Render paths can receive stale scroll offsets after filters, deletes,
// or async count refreshes shrink a list, so clamp both low and high bounds.
func (v View) visibleWindow(itemCount, rows int) (int, int) {
	if itemCount <= 0 || rows <= 0 {
		return 0, 0
	}
	maxStart := max(itemCount-rows, 0)
	start := min(max(v.ScrollOffset, 0), maxStart)
	if v.Cursor >= 0 {
		cursor := min(v.Cursor, itemCount-1)
		if cursor < start {
			start = cursor
		} else if cursor >= start+rows {
			start = cursor - rows + 1
		}
	}
	start = min(max(start, 0), maxStart)
	end := min(itemCount, start+rows)
	return start, end
}

// CursorUp returns a copy of v with the cursor moved up by one row.
func (v View) CursorUp() View {
	n := v.itemCount()
	if n == 0 {
		v.Cursor = 0
		v.ScrollOffset = 0
		return v
	}
	if v.Cursor > n-1 {
		v.Cursor = n - 1
	} else if v.Cursor > 0 {
		v.Cursor--
	}
	if v.Cursor < 0 {
		v.Cursor = 0
	}
	if v.visibleRows() <= 0 {
		v.ScrollOffset = v.Cursor
		return v
	}
	if v.Cursor < v.ScrollOffset {
		v.ScrollOffset = v.Cursor
	}
	return v
}

// CursorDown returns a copy of v with the cursor moved down by one row.
func (v View) CursorDown() View {
	n := v.itemCount()
	if n == 0 {
		v.Cursor = 0
		v.ScrollOffset = 0
		return v
	}
	limit := n - 1
	if v.Cursor > limit {
		v.Cursor = limit
	} else if v.Cursor < limit {
		v.Cursor++
	}
	if v.Cursor < 0 {
		v.Cursor = 0
	}
	maxRows := v.visibleRows()
	if maxRows <= 0 {
		v.ScrollOffset = v.Cursor
		return v
	}
	// Clamp a stale ScrollOffset down when the list has shrunk below it:
	// otherwise the cursor renders outside the visible window.
	if v.ScrollOffset > v.Cursor {
		v.ScrollOffset = v.Cursor
	}
	if v.Cursor >= v.ScrollOffset+maxRows {
		v.ScrollOffset = v.Cursor - maxRows + 1
	}
	return v
}

// HalfPageDown moves the cursor half a visible page down.
func (v View) HalfPageDown() View { return v.moveCursor(v.visibleRows() / 2) }

// HalfPageUp moves the cursor half a visible page up.
func (v View) HalfPageUp() View { return v.moveCursor(-v.visibleRows() / 2) }

// FullPageDown moves the cursor a full visible page down.
func (v View) FullPageDown() View { return v.moveCursor(v.visibleRows()) }

// FullPageUp moves the cursor a full visible page up.
func (v View) FullPageUp() View { return v.moveCursor(-v.visibleRows()) }

// GoToTop moves the cursor to the first row.
func (v View) GoToTop() View {
	v.Cursor = 0
	v.ScrollOffset = 0
	return v
}

// GoToBottom moves the cursor to the last row.
func (v View) GoToBottom() View {
	n := v.itemCount()
	if n == 0 {
		v.Cursor = 0
		v.ScrollOffset = 0
		return v
	}
	v.Cursor = n - 1
	maxRows := v.visibleRows()
	if v.Cursor >= maxRows {
		v.ScrollOffset = v.Cursor - maxRows + 1
	}
	// Clamp ScrollOffset down when the list shrank below a previously
	// scrolled position: otherwise the cursor renders above the visible
	// window after a filter/delete reduces itemCount.
	if v.ScrollOffset > v.Cursor {
		v.ScrollOffset = v.Cursor
	}
	if v.ScrollOffset < 0 {
		v.ScrollOffset = 0
	}
	return v
}

func (v View) moveCursor(delta int) View {
	n := v.itemCount()
	if n == 0 {
		v.Cursor = 0
		v.ScrollOffset = 0
		return v
	}
	v.Cursor += delta
	v.Cursor = max(0, min(v.Cursor, n-1))
	maxRows := v.visibleRows()
	if maxRows <= 0 {
		v.ScrollOffset = v.Cursor
		return v
	}
	if v.Cursor < v.ScrollOffset {
		v.ScrollOffset = v.Cursor
	} else if v.Cursor >= v.ScrollOffset+maxRows {
		v.ScrollOffset = v.Cursor - maxRows + 1
	}
	return v
}

// OpenTracks returns a copy of v transitioned to TrackLevel with tracks
// loaded. The current tag cursor and scroll offset are saved for restoration
// by Back. When a filter is active, the saved cursor is translated back to the
// unfiltered index of the focused tag so the cursor lands on the same entry
// after Back clears the filter.
func (v View) OpenTracks(tag db.TagRow, tracks []db.PlaylistTrackRow) View {
	v.TagCursor, v.TagScrollOffset = v.unfilteredTagCursorPos()
	v.SelectedTag = tag
	v.Tracks = tracks
	v.Level = TrackLevel
	v.Cursor = 0
	v.ScrollOffset = 0
	v.FilterActive = false
	v.FilterText = ""
	v.SelectedHashes = nil
	return v
}

// unfilteredTagCursorPos returns the cursor translated to the
// post-Back-clear EffectiveTags slice (i.e. v.Tags minus zero-count entries)
// when a filter is active. ScrollOffset is reset to 0 so clampRestoredCursor
// (run by Back) can re-establish a visible window for the restored cursor.
// Falls back to the raw values when no filter is active or the focused tag
// cannot be resolved.
func (v View) unfilteredTagCursorPos() (int, int) {
	if v.FilterText == "" {
		return v.Cursor, v.ScrollOffset
	}
	items := v.EffectiveTags()
	if len(items) == 0 || v.Cursor < 0 || v.Cursor >= len(items) {
		return v.Cursor, v.ScrollOffset
	}
	focused := items[v.Cursor]
	// Compute index inside the filter-cleared EffectiveTags slice (Tags
	// minus zero-count entries), which is what Back's clampRestoredCursor
	// interprets. Translating to v.Tags directly would skew past zero-count
	// tags, leaving the cursor on a different tag after Back clears the
	// filter.
	idx := 0
	for _, t := range v.Tags {
		if v.TagCounts != nil && v.TagCounts[t.ID] == 0 {
			continue
		}
		if t.ID == focused.ID {
			return idx, 0
		}
		idx++
	}
	return v.Cursor, v.ScrollOffset
}

// Back returns a copy of v navigated one level up. At TrackLevel it returns to
// TagLevel. At TagLevel it is a no-op. The restored cursor/scroll are clamped
// against the parent list size so that mutations (e.g. a deleted tag or a tag
// whose count dropped to zero) during the drill-down do not leave the cursor
// pointing past the new end.
func (v View) Back() View {
	if v.Level == TrackLevel {
		v.Level = TagLevel
		v.Cursor = v.TagCursor
		v.ScrollOffset = v.TagScrollOffset
		v.Tracks = nil
		v.FilterActive = false
		v.FilterText = ""
		v.SelectedHashes = nil
		v = v.clampRestoredCursor()
	}
	return v
}

// clampRestoredCursor clamps Cursor to [0, itemCount-1] and adjusts
// ScrollOffset to keep the cursor visible. Used after Back() restores a saved
// cursor that may now point past the (possibly shrunk) parent list. When the
// parent slice is nil the clamp is skipped: callers that build a temporary
// View without populating the parent slice (and clamp themselves) rely on this
// pass-through behaviour. An empty (non-nil) parent triggers a reset to 0.
func (v View) clampRestoredCursor() View {
	if v.Tags == nil {
		return v
	}
	n := v.itemCount()
	if n == 0 {
		v.Cursor = 0
		v.ScrollOffset = 0
		return v
	}
	if v.Cursor < 0 {
		v.Cursor = 0
	}
	if v.Cursor > n-1 {
		v.Cursor = n - 1
	}
	if v.ScrollOffset > v.Cursor {
		v.ScrollOffset = v.Cursor
	}
	if v.ScrollOffset < 0 {
		v.ScrollOffset = 0
	}
	// Ensure the cursor remains visible when the saved ScrollOffset is far
	// below the (possibly clamped) Cursor — otherwise the cursor renders off
	// the bottom of the visible window after Back().
	maxRows := v.visibleRows()
	if maxRows <= 0 {
		v.ScrollOffset = v.Cursor
	} else if v.Cursor >= v.ScrollOffset+maxRows {
		v.ScrollOffset = v.Cursor - maxRows + 1
	}
	return v
}
