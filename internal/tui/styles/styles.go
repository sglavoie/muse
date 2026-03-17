package styles

import "charm.land/lipgloss/v2"

// Styles holds the shared lipgloss styles used across TUI views.
type Styles struct {
	Dim     lipgloss.Style
	Accent  lipgloss.Style
	Bold    lipgloss.Style
	HasDark bool
}

// NewStyles constructs a Styles value for the given terminal background.
func NewStyles(hasDark bool) Styles {
	return Styles{
		Dim:     lipgloss.NewStyle().Faint(true),
		Accent:  lipgloss.NewStyle().Reverse(true),
		Bold:    lipgloss.NewStyle().Bold(true),
		HasDark: hasDark,
	}
}
