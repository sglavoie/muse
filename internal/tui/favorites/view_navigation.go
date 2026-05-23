package favorites

// CursorUp returns a copy of v with the cursor moved up by one row.
func (v View) CursorUp() View {
	n := v.itemCount()
	if n == 0 {
		v.Cursor = 0
		v.ScrollOffset = 0
		return v
	}
	if v.Cursor > 0 {
		v.Cursor--
	}
	v.Cursor = max(0, min(v.Cursor, n-1))
	return v.keepCursorVisible()
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
	if v.Cursor < limit {
		v.Cursor++
	}
	v.Cursor = max(0, min(v.Cursor, n-1))
	return v.keepCursorVisible()
}

// HalfPageDown moves the cursor half a visible page down.
func (v View) HalfPageDown() View { return v.moveCursor(v.halfPageDelta()) }

// HalfPageUp moves the cursor half a visible page up.
func (v View) HalfPageUp() View { return v.moveCursor(-v.halfPageDelta()) }

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
	return v.keepCursorVisible()
}

// moveCursor shifts the cursor by delta rows, clamping to [0, itemCount-1].
func (v View) moveCursor(delta int) View {
	n := v.itemCount()
	if n == 0 {
		v.Cursor = 0
		v.ScrollOffset = 0
		return v
	}
	v.Cursor += delta
	v.Cursor = max(0, min(v.Cursor, n-1))
	return v.keepCursorVisible()
}

// keepCursorVisible adjusts ScrollOffset so the current cursor remains inside
// the visible window. When no rows fit, anchor the scroll to the cursor rather
// than advancing past it, so a later resize does not strand the selection.
func (v View) keepCursorVisible() View {
	if v.ScrollOffset < 0 {
		v.ScrollOffset = 0
	}
	maxRows := v.visibleRows()
	if maxRows <= 0 {
		v.ScrollOffset = v.Cursor
		if v.ScrollOffset < 0 {
			v.ScrollOffset = 0
		}
		return v
	}
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

func (v View) halfPageDelta() int {
	return max(1, v.visibleRows()/2)
}
