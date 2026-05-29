package tui

import tea "charm.land/bubbletea/v2"

type inputMsgForwarder func(Model, tea.Msg) (Model, tea.Cmd, bool)

var orderedInputForwarders = []inputMsgForwarder{
	Model.forwardHelpSearchMsg,
	Model.forwardSettingsMsg,
	Model.forwardMetadataEditorMsg,
	Model.forwardSmartRuleEditorMsg,
	Model.forwardCommandPaletteMsg,
	Model.forwardFilterMsg,
	Model.forwardPlaylistCreateMsg,
	Model.forwardPlaylistRenameMsg,
	Model.forwardQueueSaveMsg,
	Model.forwardTagCreateMsg,
	Model.forwardTagRenameMsg,
}

// forwardToActiveInput routes a generic tea.Msg to whichever text input is
// currently focused. This enables paste (and any other multi-step textinput
// commands) to complete — the bubbles v2 textinput uses a Paste command that
// produces an internal message which must reach textinput.Update again.
// Returns (model, cmd, true) if a focused input consumed the message.
func (m Model) forwardToActiveInput(msg tea.Msg) (Model, tea.Cmd, bool) {
	for _, forwarder := range orderedInputForwarders {
		if model, cmd, ok := forwarder(m, msg); ok {
			return model, cmd, true
		}
	}
	return m, nil, false
}

func (m Model) forwardHelpSearchMsg(msg tea.Msg) (Model, tea.Cmd, bool) {
	if m.helpVisible && m.helpSearchFilter.Active {
		var cmd tea.Cmd
		m.helpSearchFilter, cmd = m.helpSearchFilter.UpdateMsg(msg)
		m.refreshHelpViewport()
		return m, cmd, true
	}
	return m, nil, false
}

func (m Model) forwardSettingsMsg(msg tea.Msg) (Model, tea.Cmd, bool) {
	if m.overlay.settings != nil {
		updated, cmd, ok := m.overlay.settings.ForwardMsg(msg)
		if ok {
			m.overlay.settings = &updated
			return m, cmd, true
		}
	}
	return m, nil, false
}

func (m Model) forwardMetadataEditorMsg(msg tea.Msg) (Model, tea.Cmd, bool) {
	if m.overlay.metadataEditor != nil {
		updated, cmd := m.overlay.metadataEditor.ForwardMsg(msg)
		m.overlay.metadataEditor = &updated
		return m, cmd, true
	}
	return m, nil, false
}

func (m Model) forwardSmartRuleEditorMsg(msg tea.Msg) (Model, tea.Cmd, bool) {
	if m.overlay.smartRuleEditor != nil {
		updated, cmd, ok := m.overlay.smartRuleEditor.ForwardMsg(msg)
		if ok {
			m.overlay.smartRuleEditor = &updated
			return m, cmd, true
		}
	}
	return m, nil, false
}

func (m Model) forwardCommandPaletteMsg(msg tea.Msg) (Model, tea.Cmd, bool) {
	if m.overlay.commandPalette != nil {
		updated := *m.overlay.commandPalette
		var cmd tea.Cmd
		updated.filter, cmd = updated.filter.UpdateMsg(msg)
		updated.refilter()
		m.overlay.commandPalette = &updated
		return m, cmd, true
	}
	return m, nil, false
}

func (m Model) forwardFilterMsg(msg tea.Msg) (Model, tea.Cmd, bool) {
	if ops := m.activeFilterOps(); ops != nil {
		m, cmd := ops.updateMsg(msg)
		return m, cmd, true
	}
	return m, nil, false
}

func (m Model) forwardPlaylistCreateMsg(msg tea.Msg) (Model, tea.Cmd, bool) {
	if m.playlists.isCreating() && m.activeView == viewPlaylists {
		var cmd tea.Cmd
		m.playlists, cmd = m.playlists.updateCreateInputMsg(msg)
		return m, cmd, true
	}
	return m, nil, false
}

func (m Model) forwardPlaylistRenameMsg(msg tea.Msg) (Model, tea.Cmd, bool) {
	if m.playlists.isRenaming() && m.activeView == viewPlaylists {
		var cmd tea.Cmd
		m.playlists, cmd = m.playlists.updateRenameInputMsg(msg)
		return m, cmd, true
	}
	return m, nil, false
}

func (m Model) forwardQueueSaveMsg(msg tea.Msg) (Model, tea.Cmd, bool) {
	if m.queue.isSavingAsPlaylist() && m.activeView == viewQueue {
		var cmd tea.Cmd
		m.queue, cmd = m.queue.updateSaveInputMsg(msg)
		return m, cmd, true
	}
	return m, nil, false
}

func (m Model) forwardTagCreateMsg(msg tea.Msg) (Model, tea.Cmd, bool) {
	if m.tagsView.isCreating() && m.activeView == viewTags {
		var cmd tea.Cmd
		m.tagsView, cmd = m.tagsView.updateCreateInputMsg(msg)
		return m, cmd, true
	}
	return m, nil, false
}

func (m Model) forwardTagRenameMsg(msg tea.Msg) (Model, tea.Cmd, bool) {
	if m.tagsView.isRenaming() && m.activeView == viewTags {
		var cmd tea.Cmd
		m.tagsView, cmd = m.tagsView.updateRenameInputMsg(msg)
		return m, cmd, true
	}
	return m, nil, false
}
