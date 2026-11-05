package tui

import (
	tea "charm.land/bubbletea/v2"
)

// Update dispatches msg and then drains any listen the handler ended. The
// drain lives here rather than at each end site because several of those sites
// are value-receiver helpers that cannot return a tea.Cmd, and updateInner's
// many early returns make this the only point that observes all of them.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if resume, ok := msg.(browsingResumeMsg); ok {
		return m.resumeBrowsing(resume)
	}
	if _, key := msg.(tea.KeyPressMsg); key {
		m.cancelBrowsingRestore()
		m = m.dismissTrackCard()
	}
	next, cmd := m.updateInner(msg)
	// Every branch of updateInner returns this Model.
	updated := next.(Model)
	if m.sortPreferenceKey() != updated.sortPreferenceKey() {
		updated = updated.restoreSortPreference()
	}
	updated = updated.restoreNavigationParents(m)
	updated, flushCmd := updated.drainListenFlushes()
	updated = updated.publishControlState()
	updated, polishCmd := updated.syncPolish(m)
	return updated, batchNonNil(cmd, flushCmd, polishCmd)
}

// syncPolish keeps cover art and animations in step with what an update
// changed: lookups for newly visible covers, Kitty uploads for the next
// frame, the cover accent, and the animation frame loop.
func (m Model) syncPolish(prev Model) (Model, tea.Cmd) {
	m = m.noteMotionChanges(prev)
	if m.playback.currentHash != prev.playback.currentHash || m.playback.state != prev.playback.state {
		m = m.refreshArtworkAccent()
	}
	m, artCmd := m.syncArtwork()
	placeCmd := m.placeArtwork()
	m, frameCmd := m.scheduleMotion()
	return m, batchNonNil(artCmd, placeCmd, frameCmd)
}

// batchNonNil batches cmds, returning nil when every one is nil so callers
// that compare the result against nil keep working.
func batchNonNil(cmds ...tea.Cmd) tea.Cmd {
	var live []tea.Cmd
	for _, c := range cmds {
		if c != nil {
			live = append(live, c)
		}
	}
	switch len(live) {
	case 0:
		return nil
	case 1:
		return live[0]
	default:
		return tea.Batch(live...)
	}
}
