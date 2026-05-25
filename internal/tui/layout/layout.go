package layout

// VisibleRows returns the maximum number of content rows that fit on screen
// given the total height and the number of lines reserved for chrome (headers,
// margins, etc.). When height is zero or negative (before the first
// WindowSizeMsg) a safe default of 50 is returned.
func VisibleRows(height, reserve int) int {
	if reserve < 0 {
		reserve = 0
	}
	if height > 0 {
		return max(0, height-reserve)
	}
	return 50
}

// VisibleWindow returns the clamped [start, end) index range of items that
// are visible given a scroll offset, total item count, and maximum displayable
// rows. Both start and end are 0 when there is nothing to display.
func VisibleWindow(scrollOffset, itemCount, maxRows int) (int, int) {
	if itemCount <= 0 || maxRows <= 0 {
		return 0, 0
	}
	if scrollOffset < 0 {
		scrollOffset = 0
	}
	maxStart := itemCount - maxRows
	if maxStart < 0 {
		maxStart = 0
	}
	if scrollOffset > maxStart {
		scrollOffset = maxStart
	}
	end := scrollOffset + maxRows
	if end > itemCount {
		end = itemCount
	}
	return scrollOffset, end
}
