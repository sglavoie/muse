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
	end := itemCount
	if maxRows < itemCount-scrollOffset {
		end = scrollOffset + maxRows
	}
	return scrollOffset, end
}

// VisibleWindowForCursor is VisibleWindow plus cursor tracking: it returns the
// clamped [start, end) index range that keeps the cursor's row on screen.
//
// scrollOffset is clamped against (itemCount - rows) rather than itemCount: an
// offset past the last visible window slice would render an empty body even
// though items (and possibly the cursor) still exist after a delete or filter
// shrunk the list.
//
// The cursor adjustment applies only when 0 <= cursor < itemCount. An
// out-of-range cursor is left alone rather than clamped, so a stale cursor
// (mid-delete, mid-filter) does not yank the window; callers that want
// clamping must clamp explicitly before calling.
//
// label, when non-nil, reports the bucket label of item i; a non-empty label on
// the cursor's item costs one row of the window, because a bucket header will
// be drawn above the data rows.
func VisibleWindowForCursor(cursor, scrollOffset, itemCount, maxRows int, label func(int) string) (start, end int) {
	if itemCount <= 0 || maxRows <= 0 {
		return 0, 0
	}
	rows := dataRows(cursor, itemCount, maxRows, label)
	start = min(max(scrollOffset, 0), max(itemCount-rows, 0))
	start = trackCursor(start, cursor, itemCount, rows)
	return start, min(itemCount, start+rows)
}

// dataRows is maxRows minus the row a bucket header steals above the cursor's
// item. An out-of-range cursor falls back to the first item's label.
func dataRows(cursor, itemCount, maxRows int, label func(int) string) int {
	if label == nil || maxRows <= 1 {
		return maxRows
	}
	labelIdx := cursor
	if labelIdx < 0 || labelIdx >= itemCount {
		labelIdx = 0
	}
	if label(labelIdx) == "" {
		return maxRows
	}
	return maxRows - 1
}

// trackCursor pulls start toward the cursor so its row stays on screen, leaving
// an out-of-range cursor alone.
func trackCursor(start, cursor, itemCount, rows int) int {
	if cursor < 0 || cursor >= itemCount {
		return start
	}
	if cursor < start {
		return max(cursor, 0)
	}
	if cursor >= start+rows {
		return max(cursor-rows+1, 0)
	}
	return max(start, 0)
}

// ClampAfterRemoval adjusts cursor and scrollOffset after items are removed
// from a list of the given length. Returns the clamped values.
func ClampAfterRemoval(cursor, scrollOffset, length int) (int, int) {
	if length == 0 {
		return 0, 0
	}
	if cursor >= length {
		cursor = length - 1
	}
	if cursor < 0 {
		cursor = 0
	}
	maxOffset := length - 1
	if scrollOffset > maxOffset {
		scrollOffset = maxOffset
	}
	// Ensure the cursor is not above the visible window.
	if scrollOffset > cursor {
		scrollOffset = cursor
	}
	return cursor, scrollOffset
}
