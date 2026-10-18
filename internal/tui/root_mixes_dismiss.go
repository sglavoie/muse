package tui

import (
	tea "charm.land/bubbletea/v2"
	"slices"
)

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
	if m.ActiveView != viewMixes || m.mixes.Level != 1 {
		return m, nil
	}
	t := m.mixes.FocusedTrack()
	if t == nil {
		return m, nil
	}
	dw := m.dismissalWriter
	if dw == nil {
		m.statusBar.Notify("Dismissal not available")
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
// adds the hash to dismissedHashes and removes the track from every cached
// mix. Without sweeping all cached mixes the dismissed track would still
// surface when the user navigates to another previously-loaded mix that
// shared the same hash, until the cache is rebuilt.
func (m Model) handleMixDismissedMsg(msg mixDismissedMsg) Model {
	if m.dismissedHashes == nil {
		m.dismissedHashes = make(map[string]struct{})
	}
	m.dismissedHashes[msg.hash] = struct{}{}
	m.lastMixDismissed = msg.hash
	// A dismissal is a heavy scoring penalty in the planner, so the set is an
	// Auto-DJ input as much as a mix filter.
	m.djInvalidate()
	delete(m.mixes.TrackView.SelectedHashes, msg.hash)
	if m.visual.view == viewMixes {
		m = exitVisualMode(m)
	}

	for id, tracks := range m.mixes.CachedMixes {
		m.mixes.CachedMixes[id] = removeTrackByHash(slices.Clone(tracks), msg.hash)
	}
	if tracks, ok := m.mixes.CachedMixes[m.mixes.ActiveMix]; ok {
		// Refresh the track view if currently open on this mix, preserving
		// cursor position, sort state, filter text, and selection (bug: the
		// previous openMix call reset all of these).
		if m.ActiveView == viewMixes && m.mixes.Level == 1 {
			m.mixes.TrackView.SourceTracks = tracks
			m.mixes.TrackView = m.mixes.TrackView.RefreshView().ClampCursor()
		}
	}
	m.statusBar.Notify("Dismissed — D restores this recommendation")
	return m
}

// handleMixUndismiss restores the last successful dismissal, even after focus
// moved or the mix became empty. Failed writes keep the target available to retry.
func (m Model) handleMixUndismiss() (Model, tea.Cmd) {
	if m.ActiveView != viewMixes {
		return m, nil
	}
	hash := m.lastMixDismissed
	if hash == "" {
		m.statusBar.Notify("No recommendation to restore")
		return m, nil
	}
	dw := m.dismissalWriter
	if dw == nil {
		m.statusBar.Notify("Dismissal not available")
		return m, nil
	}
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
	if m.lastMixDismissed == msg.hash {
		m.lastMixDismissed = ""
	}
	m.djInvalidate()
	m.statusBar.Notify("Recommendation restored")
	return m
}

func (m Model) reloadOpenMixCmd() tea.Cmd {
	if m.ActiveView == viewMixes && m.mixes.Level == 1 {
		return m.loadMixCmd(m.mixes.ActiveMix)
	}
	return nil
}
