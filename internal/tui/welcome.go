package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/tui/text"
)

const welcomeKeyColWidth = 20

func welcomeRow(key, action string) OverlayRow {
	return OverlayRow{Text: text.Pad(key, welcomeKeyColWidth) + action}
}

// handleWelcomeKeyPress consumes any key press while the welcome overlay is
// shown, dismissing it so the user proceeds to the library view.
func (m Model) handleWelcomeKeyPress(_ tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	if !m.configCreated || m.welcomeDismissed {
		return m, nil, false
	}
	m.welcomeDismissed = true
	return m, nil, true
}

func (m Model) renderWelcomeOverlay() string {
	o := OverlayBox{
		Title: " Welcome to Muse ",
		Rows: []OverlayRow{
			{Blank: true},
			welcomeRow("j / k  or  ↑ / ↓", "Navigate"),
			welcomeRow("Enter", "Add to queue"),
			welcomeRow("Space", "Play / Pause"),
			welcomeRow("/", "Filter tracks"),
			welcomeRow("Tab", "Switch views"),
			welcomeRow("?", "Show all keybindings"),
			{Blank: true},
		},
		Hint: "  Press any key to start  ",
	}
	return o.Render(m.termWidth, m.termHeight, *m.styles)
}
