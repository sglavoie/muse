package insights

import (
	"fmt"

	"github.com/sglavoie/muse/internal/tui/filter"
)

// NavigationKeys identifies briefing rows and sessions independently of order.
// Track lists use the shared library view's track identities instead.
func (v View) NavigationKeys() []string {
	var keys []string
	if v.Level == LevelSessions {
		for _, row := range v.SessionList {
			keys = append(keys, fmt.Sprintf("session:%s", row.ID))
		}
		return keys
	}
	for _, row := range v.Rows() {
		keys = append(keys, fmt.Sprintf("row:%d:%s:%s", row.Drill, row.Key, row.Label))
	}
	return keys
}

func (v View) NavigationPosition() filter.Position {
	if v.Level == LevelSessions {
		return filter.CapturePosition(v.NavigationKeys(), v.sessionCursor, v.sessionScroll)
	}
	return filter.CapturePosition(v.NavigationKeys(), v.Cursor, v.ScrollOffset)
}

func (v *View) RestoreNavigationPosition(p filter.Position) {
	cursor, scroll := p.Restore(v.NavigationKeys(), p.Key)
	if v.Level == LevelSessions {
		v.sessionCursor, v.sessionScroll = cursor, scroll
	} else {
		v.Cursor, v.ScrollOffset = cursor, scroll
	}
	if v.Height > 0 {
		v.ClampCursor()
	}
}

func (v View) ResolutionIndex() int { return v.resolutionIdx }

func (v *View) RestorePanel(panel Panel, window, resolution int) {
	v.Panel = Panel(max(0, min(int(panel), len(insightsPanelSpecs)-1)))
	v.WindowIdx = max(0, min(window, len(Windows)-1))
	v.resolutionIdx = max(0, min(resolution, len(insightsResolutions)-1))
}
