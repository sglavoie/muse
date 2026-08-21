package tui

import tea "charm.land/bubbletea/v2"

type keyPressHandler func(Model, tea.KeyPressMsg) (Model, tea.Cmd, bool)

var orderedKeyPressHandlers = []keyPressHandler{
	Model.handleWelcomeKeyPress,
	Model.handleHelpKeyPress,
	Model.handleDebugKeyPress,
	Model.handleSettingsKeyPress,
	Model.handleNowPlayingKeyPress,
	Model.handleQueueSummaryKeyPress,
	Model.handleTrackInfoKeyPress,
	Model.handlePreviewKeyPress,
	Model.handleMetadataEditorKeyPress,
	Model.handleMetadataImportKeyPress,
	Model.handleSmartRuleEditorKeyPress,
	Model.handleFilterKeyPress,
	Model.handlePlaylistCreateInput,
	Model.handlePlaylistRenameInput,
	Model.handleQueueSaveInput,
	Model.handleGoToKeyPress,
	Model.handleCommandPaletteKeyPress,
	Model.handleDestPickerKeyPress,
	Model.handleTagPickerKeyPress,
	Model.handleTagFilterPickerKeyPress,
	Model.handleTagMergeKeyPress,
	Model.handleFavoriteTargetPickerKeyPress,
	Model.handleTagCreateInputKeyPress,
	Model.handleTagRenameInputKeyPress,
	Model.handleScanCancelKeyPress,
	Model.handleInsightsMergeKeyPress,
}

// routeKeyPress applies root key interception in a single explicit order.
// Keeping the precedence centralized makes modal behavior easier to reason
// about and safer to extend without accidentally changing which overlay wins.
func (m Model) routeKeyPress(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	for _, handler := range orderedKeyPressHandlers {
		if model, cmd, ok := handler(m, msg); ok {
			return model, cmd, true
		}
	}
	return m, nil, false
}
