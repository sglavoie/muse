package tui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/desktop"
)

// handleTrackInfoKeyPress handles key events while the track info overlay is
// active. Returns (model, cmd, true) when the key was consumed.
func (m Model) handleTrackInfoKeyPress(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	if m.overlay.trackInfo == nil {
		return m, nil, false
	}
	switch msg.String() {
	case "ctrl+c":
		m.overlay.trackInfo = nil
		return m.markQuitting(), tea.Quit, true
	case "esc", "i":
		m.overlay.trackInfo = nil
		return m, nil, true
	case "y":
		if m.fixtureSafeCommands {
			m.statusBar.Notify("Clipboard copy blocked in fixture mode")
			return m, nil, true
		}
		return m, desktop.CopyToClipboard(m.appCtx, m.overlay.trackInfo.Path), true
	default:
		next := *m.overlay.trackInfo
		next.Scroll(msg.String(), m.termWidth, max(m.termHeight-3, 1))
		m.overlay.trackInfo = &next
		return m, nil, true
	}
}
