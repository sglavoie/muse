package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/tui/overlay"
)

type showRecentMessagesMsg struct{}

func (m Model) handleMessagesKeyPress(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	if m.overlay.messages == nil {
		return m, nil, false
	}
	switch msg.String() {
	case "ctrl+c":
		m.overlay.messages = nil
		return m.markQuitting(), tea.Quit, true
	case "q", "esc":
		m.overlay.messages = nil
	default:
		next := *m.overlay.messages
		next.Scroll(msg.String(), m.termWidth, max(m.termHeight-3, 1))
		m.overlay.messages = &next
	}
	return m, nil, true
}

func (m Model) showRecentMessages() Model {
	history := m.statusBar.RecentMessages()
	lines := make([]string, len(history))
	for i, entry := range history {
		lines[i] = entry.At.Format("15:04:05") + "  " + entry.Text
	}
	m.overlay.messages = overlay.NewMessages(lines)
	return m
}
