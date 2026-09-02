package insights

import (
	"github.com/sglavoie/muse/internal/tui/keys"
	"github.com/sglavoie/muse/internal/tui/sorting"
)

// Panel, window and cursor movement: the controls that decide what the briefing
// is pointed at, without knowing what any of it means. Everything here is
// arithmetic over the row set a panel already produced.

// ShiftPanel moves the active panel by delta, wrapping, and resets the row
// cursor since the new panel has a different row set.
func (v View) ShiftPanel(delta int) View {
	n := len(insightsPanelSpecs)
	idx := 0
	for i, spec := range insightsPanelSpecs {
		if spec.id == v.Panel {
			idx = i
			break
		}
	}
	v.Panel = insightsPanelSpecs[((idx+delta)%n+n)%n].id
	v.Cursor = 0
	v.ScrollOffset = 0
	return v
}

// ShiftWindow moves the active window by delta, wrapping.
func (v View) ShiftWindow(delta int) View {
	n := len(Windows)
	v.WindowIdx = ((v.WindowIdx+delta)%n + n) % n
	return v
}

// ShiftResolution moves the active bucket size by delta, wrapping.
func (v View) ShiftResolution(delta int) View {
	n := len(insightsResolutions)
	v.resolutionIdx = ((v.resolutionIdx+delta)%n + n) % n
	return v
}

func (v *View) ClampCursor() {
	if v.Level == LevelSessions {
		if n := len(v.SessionList); n > 0 {
			v.sessionCursor = min(max(v.sessionCursor, 0), n-1)
			v.sessionScroll = sorting.AdjustScroll(v.sessionCursor, v.sessionScroll, v.sessionBodyRows())
		} else {
			v.sessionCursor, v.sessionScroll = 0, 0
		}
		return
	}
	rows := v.Rows()
	if len(rows) == 0 {
		v.Cursor = 0
		v.ScrollOffset = 0
		return
	}
	v.Cursor = min(max(v.Cursor, 0), len(rows)-1)
	v.ScrollOffset = min(max(v.ScrollOffset, 0), v.Cursor)
	// Section headers spend body lines alongside the rows under them, so
	// scrolling by row count alone would let the cursor slide under the bottom
	// edge. Data Health is the panel that reaches six sections and shows it.
	budget := v.BodyRows()
	for v.ScrollOffset < v.Cursor && PanelBodyLines(rows, v.ScrollOffset, v.Cursor) > budget {
		v.ScrollOffset++
	}
}

// PanelBodyLines counts the lines rows[from..to] occupy once every section
// header between them is counted too.
func PanelBodyLines(rows []Row, from, to int) int {
	lines, section := 0, ""
	for i := from; i <= to; i++ {
		if rows[i].Section != section {
			section = rows[i].Section
			lines++
		}
		lines++
	}
	return lines
}

// BodyRows is how many panel rows fit under the header block.
func (v View) BodyRows() int {
	return max(v.Height-v.headerHeight(), 1)
}

func (v View) HandleCursorAction(a keys.Action) View {
	if v.AtTracks() {
		v.TrackView = v.TrackView.HandleAction(a)
		return v
	}
	if v.Level == LevelSessions {
		return v.handleSessionCursorAction(a)
	}
	n := len(v.Rows())
	if n == 0 {
		return v
	}
	v.Cursor = moveInsightsCursor(v.Cursor, n, v.BodyRows(), a)
	v.ClampCursor()
	return v
}

func (v View) handleSessionCursorAction(a keys.Action) View {
	n := len(v.SessionList)
	if n == 0 {
		return v
	}
	v.sessionCursor = moveInsightsCursor(v.sessionCursor, n, v.sessionBodyRows(), a)
	v.ClampCursor()
	return v
}

// moveInsightsCursor applies one cursor action. The result is left unclamped
// for the caller's clampCursor, which also owns the scroll offset.
func moveInsightsCursor(cursor, n, page int, a keys.Action) int {
	switch a {
	case keys.ActionCursorUp:
		cursor--
	case keys.ActionCursorDown:
		cursor++
	case keys.ActionHalfPageUp:
		cursor -= max(page/2, 1)
	case keys.ActionHalfPageDown:
		cursor += max(page/2, 1)
	case keys.ActionFullPageUp:
		cursor -= page
	case keys.ActionFullPageDown:
		cursor += page
	case keys.ActionGoToTop:
		cursor = 0
	case keys.ActionGoToBottom:
		cursor = n - 1
	}
	return cursor
}
