package tui

import tea "charm.land/bubbletea/v2"

// handleCommandPaletteKeyPress intercepts key presses when the command palette is active.
func (m Model) handleCommandPaletteKeyPress(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	if m.overlay.commandPalette == nil {
		return m, nil, false
	}
	if msg.String() == "ctrl+c" {
		return m, tea.Quit, true
	}
	updated, cmd := m.overlay.commandPalette.HandleKey(msg.String(), msg)
	m.overlay.commandPalette = &updated
	return m, cmd, true
}
