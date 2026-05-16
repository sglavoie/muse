package tui

import tea "charm.land/bubbletea/v2"

// loadDismissedSet reads dismissed track hashes from the database and returns
// them as a set. Returns nil when reader is nil or on error.
func loadDismissedSet(reader dismissalReader) map[string]struct{} {
	if reader == nil {
		return nil
	}
	set, err := reader.ReadDismissedTrackHashes()
	if err != nil {
		return nil
	}
	return set
}

// mixDismissedMsg is sent when a track has been successfully dismissed.
type mixDismissedMsg struct{ hash string }

// mixDismissErrorMsg is sent when a dismiss DB write fails.
type mixDismissErrorMsg struct {
	hash string
	err  error
}

// mixUndismissedMsg is sent when a track has been successfully undismissed.
type mixUndismissedMsg struct{ hash string }

// mixUndismissErrorMsg is sent when an undismiss DB write fails.
type mixUndismissErrorMsg struct{ err error }

// handleMixDismiss dismisses the focused track at level 1. It writes to the
// database asynchronously and, on success, removes the track from the cached
// mix and the in-memory dismissedHashes set (bug hotspot #42: async validation).
func (m Model) handleMixDismiss() (Model, tea.Cmd) {
	if m.activeView != viewMixes || m.mixes.level != 1 {
		return m, nil
	}
	t := m.mixes.focusedTrack()
	if t == nil {
		return m, nil
	}
	dw := m.dismissalWriter
	if dw == nil {
		m.statusBar.notify("Dismissal not available")
		return m, nil
	}
	hash := t.Hash
	return m, func() tea.Msg {
		if err := dw.DismissTrack(hash); err != nil {
			return mixDismissErrorMsg{hash: hash, err: err}
		}
		return mixDismissedMsg{hash: hash}
	}
}

// handleMixDismissedMsg updates in-memory state after a successful dismiss:
// adds the hash to dismissedHashes and removes the track from the open mix.
func (m Model) handleMixDismissedMsg(msg mixDismissedMsg) Model {
	if m.dismissedHashes == nil {
		m.dismissedHashes = make(map[string]struct{})
	}
	m.dismissedHashes[msg.hash] = struct{}{}

	if tracks, ok := m.mixes.cachedMixes[m.mixes.activeMix]; ok {
		updated := removeTrackByHash(tracks, msg.hash)
		m.mixes.cachedMixes[m.mixes.activeMix] = updated
		// Refresh the track view if currently open on this mix, preserving
		// cursor position, sort state, filter text, and selection (bug: the
		// previous openMix call reset all of these).
		if m.activeView == viewMixes && m.mixes.level == 1 {
			m.mixes.trackView.sourceTracks = updated
			m.mixes.trackView = m.mixes.trackView.refreshView().clampCursor()
		}
	}
	m.statusBar.notify("Dismissed — won't appear in future mixes")
	return m
}

// handleMixUndismiss undismisses the focused track at level 1. The track
// reappears on the next mix load.
func (m Model) handleMixUndismiss() (Model, tea.Cmd) {
	if m.activeView != viewMixes || m.mixes.level != 1 {
		return m, nil
	}
	t := m.mixes.focusedTrack()
	if t == nil {
		return m, nil
	}
	dw := m.dismissalWriter
	if dw == nil {
		m.statusBar.notify("Dismissal not available")
		return m, nil
	}
	hash := t.Hash
	return m, func() tea.Msg {
		if err := dw.UndismissTrack(hash); err != nil {
			return mixUndismissErrorMsg{err: err}
		}
		return mixUndismissedMsg{hash: hash}
	}
}

// handleMixUndismissedMsg removes the hash from in-memory dismissedHashes.
func (m Model) handleMixUndismissedMsg(msg mixUndismissedMsg) Model {
	delete(m.dismissedHashes, msg.hash)
	m.statusBar.notify("Undismissed — will reappear on next mix load")
	return m
}
