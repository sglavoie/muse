package playlists

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

// SetHeight supplies the current terminal height for page navigation.
func (m *DestinationPickerModal) SetHeight(height int) { m.height = height }

func (m DestinationPickerModal) visibleRows() int {
	if m.height <= 0 {
		return len(m.destinations())
	}
	// Borders, title, separator, footer, and a spare line for the final newline.
	overhead := 6
	if m.search.Active || m.search.Text != "" {
		overhead++
	}
	return max(1, m.height-overhead)
}

func (m DestinationPickerModal) matchingPlaylists() []int {
	query := strings.ToLower(m.search.Text)
	var matches []int
	for i, p := range m.playlists {
		if strings.Contains(strings.ToLower(p.Name), query) {
			matches = append(matches, i)
		}
	}
	return matches
}

// HandleKeyPress routes text to the search input before navigation shortcuts.
func (m DestinationPickerModal) HandleKeyPress(msg tea.KeyPressMsg) (DestinationPickerModal, tea.Cmd) {
	if !m.search.Active {
		return m.HandleKey(msg.String())
	}
	switch msg.String() {
	case "esc":
		return m.clearSearch(), nil
	case "enter":
		m.search = m.search.Commit()
		return m, nil
	default:
		return m.ForwardMsg(msg)
	}
}

// ForwardMsg also accepts paste results and cursor updates from textinput.
func (m DestinationPickerModal) ForwardMsg(msg tea.Msg) (DestinationPickerModal, tea.Cmd) {
	before := m.search.Text
	var cmd tea.Cmd
	m.search, cmd = m.search.UpdateMsg(msg)
	if m.search.Text != before {
		m.cursor = 0
		if len(m.matchingPlaylists()) > 0 {
			m.cursor = 1
		}
	}
	return m, cmd
}

func (m DestinationPickerModal) clearSearch() DestinationPickerModal {
	matches := m.matchingPlaylists()
	cursor := clamp(m.cursor, 0, len(matches)+1)
	switch cursor {
	case 0:
		m.cursor = 0
	case len(matches) + 1:
		m.cursor = len(m.playlists) + 1
	default:
		m.cursor = matches[cursor-1] + 1
	}
	m.search = m.search.Deactivate()
	return m
}

func (m DestinationPickerModal) searchLabel() string {
	if len(m.matchingPlaylists()) == 0 && m.search.Text != "" {
		return "/ " + m.search.View() + " · No matching playlists"
	}
	return "/ " + m.search.View()
}
