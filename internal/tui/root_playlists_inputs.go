package tui

import tea "charm.land/bubbletea/v2"

// handlePlaylistCreateInput intercepts keypresses while the inline playlist
// create input is active. Returns true in the third value when the key was
// consumed, preventing further dispatch.
func (m Model) handlePlaylistCreateInput(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	if !m.playlists.IsCreating() || m.ActiveView != viewPlaylists {
		return m, nil, false
	}
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit, true
	case "esc":
		m.playlists = m.playlists.CancelCreate()
		return m, nil, true
	case "enter":
		if !m.playlists.CreateInputIsEmpty() {
			name := m.playlists.CreateInputValue()
			if playlistNameExists(m.playlists.Playlists, name, 0) {
				m.statusBar.Notify("A playlist with that name already exists")
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
			m.playlists = m.playlists.CancelCreate()
		}
		// Empty/whitespace input: keep the field active (no-op).
		return m, nil, true
	default:
		var cmd tea.Cmd
		m.playlists, cmd = m.playlists.UpdateCreateInput(msg)
		return m, cmd, true
	}
}

// handlePlaylistRenameInput intercepts keypresses while the inline playlist
// rename input is active. Returns true in the third value when the key was
// consumed, preventing further dispatch.
func (m Model) handlePlaylistRenameInput(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	if !m.playlists.IsRenaming() || m.ActiveView != viewPlaylists {
		return m, nil, false
	}
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit, true
	case "esc":
		m.playlists = m.playlists.CancelRename()
		return m, nil, true
	case "enter":
		if !m.playlists.RenameInputIsEmpty() {
			name := m.playlists.RenameInputValue()
			pl, ok := m.playlists.FocusedPlaylist()
			if !ok {
				m.playlists = m.playlists.CancelRename()
				return m, nil, true
			}
			if playlistNameExists(m.playlists.Playlists, name, pl.ID) {
				m.statusBar.Notify("A playlist with that name already exists")
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
			m.playlists = m.playlists.CancelRename()
		}
		// Empty/whitespace input: keep the field active (no-op).
		return m, nil, true
	default:
		var cmd tea.Cmd
		m.playlists, cmd = m.playlists.UpdateRenameInput(msg)
		return m, cmd, true
	}
}

// handleQueueSaveInput intercepts keypresses while the inline save-as-playlist
// input is active. Returns true in the third value when the key was consumed,
// preventing further dispatch.
func (m Model) handleQueueSaveInput(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	if !m.queue.IsSavingAsPlaylist() || m.ActiveView != viewQueue {
		return m, nil, false
	}
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit, true
	case "esc":
		m.queue = m.queue.CancelSaveAsPlaylist()
		return m, nil, true
	case "enter":
		if !m.queue.SaveInputIsEmpty() {
			name := m.queue.SaveInputValue()
			if playlistNameExists(m.playlists.Playlists, name, 0) {
				m.statusBar.Notify("A playlist with that name already exists")
				return m, nil, true
			}
			if m.playlistWriter != nil {
				pw := m.playlistWriter
				pa := m.playlistAdder
				var hashes []string
				if m.queue.SavingFromSelection {
					hashes = m.queue.SelectedHashes()
				} else {
					hashes = make([]string, len(m.queue.Entries))
					for i, e := range m.queue.Entries {
						hashes[i] = e.Hash
					}
				}
				count := len(hashes)
				fromSelection := m.queue.SavingFromSelection
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
			m.queue = m.queue.CancelSaveAsPlaylist()
		}
		// Empty/whitespace input: keep the field active (no-op).
		return m, nil, true
	default:
		var cmd tea.Cmd
		m.queue, cmd = m.queue.UpdateSaveInput(msg)
		return m, cmd, true
	}
}
