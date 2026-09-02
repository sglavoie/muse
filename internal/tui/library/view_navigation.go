package library

import "github.com/sglavoie/muse/internal/tui/layout"

// CursorUp moves the cursor one row up, stopping at the first track.
// The scroll offset is adjusted so the cursor stays visible.
func (v Panel) CursorUp() Panel {
	n := v.effectiveTrackCount()
	if n == 0 {
		v.Cursor = 0
		v.ScrollOffset = 0
		return v
	}
	switch {
	case v.Cursor < 0:
		v.Cursor = 0
	case v.Cursor >= n:
		v.Cursor = n - 1
	case v.Cursor > 0:
		v.Cursor--
	}
	return v.keepCursorVisible()
}

// CursorDown moves the cursor one row down, stopping at the last track.
// The scroll offset is adjusted so the cursor stays visible.
func (v Panel) CursorDown() Panel {
	n := v.effectiveTrackCount()
	if n == 0 {
		v.Cursor = 0
		v.ScrollOffset = 0
		return v
	}
	switch {
	case v.Cursor < 0:
		v.Cursor = 0
	case v.Cursor >= n:
		v.Cursor = n - 1
	case v.Cursor < n-1:
		v.Cursor++
	}
	return v.keepCursorVisible()
}

// HalfPageDown moves the cursor half a visible page down.
func (v Panel) HalfPageDown() Panel {
	return v.moveCursor(v.halfPageRows())
}

// HalfPageUp moves the cursor half a visible page up.
func (v Panel) HalfPageUp() Panel {
	return v.moveCursor(-v.halfPageRows())
}

// FullPageDown moves the cursor a full visible page down.
func (v Panel) FullPageDown() Panel {
	return v.moveCursor(v.visibleRows())
}

// FullPageUp moves the cursor a full visible page up.
func (v Panel) FullPageUp() Panel {
	return v.moveCursor(-v.visibleRows())
}

// GoToTop moves the cursor to the first row.
func (v Panel) GoToTop() Panel {
	v.Cursor = 0
	v.ScrollOffset = 0
	return v
}

// GoToBottom moves the cursor to the last row.
func (v Panel) GoToBottom() Panel {
	n := v.effectiveTrackCount()
	if n == 0 {
		v.Cursor = 0
		v.ScrollOffset = 0
		return v
	}
	v.Cursor = n - 1
	maxRows := v.scrollRows()
	v.ScrollOffset = max(0, v.Cursor-maxRows+1)
	return v
}

// moveCursor shifts the cursor by delta rows, clamping to [0, trackCount-1],
// and adjusts the scroll offset so the cursor stays visible.
func (v Panel) moveCursor(delta int) Panel {
	n := v.effectiveTrackCount()
	if n == 0 {
		v.Cursor = 0
		v.ScrollOffset = 0
		return v
	}
	v.Cursor += delta
	v.Cursor = max(0, min(v.Cursor, n-1))
	return v.keepCursorVisible()
}

func (v Panel) keepCursorVisible() Panel {
	if v.ScrollOffset < 0 {
		v.ScrollOffset = 0
	}
	maxRows := v.scrollRows()
	if v.Cursor < v.ScrollOffset {
		v.ScrollOffset = v.Cursor
	} else if v.Cursor >= v.ScrollOffset+maxRows {
		v.ScrollOffset = v.Cursor - maxRows + 1
	}
	if v.ScrollOffset < 0 {
		v.ScrollOffset = 0
	}
	return v
}

func (v Panel) scrollRows() int {
	return max(1, v.visibleRows())
}

func (v Panel) halfPageRows() int {
	rows := v.visibleRows()
	if rows <= 0 {
		return 0
	}
	return max(1, rows/2)
}

// visibleRows returns the maximum number of track rows that fit on screen.
func (v Panel) visibleRows() int {
	if v.Width == 0 {
		reserve := 0
		if v.Filter.Active || v.Filter.Text != "" {
			reserve = 1
		}
		return layout.VisibleRows(v.Height, reserve)
	}
	reserve := 2 // state strip + column header
	return layout.VisibleRows(v.Height, reserve)
}
