package tui

import (
	tea "charm.land/bubbletea/v2"
)

// Update dispatches msg and then drains any listen the handler ended. The
// drain lives here rather than at each end site because several of those sites
// are value-receiver helpers that cannot return a tea.Cmd, and updateInner's
// many early returns make this the only point that observes all of them.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.updateInner(msg)
	// Every branch of updateInner returns this Model.
	updated := next.(Model)
	if m.sortPreferenceKey() != updated.sortPreferenceKey() {
		updated = updated.restoreSortPreference()
	}
	updated = updated.restoreNavigationParents(m)
	updated, flushCmd := updated.drainListenFlushes()
	updated = updated.publishControlState()
	if flushCmd == nil {
		return updated, cmd
	}
	return updated, tea.Batch(cmd, flushCmd)
}
