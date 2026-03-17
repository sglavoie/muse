package styles

import "charm.land/lipgloss/v2"

// Styles holds the shared lipgloss styles used across TUI views.
type Styles struct {
	Dim        lipgloss.Style
	Accent     lipgloss.Style
	AccentMark lipgloss.Style // foreground-only accent for "●" marker on non-focused rows
	Bold       lipgloss.Style
	Heading    lipgloss.Style // bold + blue + underlined for section titles
	Success    lipgloss.Style // green foreground for playing state
	Warning    lipgloss.Style // gold foreground for paused state
	Error      lipgloss.Style // red foreground for errors / stopped
	ColHead    lipgloss.Style // bold + faint for column headers
	StatusBar  lipgloss.Style // full-width status bar background + foreground
	HasDark    bool
}

// NewStyles constructs a Styles value for the given terminal background.
func NewStyles(hasDark bool) Styles {
	ld := lipgloss.LightDark(hasDark)
	return Styles{
		Dim:        lipgloss.NewStyle().Faint(true),
		Accent:     lipgloss.NewStyle().Background(ld(lipgloss.Color("#d0d7e3"), lipgloss.Color("#2c3e50"))),
		AccentMark: lipgloss.NewStyle().Foreground(ld(lipgloss.Color("#4078f2"), lipgloss.Color("#61afef"))),
		Bold:       lipgloss.NewStyle().Bold(true),
		Heading:    lipgloss.NewStyle().Bold(true).Underline(true).Foreground(ld(lipgloss.Color("#4078f2"), lipgloss.Color("#61afef"))),
		Success:    lipgloss.NewStyle().Foreground(ld(lipgloss.Color("#50a14f"), lipgloss.Color("#98c379"))),
		Warning:    lipgloss.NewStyle().Foreground(ld(lipgloss.Color("#986801"), lipgloss.Color("#e5c07b"))),
		Error:      lipgloss.NewStyle().Foreground(ld(lipgloss.Color("#e45649"), lipgloss.Color("#e06c75"))),
		ColHead:    lipgloss.NewStyle().Bold(true).Faint(true),
		StatusBar:  lipgloss.NewStyle().Background(ld(lipgloss.Color("#e8eaed"), lipgloss.Color("#282c34"))).Foreground(ld(lipgloss.Color("#383a42"), lipgloss.Color("#abb2bf"))),
		HasDark:    hasDark,
	}
}
