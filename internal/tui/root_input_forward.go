package tui

import tea "charm.land/bubbletea/v2"

// forwardToActiveInput routes a generic tea.Msg to whichever text input is
// currently focused. This enables paste (and any other multi-step textinput
// commands) to complete — the bubbles v2 textinput uses a Paste command that
// produces an internal message which must reach textinput.Update again.
// Returns (model, cmd, true) if a focused input consumed the message.
func (m Model) forwardToActiveInput(msg tea.Msg) (Model, tea.Cmd, bool) {
	// Help search filter.
	if m.helpVisible && m.helpSearchFilter.Active {
		var cmd tea.Cmd
		m.helpSearchFilter, cmd = m.helpSearchFilter.UpdateMsg(msg)
		m.refreshHelpViewport()
		return m, cmd, true
	}

	// Settings overlay text inputs.
	if m.overlay.settings != nil {
		updated, cmd, ok := m.overlay.settings.ForwardMsg(msg)
		if ok {
			m.overlay.settings = &updated
			return m, cmd, true
		}
	}

	// Metadata editor.
	if m.overlay.metadataEditor != nil {
		updated, cmd := m.overlay.metadataEditor.ForwardMsg(msg)
		m.overlay.metadataEditor = &updated
		return m, cmd, true
	}

	// Smart rule editor.
	if m.overlay.smartRuleEditor != nil {
		updated, cmd, ok := m.overlay.smartRuleEditor.ForwardMsg(msg)
		if ok {
			m.overlay.smartRuleEditor = &updated
			return m, cmd, true
		}
	}

	// Filter bar (library, artists, albums, queue, favorites, genres, recently added).
	if ops := m.activeFilterOps(); ops != nil {
		m, cmd := ops.updateMsg(msg)
		return m, cmd, true
	}

	// Playlist create input.
	if m.playlists.isCreating() && m.activeView == viewPlaylists {
		var cmd tea.Cmd
		m.playlists, cmd = m.playlists.updateCreateInputMsg(msg)
		return m, cmd, true
	}

	// Playlist rename input.
	if m.playlists.isRenaming() && m.activeView == viewPlaylists {
		var cmd tea.Cmd
		m.playlists, cmd = m.playlists.updateRenameInputMsg(msg)
		return m, cmd, true
	}

	// Queue save-as-playlist input.
	if m.queue.isSavingAsPlaylist() && m.activeView == viewQueue {
		var cmd tea.Cmd
		m.queue, cmd = m.queue.updateSaveInputMsg(msg)
		return m, cmd, true
	}

	return m, nil, false
}
