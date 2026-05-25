package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/db"
)

// handleSmartRuleEditorKeyPress routes key events to the smart rule editor
// overlay when it is active. Returns (model, cmd, true) when the key was consumed.
func (m Model) handleSmartRuleEditorKeyPress(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	if m.overlay.smartRuleEditor == nil {
		return m, nil, false
	}
	if msg.String() == "ctrl+c" {
		return m, tea.Quit, true
	}
	updated, act, cmd := m.overlay.smartRuleEditor.HandleKey(msg)
	m.overlay.smartRuleEditor = &updated
	switch act {
	case ruleEditorClose:
		m.overlay.smartRuleEditor = nil
		return m, cmd, true
	case ruleEditorCommit:
		name := updated.Name()
		q := updated.buildQuery()
		isEdit := updated.isEdit
		editID := updated.editID
		m.overlay.smartRuleEditor = nil
		sw := m.smartPlaylistWriter
		if sw != nil {
			return m, func() tea.Msg {
				queryJSON, err := db.MarshalSmartPlaylistQuery(q)
				if err != nil {
					return TransientNotifyMsg("Smart playlist error: " + err.Error())
				}
				if isEdit {
					if err := sw.UpdateSmartPlaylistQuery(editID, name, queryJSON); err != nil {
						return TransientNotifyMsg("Update failed: " + err.Error())
					}
					rows, err := sw.ReadPlaylists()
					if err != nil {
						return TransientNotifyMsg("Refresh failed: " + err.Error())
					}
					return smartPlaylistEditedMsg{playlists: rows}
				}
				id, err := sw.CreateSmartPlaylist(name, queryJSON)
				if err != nil {
					return TransientNotifyMsg("Create failed: " + err.Error())
				}
				rows, err := sw.ReadPlaylists()
				if err != nil {
					return TransientNotifyMsg("Refresh failed: " + err.Error())
				}
				return smartPlaylistCreatedMsg{id: id, playlists: rows}
			}, true
		}
	}
	return m, cmd, true
}
