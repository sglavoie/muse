package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/config"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// themeState returns the preset and primary color in effect, preferring the
// settings overlay's unsaved choice while it is open.
func (m Model) themeState() (preset, primary string) {
	if m.overlay.settings != nil {
		return m.overlay.settings.ThemeState()
	}
	return m.cfg.Theme, m.cfg.PrimaryColor
}

// effectivePrimary is the primary color the theme draws with: the cover
// accent while one applies, except while Settings previews a choice.
func (m Model) effectivePrimary(primary string) string {
	if m.overlay.settings == nil && m.accentOverride != "" {
		return m.accentOverride
	}
	return primary
}

func (m Model) applyTheme() Model {
	preset, primary := m.themeState()
	hasDark := m.terminalHasDark
	switch preset {
	case "dark":
		hasDark = true
	case "light":
		hasDark = false
	}
	s := styles.NewTheme(hasDark, m.effectivePrimary(primary))
	m.styles = &s
	if m.helpVisible {
		m.refreshHelpViewport()
	}
	if m.debugVisible {
		panel := collectDebugInfo(m)
		m.debugViewport.SetContent(panel.render(*m.styles))
	}
	return m
}

// Toggle the effective preset, including when Auto selected it. The primary
// override and unrelated preferences are preserved.
func (m Model) handleToggleThemeMsg() (Model, tea.Cmd) {
	m.cfg.Theme = "dark"
	label := "Dark"
	if m.styles.HasDark {
		m.cfg.Theme = "light"
		label = "Light"
	}
	m = m.applyTheme()
	m.statusBar.Notify("Theme: " + label)
	if m.configPath == "" {
		return m, nil
	}
	path, cfg := m.configPath, m.cfg
	return m, func() tea.Msg { return settingsSavedMsg{err: config.Save(path, cfg)} }
}
