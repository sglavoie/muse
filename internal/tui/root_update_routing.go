package tui

import tea "charm.land/bubbletea/v2"

func (m Model) handleWindowSizeMsg(msg tea.WindowSizeMsg) Model {
	m.termWidth = msg.Width
	m.termHeight = msg.Height
	m.setViewDimensions(msg.Width, msg.Height)
	m.clampAllScrollOffsets()
	if m.helpVisible {
		m.helpViewport.SetWidth(msg.Width)
		extraLines := 0
		if m.helpSearchFilter.Active || m.helpSearchFilter.Text != "" {
			extraLines = 1
		}
		m.helpViewport.SetHeight(max(msg.Height-2-extraLines, 0))
	}
	if m.debugVisible {
		m.debugViewport.SetWidth(msg.Width)
		m.debugViewport.SetHeight(max(msg.Height-2, 0))
	}
	if m.statsVisible {
		m.statsViewport.SetWidth(msg.Width)
		m.statsViewport.SetHeight(max(msg.Height-3, 0))
	}
	return m
}

func (m Model) handleKeyPressMsg(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// Dismiss the first-run config notice on any keypress.
	m.configCreated = false

	if model, cmd, ok := m.routeKeyPress(msg); ok {
		return model, cmd
	}
	return m.handleKeyAction(m.registry.resolve(m.activeViewContext(), msg.String()))
}

func (m Model) handleUnhandledMsg(msg tea.Msg) (tea.Model, tea.Cmd) {
	if model, cmd, ok := m.forwardToActiveInput(msg); ok {
		return model, cmd
	}
	return m, nil
}