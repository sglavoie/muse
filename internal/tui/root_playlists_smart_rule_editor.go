package tui

import (
	"errors"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/tui/smartrules"
)

type smartPlaylistSaveResultMsg struct {
	id         int64
	editing    bool
	playlists  []db.PlaylistRow
	writeErr   error
	refreshErr error
}

// handleSmartRuleEditorKeyPress routes keys while the editor is open.
func (m Model) handleSmartRuleEditorKeyPress(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	if m.overlay.smartRuleEditor == nil {
		return m, nil, false
	}
	if msg.String() == "ctrl+c" {
		return m.markQuitting(), tea.Quit, true
	}
	updated, act, cmd := m.overlay.smartRuleEditor.HandleKey(msg)
	m.overlay.smartRuleEditor = &updated
	switch act {
	case smartrules.ActionClose:
		m.overlay.smartRuleEditor = nil
	case smartrules.ActionCommit:
		if m.smartPlaylistWriter == nil {
			updated = updated.SaveFailed(errors.New("playlist storage unavailable"))
			m.overlay.smartRuleEditor = &updated
			return m, nil, true
		}
		submitted := updated.Submit()
		m.overlay.smartRuleEditor = &submitted
		return m, saveSmartPlaylistCmd(m.smartPlaylistWriter, updated), true
	}
	return m, cmd, true
}

func saveSmartPlaylistCmd(writer smartPlaylistWriter, editor smartrules.SmartRuleEditor) tea.Cmd {
	name, query := editor.Name(), editor.BuildQuery()
	result := smartPlaylistSaveResultMsg{id: editor.EditID, editing: editor.IsEdit}
	return func() tea.Msg {
		raw, err := db.MarshalSmartPlaylistQuery(query)
		if err != nil {
			result.writeErr = err
			return result
		}
		if result.editing {
			result.writeErr = writer.UpdateSmartPlaylistQuery(result.id, name, raw)
		} else {
			result.id, result.writeErr = writer.CreateSmartPlaylist(name, raw)
		}
		if result.writeErr == nil {
			result.playlists, result.refreshErr = writer.ReadPlaylists()
		}
		return result
	}
}

func (m Model) handleSmartPlaylistSaveResult(msg smartPlaylistSaveResultMsg) (Model, tea.Cmd) {
	if msg.writeErr != nil {
		if m.overlay.smartRuleEditor != nil {
			editor := m.overlay.smartRuleEditor.SaveFailed(msg.writeErr)
			m.overlay.smartRuleEditor = &editor
		}
		m.statusBar.NotifyError("Smart playlist save failed: " + msg.writeErr.Error())
		return m, nil
	}
	m.overlay.smartRuleEditor = nil
	if msg.refreshErr != nil {
		m.statusBar.NotifyError("Smart playlist saved, but list refresh failed: " + msg.refreshErr.Error())
		return m, nil
	}
	if msg.editing {
		return m.handleSmartPlaylistEditedMsg(smartPlaylistEditedMsg{playlists: msg.playlists})
	}
	return m.handleSmartPlaylistCreatedMsg(smartPlaylistCreatedMsg{id: msg.id, playlists: msg.playlists})
}
