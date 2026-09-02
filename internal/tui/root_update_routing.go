package tui

import tea "charm.land/bubbletea/v2"

func (m Model) handleWindowSizeMsg(msg tea.WindowSizeMsg) Model {
	m.termWidth = msg.Width
	m.termHeight = msg.Height
	m.setViewDimensions(msg.Width, msg.Height)
	m.clampAllScrollOffsets()
	if m.helpVisible {
		m.helpViewport.SetWidth(msg.Width)
		m.refreshHelpViewport()
	}
	if m.debugVisible {
		m.debugViewport.SetWidth(msg.Width)
		m.debugViewport.SetHeight(max(msg.Height-2, 0))
	}
	return m
}

func (m Model) handleKeyPressMsg(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// Dismiss the first-run config notice on any keypress.
	m.configCreated = false

	if model, cmd, ok := m.routeKeyPress(msg); ok {
		return model, cmd
	}
	return m.handleKeyAction(m.registry.Resolve(m.activeViewContext(), msg.String()))
}

func (m Model) handleUnhandledMsg(msg tea.Msg) (tea.Model, tea.Cmd) {
	if model, cmd, ok := m.forwardToActiveInput(msg); ok {
		return model, cmd
	}
	return m, nil
}
