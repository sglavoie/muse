package tui

import (
	"path/filepath"

	tea "charm.land/bubbletea/v2"
)

// commandPaletteActionMsg carries an action to replay through handleKeyAction
// after the palette has been dismissed.
type commandPaletteActionMsg struct {
	action action
}

// buildPaletteCommands returns the list of commands available in the palette.
func (m Model) buildPaletteCommands() []PaletteCommand {
	var cmds []PaletteCommand

	if m.configPath != "" {
		dir := filepath.Dir(m.configPath)
		cmds = append(cmds, PaletteCommand{
			Name: "Open config directory",
			Exec: func() tea.Cmd { return openDir(m.appCtx, dir) },
		})
	}

	if m.dataDir != "" {
		dir := m.dataDir
		cmds = append(cmds, PaletteCommand{
			Name: "Open database directory",
			Exec: func() tea.Cmd { return openDir(m.appCtx, dir) },
		})
	}

	cmds = append(cmds, PaletteCommand{
		Name: "Show debug panel",
		Exec: func() tea.Cmd {
			return func() tea.Msg { return commandPaletteActionMsg{action: actionDebug} }
		},
	})

	return cmds
}

// handleCommandPaletteKeyPress intercepts key presses when the command palette is active.
func (m Model) handleCommandPaletteKeyPress(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	if m.overlay.commandPalette == nil {
		return m, nil, false
	}
	if msg.String() == "ctrl+c" {
		return m, tea.Quit, true
	}
	updated, cmd := m.overlay.commandPalette.HandleKey(msg.String(), msg)
	m.overlay.commandPalette = &updated
	return m, cmd, true
}
