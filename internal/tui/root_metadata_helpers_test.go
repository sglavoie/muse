package tui

import tea "charm.land/bubbletea/v2"

// pressMKey sends an uppercase 'M' key to the model, triggering actionEditMetadata.
func pressMKey(m Model) Model {
	next, _ := m.Update(tea.KeyPressMsg{Code: 'M', Text: "M"})
	return next.(Model)
}

// typeIntoEditor sends a string of characters to the editor in m.overlay.metadataEditor
// and returns the updated Model.
func typeIntoEditor(m Model, text string) Model {
	for _, ch := range text {
		ed, _, _ := m.overlay.metadataEditor.HandleKey(tea.KeyPressMsg{Code: ch, Text: string(ch)})
		m.overlay.metadataEditor = &ed
	}
	return m
}

// backspaceInEditor sends n backspace presses to the focused editor field.
func backspaceInEditor(m Model, n int) Model {
	for range n {
		ed, _, _ := m.overlay.metadataEditor.HandleKey(tea.KeyPressMsg{Code: tea.KeyBackspace})
		m.overlay.metadataEditor = &ed
	}
	return m
}
