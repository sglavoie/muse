package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/tui/keys"
)

// handleLibraryKeyAction handles library, artist, and album navigation key
// actions. It returns true in the third value when the action was handled.
func (m Model) handleLibraryKeyAction(action keys.Action) (tea.Model, tea.Cmd, bool) {
	if model, cmd, ok := m.handleLibraryStateAction(action); ok {
		return model, cmd, true
	}
	if model, cmd, ok := m.handleLibraryQueueAction(action); ok {
		return model, cmd, true
	}
	if model, cmd, ok := m.handleLibraryBulkAction(action); ok {
		return model, cmd, true
	}
	if model, cmd, ok := m.handleLibraryNavigationAction(action); ok {
		return model, cmd, true
	}
	return m, nil, false
}
