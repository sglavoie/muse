package tui

import "time"

// undoWindowDuration is the time after a destructive queue action during which
// pressing 'u' will reverse the action. Only a single undo slot is maintained;
// a new destructive action within the window replaces the prior slot.
const undoWindowDuration = 10 * time.Second

// scanBatchSize is the maximum number of tracks collected per batch before
// returning a ScanBatchMsg to the update loop.
const scanBatchSize = 100

// MinTerminalWidth and MinTerminalHeight are the smallest terminal dimensions
// where Muse supports rendering the normal TUI. Below this floor the root view
// renders a compact resize prompt instead of treating normal layout overflow as
// a bug.
const (
	MinTerminalWidth  = 60
	MinTerminalHeight = 15
)
