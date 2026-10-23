package tui

import (
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/tui/filter"
)

// handleHelpKeyPress handles key events while the help overlay is active.
// Returns (model, cmd, true) when the key was consumed, (model, nil, false) to
// fall through to global key handling (ctrl+c only).
func (m Model) handleHelpKeyPress(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	if !m.helpVisible {
		return m, nil, false
	}
	if m.helpSearchFilter.Active {
		// Filter input mode: capture typing.
		switch msg.String() {
		case "ctrl+c":
			// Fall through to global key handling for quit.
			return m, nil, false
		case "esc":
			m.helpSearchFilter = m.helpSearchFilter.Deactivate()
			m.refreshHelpViewport()
			return m, nil, true
		case "enter":
			m.helpSearchFilter = m.helpSearchFilter.Commit()
			m.refreshHelpViewport()
			return m, nil, true
		default:
			var cmd tea.Cmd
			m.helpSearchFilter, cmd = m.helpSearchFilter.Update(msg)
			m.refreshHelpViewport()
			return m, cmd, true
		}
	}
	// Normal help overlay mode.
	if next, cmd, ok := m.handleInfoTransport(msg); ok {
		return next, cmd, true
	}
	switch msg.String() {
	case "ctrl+c":
		// Fall through to global key handling for quit.
		return m, nil, false
	case "/":
		var cmd tea.Cmd
		m.helpSearchFilter, cmd = m.helpSearchFilter.Activate()
		m.refreshHelpViewport()
		return m, cmd, true
	case "esc":
		if m.helpSearchFilter.Text != "" {
			m.helpSearchFilter = filter.State{}
			m.refreshHelpViewport()
			return m, nil, true
		}
		m.helpVisible = false
		m.helpViewport = viewport.Model{}
		m.helpFiltered = false
		m.helpSearchFilter = filter.State{}
		m.ActiveView = m.prevActiveView
		return m, nil, true
	case "?":
		m.helpFiltered = !m.helpFiltered
		m.refreshHelpViewport()
		return m, nil, true
	case "g", "home":
		// The viewport's default keymap has no top/bottom jump; match the
		// g / G every list view uses.
		m.helpViewport.GotoTop()
		return m, nil, true
	case "G", "end":
		m.helpViewport.GotoBottom()
		return m, nil, true
	default:
		var cmd tea.Cmd
		m.helpViewport, cmd = m.helpViewport.Update(msg)
		return m, cmd, true
	}
}

// renderHelpFooter stays outside the scrollable content so recovery keys remain visible.
func (m Model) renderHelpFooter() string {
	hints := "j/k scroll · / search · ? all/view · esc close"
	if m.helpSearchFilter.Active {
		hints = "enter apply · esc clear"
	} else if m.helpSearchFilter.Text != "" {
		hints = "/ search · ? all/context · esc clear"
	}
	position := fmt.Sprintf("%d%%", int(m.helpViewport.ScrollPercent()*100))
	room := max(0, m.termWidth-len(position)-3)
	return m.styles.Dim.Render(ansi.Truncate(hints, room, "…") + " · " + position)
}
