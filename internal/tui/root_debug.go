package tui

import (
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
)

// handleDebugKeyPress handles key events while the debug panel is active.
// Returns (model, cmd, true) when the key was consumed, (model, nil, false) to
// fall through to global key handling (ctrl+c only).
func (m Model) handleDebugKeyPress(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	if !m.debugVisible {
		return m, nil, false
	}
	switch msg.String() {
	case "ctrl+c":
		// Fall through to global key handling for quit.
		return m, nil, false
	case "esc":
		m.debugVisible = false
		m.debugViewport = viewport.Model{}
		m.ActiveView = m.debugPrevView
		return m, nil, true
	default:
		var cmd tea.Cmd
		m.debugViewport, cmd = m.debugViewport.Update(msg)
		return m, cmd, true
	}
}
