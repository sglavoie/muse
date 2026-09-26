package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/tui/keys"
)

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
	return m.handleKeyAction(m.resolveKey(msg.String()))
}

// resolveKey maps key to its action in the active view. Mixes shadows the
// global stop key with sort-cycle, but only an open mix has tracks to sort, so
// on the mix list that key keeps its global meaning instead of doing nothing.
func (m Model) resolveKey(key string) keys.Action {
	a := m.registry.Resolve(m.activeViewContext(), key)
	if a == keys.ActionSortCycle && m.ActiveView == viewMixes && m.mixes.Level == 0 {
		return m.registry.Resolve(keys.ContextGlobal, key)
	}
	return a
}

func (m Model) handleUnhandledMsg(msg tea.Msg) (tea.Model, tea.Cmd) {
	if model, cmd, ok := m.forwardToActiveInput(msg); ok {
		return model, cmd
	}
	return m, nil
}
