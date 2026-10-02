package tui

import (
	tea "charm.land/bubbletea/v2"
)

// handleTrackInfoKeyPress handles key events while the track info overlay is
// active. Returns (model, cmd, true) when the key was consumed.
func (m Model) handleTrackInfoKeyPress(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	if m.overlay.trackInfo == nil {
		return m, nil, false
	}
	switch msg.String() {
	case "ctrl+c":
		m.overlay.trackInfo = nil
		return m.markQuitting(), tea.Quit, true
	case "esc", "i":
		m.overlay.trackInfo = nil
		return m, nil, true
	case "y":
		m.statusBar.Notify("Path copied to clipboard")
		return m, tea.SetClipboard(m.overlay.trackInfo.Path), true
	default:
		return m, nil, true
	}
}
