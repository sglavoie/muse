package tui

import tea "charm.land/bubbletea/v2"

// handlePlaylistCreateInput intercepts keypresses while the inline playlist
// create input is active. Returns true in the third value when the key was
// consumed, preventing further dispatch.
func (m Model) handlePlaylistCreateInput(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	if !m.playlists.isCreating() || m.activeView != viewPlaylists {
		return m, nil, false
	}
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit, true
	case "esc":
		m.playlists = m.playlists.cancelCreate()
		return m, nil, true
	case "enter":
		if !m.playlists.createInputIsEmpty() {
			name := m.playlists.createInputValue()
			if playlistNameExists(m.playlists.playlists, name, 0) {
				m.statusBar.notification = newNotification("A playlist with that name already exists")
				return m, nil, true
			}
			if m.playlistWriter != nil {
				pw := m.playlistWriter
				return m, func() tea.Msg {
					id, err := pw.CreatePlaylist(name)
					if err != nil {
						return playlistCreateErrorMsg{err: err}
					}
					rows, err := pw.ReadPlaylists()
					if err != nil {
						return playlistCreateErrorMsg{err: err}
					}
					return playlistCreatedMsg{id: id, playlists: rows}
				}, true
			}
			// No writer: dismiss the input without persisting.
			m.playlists = m.playlists.cancelCreate()
		}
		// Empty/whitespace input: keep the field active (no-op).
		return m, nil, true
	default:
		var cmd tea.Cmd
		m.playlists, cmd = m.playlists.updateCreateInput(msg)
		return m, cmd, true
	}
}

// handlePlaylistRenameInput intercepts keypresses while the inline playlist
// rename input is active. Returns true in the third value when the key was
// consumed, preventing further dispatch.
func (m Model) handlePlaylistRenameInput(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	if !m.playlists.isRenaming() || m.activeView != viewPlaylists {
		return m, nil, false
	}
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit, true
	case "esc":
		m.playlists = m.playlists.cancelRename()
		return m, nil, true
	case "enter":
		if !m.playlists.renameInputIsEmpty() {
			name := m.playlists.renameInputValue()
			pl, ok := m.playlists.focusedPlaylist()
			if !ok {
				m.playlists = m.playlists.cancelRename()
				return m, nil, true
			}
			if playlistNameExists(m.playlists.playlists, name, pl.ID) {
				m.statusBar.notification = newNotification("A playlist with that name already exists")
				return m, nil, true
			}
			if m.playlistRenamer != nil {
				pr := m.playlistRenamer
				id := pl.ID
				return m, func() tea.Msg {
					if err := pr.RenamePlaylist(id, name); err != nil {
						return playlistRenameErrorMsg{err: err}
					}
					rows, err := pr.ReadPlaylists()
					if err != nil {
						return playlistRenameErrorMsg{err: err}
					}
					return playlistRenamedMsg{id: id, playlists: rows}
				}, true
			}
			// No renamer: dismiss the input without persisting.
			m.playlists = m.playlists.cancelRename()
		}
		// Empty/whitespace input: keep the field active (no-op).
		return m, nil, true
	default:
		var cmd tea.Cmd
		m.playlists, cmd = m.playlists.updateRenameInput(msg)
		return m, cmd, true
	}
}

// handleQueueSaveInput intercepts keypresses while the inline save-as-playlist
// input is active. Returns true in the third value when the key was consumed,
// preventing further dispatch.
func (m Model) handleQueueSaveInput(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	if !m.queue.isSavingAsPlaylist() || m.activeView != viewQueue {
		return m, nil, false
	}
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit, true
	case "esc":
		m.queue = m.queue.cancelSaveAsPlaylist()
		return m, nil, true
	case "enter":
		if !m.queue.saveInputIsEmpty() {
			name := m.queue.saveInputValue()
			if playlistNameExists(m.playlists.playlists, name, 0) {
				m.statusBar.notification = newNotification("A playlist with that name already exists")
				return m, nil, true
			}
			if m.playlistWriter != nil {
				pw := m.playlistWriter
				pa := m.playlistAdder
				var hashes []string
				if m.queue.savingFromSelection {
					hashes = m.queue.selectedHashes()
				} else {
					hashes = make([]string, len(m.queue.entries))
					for i, e := range m.queue.entries {
						hashes[i] = e.Hash
					}
				}
				count := len(hashes)
				fromSelection := m.queue.savingFromSelection
				return m, func() tea.Msg {
					id, err := pw.CreatePlaylist(name)
					if err != nil {
						return queueSaveAsPlaylistErrorMsg{err: err}
					}
					if pa != nil && len(hashes) > 0 {
						if err := pa.AddTracksToPlaylist(id, hashes); err != nil {
							return queueSaveAsPlaylistErrorMsg{err: err}
						}
					}
					rows, err := pw.ReadPlaylists()
					if err != nil {
						return queueSaveAsPlaylistErrorMsg{err: err}
					}
					return queueSavedAsPlaylistMsg{playlists: rows, playlistName: name, count: count, fromSelection: fromSelection}
				}, true
			}
			// No writer: dismiss the input without persisting.
			m.queue = m.queue.cancelSaveAsPlaylist()
		}
		// Empty/whitespace input: keep the field active (no-op).
		return m, nil, true
	default:
		var cmd tea.Cmd
		m.queue, cmd = m.queue.updateSaveInput(msg)
		return m, cmd, true
	}
}
