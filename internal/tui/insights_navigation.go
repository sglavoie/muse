package tui

// Panel, window and cursor movement: the controls that decide what the briefing
// is pointed at, without knowing what any of it means. Everything here is
// arithmetic over the row set a panel already produced.

// shiftPanel moves the active panel by delta, wrapping, and resets the row
// cursor since the new panel has a different row set.
func (v InsightsView) shiftPanel(delta int) InsightsView {
	n := len(insightsPanelSpecs)
	idx := 0
	for i, spec := range insightsPanelSpecs {
		if spec.id == v.panel {
			idx = i
			break
		}
	}
	v.panel = insightsPanelSpecs[((idx+delta)%n+n)%n].id
	v.cursor = 0
	v.scrollOffset = 0
	return v
}

// shiftWindow moves the active window by delta, wrapping.
func (v InsightsView) shiftWindow(delta int) InsightsView {
	n := len(insightsWindows)
	v.windowIdx = ((v.windowIdx+delta)%n + n) % n
	return v
}

// shiftResolution moves the active bucket size by delta, wrapping.
func (v InsightsView) shiftResolution(delta int) InsightsView {
	n := len(insightsResolutions)
	v.resolutionIdx = ((v.resolutionIdx+delta)%n + n) % n
	return v
}

func (v *InsightsView) clampCursor() {
	if v.level == insightsLevelSessions {
		if n := len(v.sessionList); n > 0 {
			v.sessionCursor = min(max(v.sessionCursor, 0), n-1)
			v.sessionScroll = adjustScroll(v.sessionCursor, v.sessionScroll, v.sessionBodyRows())
		} else {
			v.sessionCursor, v.sessionScroll = 0, 0
		}
		return
	}
	rows := v.rows()
	if len(rows) == 0 {
		v.cursor = 0
		v.scrollOffset = 0
		return
	}
	v.cursor = min(max(v.cursor, 0), len(rows)-1)
	v.scrollOffset = min(max(v.scrollOffset, 0), v.cursor)
	// Section headers spend body lines alongside the rows under them, so
	// scrolling by row count alone would let the cursor slide under the bottom
	// edge. Data Health is the panel that reaches six sections and shows it.
	budget := v.bodyRows()
	for v.scrollOffset < v.cursor && panelBodyLines(rows, v.scrollOffset, v.cursor) > budget {
		v.scrollOffset++
	}
}

// panelBodyLines counts the lines rows[from..to] occupy once every section
// header between them is counted too.
func panelBodyLines(rows []insightsRow, from, to int) int {
	lines, section := 0, ""
	for i := from; i <= to; i++ {
		if rows[i].section != section {
			section = rows[i].section
			lines++
		}
		lines++
	}
	return lines
}

// bodyRows is how many panel rows fit under the header block.
func (v InsightsView) bodyRows() int {
	return max(v.height-v.headerHeight(), 1)
}

func (v InsightsView) handleCursorAction(a action) InsightsView {
	if v.atTracks() {
		v.trackView = v.trackView.HandleAction(a)
		return v
	}
	if v.level == insightsLevelSessions {
		return v.handleSessionCursorAction(a)
	}
	n := len(v.rows())
	if n == 0 {
		return v
	}
	v.cursor = moveInsightsCursor(v.cursor, n, v.bodyRows(), a)
	v.clampCursor()
	return v
}

func (v InsightsView) handleSessionCursorAction(a action) InsightsView {
	n := len(v.sessionList)
	if n == 0 {
		return v
	}
	v.sessionCursor = moveInsightsCursor(v.sessionCursor, n, v.sessionBodyRows(), a)
	v.clampCursor()
	return v
}

// moveInsightsCursor applies one cursor action. The result is left unclamped
// for the caller's clampCursor, which also owns the scroll offset.
func moveInsightsCursor(cursor, n, page int, a action) int {
	switch a {
	case actionCursorUp:
		cursor--
	case actionCursorDown:
		cursor++
	case actionHalfPageUp:
		cursor -= max(page/2, 1)
	case actionHalfPageDown:
		cursor += max(page/2, 1)
	case actionFullPageUp:
		cursor -= page
	case actionFullPageDown:
		cursor += page
	case actionGoToTop:
		cursor = 0
	case actionGoToBottom:
		cursor = n - 1
	}
	return cursor
}
