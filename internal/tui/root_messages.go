package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/desktop"
	"github.com/sglavoie/muse/internal/tui/overlay"
	"github.com/sglavoie/muse/internal/tui/statusbar"
)

type showRecentMessagesMsg struct{}
type messagesCopiedMsg struct{ err error }

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
	case "y":
		if m.fixtureSafeCommands {
			m.statusBar.Notify("Clipboard copy blocked in fixture mode")
			return m, nil, true
		}
		if content := m.overlay.messages.CopyText(); content != "" {
			copyCmd := desktop.CopyToClipboard(m.appCtx, content)
			return m, func() tea.Msg {
				result := copyCmd().(desktop.ClipboardResultMsg)
				return messagesCopiedMsg{err: result.Err}
			}, true
		}
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
		label := ""
		if entry.Severity == statusbar.SeverityError {
			label = "[error] "
		}
		lines[i] = entry.At.Format("15:04:05") + "  " + label + entry.Text
	}
	m.overlay.messages = overlay.NewMessages(lines)
	return m
}

func (m Model) handleMessagesCopied(msg messagesCopiedMsg) Model {

	if msg.err != nil {
		m.statusBar.NotifyError("Copy failed: " + msg.err.Error())
	} else {
		m.statusBar.Notify("Copied recent messages")
	}

	return m
}
