package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/config"
)

// settingsSavedMsg is dispatched after config.Save completes.
type settingsSavedMsg struct{ err error }

// handleSettingsKeyPress intercepts keys while the settings overlay is active.
// Returns (model, cmd, true) when the key was consumed, (model, nil, false) to
// fall through to global key handling (ctrl+c only).
func (m Model) handleSettingsKeyPress(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	if m.overlay.settings == nil {
		return m, nil, false
	}
	// Let ctrl+c fall through so root can quit.
	if msg.String() == "ctrl+c" {
		return m, nil, false
	}
	updated, action := m.overlay.settings.HandleKey(msg)
	m.overlay.settings = &updated
	switch action {
	case settingsCancel:
		m.overlay.settings = nil
		return m, nil, true
	case settingsSave:
		saved, cmd := m.handleSettingsSave()
		return saved, cmd, true
	}
	return m, nil, true
}

// handleSettingsOpen creates a new SettingsOverlay from the current config.
func (m Model) handleSettingsOpen() (tea.Model, tea.Cmd) {
	overlay := NewSettingsOverlay(m.cfg)
	m.overlay.settings = &overlay
	return m, nil
}

// handleSettingsSave applies overlay values to config, writes to disk, and
// returns a Cmd that dispatches settingsSavedMsg with the result.
func (m Model) handleSettingsSave() (Model, tea.Cmd) {
	if m.overlay.settings == nil {
		return m, nil
	}
	cfg := m.overlay.settings.ToConfig(m.cfg)
	m.cfg = cfg
	m.overlay.settings = nil
	if ns, ok := m.player.(audioNormalizer); ok {
		_ = ns.SetNormalize(m.cfg.NormalizeVolume)
	}
	if m.configPath == "" {
		return m, func() tea.Msg { return settingsSavedMsg{} }
	}
	path := m.configPath
	return m, func() tea.Msg {
		return settingsSavedMsg{err: config.Save(path, cfg)}
	}
}

// handleSettingsSavedMsg shows a notification after config.Save completes.
func (m Model) handleSettingsSavedMsg(msg settingsSavedMsg) Model {
	if msg.err != nil {
		m.statusBar.notification = newNotification("Settings save failed: " + msg.err.Error())
	} else {
		m.statusBar.notification = newNotificationFor(
			"Settings saved",
			time.Duration(m.cfg.TransientDurationSecs)*time.Second,
		)
	}
	return m
}
