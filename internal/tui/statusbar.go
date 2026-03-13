package tui

import (
	"fmt"

	"github.com/sglavoie/muse/internal/library"
)

// StatusBar manages the persistent status bar at the bottom of the screen.
// transientRight holds a temporary message (e.g. scan progress) that is
// cleared when the scan completes.
type StatusBar struct {
	logPath        string
	configCreated  bool
	configPath     string
	transientRight string
}

// handleScanProgress sets the transient right zone to a formatted progress string.
// When msg.Total is known (> 0) the format is "Scanning… X / Y tracks";
// before the count arrives it falls back to "Scanning… X tracks".
func (s *StatusBar) handleScanProgress(msg library.ScanProgressMsg) {
	if msg.Total > 0 {
		s.transientRight = fmt.Sprintf("Scanning… %s / %s tracks", formatCount(msg.Scanned), formatCount(msg.Total))
	} else {
		s.transientRight = fmt.Sprintf("Scanning… %s tracks", formatCount(msg.Scanned))
	}
}

// handleScanDone clears the transient right zone.
func (s *StatusBar) handleScanDone() {
	s.transientRight = ""
}

// render builds the full status bar string.
func (s StatusBar) render() string {
	bar := fmt.Sprintf("log: %s", s.logPath)
	if s.configCreated {
		bar += fmt.Sprintf("  |  config created: %s", s.configPath)
	}
	if s.transientRight != "" {
		bar += "  |  " + s.transientRight
	}
	return bar
}
