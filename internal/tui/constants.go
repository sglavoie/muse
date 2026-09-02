package tui

import "time"

// undoWindowDuration is the time after a destructive queue action during which
// pressing 'u' will reverse the action. Only a single undo slot is maintained;
// a new destructive action within the window replaces the prior slot.
const undoWindowDuration = 10 * time.Second

// scanBatchSize is the maximum number of tracks collected per batch before
// returning a ScanBatchMsg to the update loop.
const scanBatchSize = 100

// minTerminalWidth and minTerminalHeight are the smallest terminal dimensions
// where Muse supports rendering the normal TUI. Below this floor the root view
// renders a compact resize prompt instead of treating normal layout overflow as
// a bug.
const (
	minTerminalWidth  = 60
	minTerminalHeight = 15
)

// playlistFallbackVisibleRows is the assumed number of visible rows when
// the terminal height is unknown (zero). Used by ensureCursorVisible to
// keep the cursor within the viewport before the first WindowSizeMsg.
const playlistFallbackVisibleRows = 50
