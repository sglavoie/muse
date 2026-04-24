package tui

import (
	"slices"
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
	updated, action, cmd := m.overlay.settings.HandleKey(msg)
	m.overlay.settings = &updated
	switch action {
	case settingsCancel:
		m.overlay.settings = nil
		if eq, ok := m.player.(equalizerSetter); ok {
			_ = eq.SetEqualizer(m.cfg.EQEnabled, m.cfg.EQBands)
		}
		if sw, ok := m.player.(stereoWideSetter); ok {
			_ = sw.SetStereoWide(m.cfg.StereoWideEnabled, m.cfg.StereoWideAmount)
		}
		return m, cmd, true
	case settingsSave:
		saved, saveCmd := m.handleSettingsSave()
		return saved, tea.Batch(cmd, saveCmd), true
	case settingsEQLive:
		if eq, ok := m.player.(equalizerSetter); ok {
			enabled, bands := m.overlay.settings.EQState()
			_ = eq.SetEqualizer(enabled, bands)
		}
		return m, cmd, true
	case settingsStereoLive:
		if sw, ok := m.player.(stereoWideSetter); ok {
			enabled, amount := m.overlay.settings.StereoWideState()
			_ = sw.SetStereoWide(enabled, amount)
		}
		return m, cmd, true
	case settingsEQSave:
		return m, cmd, true
	case settingsEQDelete:
		if eq, ok := m.player.(equalizerSetter); ok {
			enabled, bands := m.overlay.settings.EQState()
			_ = eq.SetEqualizer(enabled, bands)
		}
		return m, cmd, true
	}
	return m, cmd, true
}

// handleSettingsOpen creates a new SettingsOverlay from the current config.
func (m Model) handleSettingsOpen() (tea.Model, tea.Cmd) {
	overlay := NewSettingsOverlay(m.cfg, m.playback.volume, m.cfg.MusicDirectories)
	m.overlay.settings = &overlay
	return m, nil
}

// handleSettingsSave applies overlay values to config, writes to disk, and
// returns a Cmd that dispatches settingsSavedMsg with the result.
// If library directories or excluded paths changed, it also updates m.libraryRoots and triggers a rescan.
func (m Model) handleSettingsSave() (Model, tea.Cmd) {
	if m.overlay.settings == nil {
		return m, nil
	}
	oldRoots := m.libraryRoots
	oldExcluded := m.cfg.ExcludedPaths
	cfg := m.overlay.settings.ToConfig(m.cfg)
	m.cfg = cfg
	m.statusBar.transientDur = time.Duration(cfg.TransientDurationSecs) * time.Second
	m.overlay.settings = nil
	if ns, ok := m.player.(audioNormalizer); ok {
		_ = ns.SetNormalize(m.cfg.NormalizeVolume)
	}
	if gt, ok := m.player.(gaplessToggler); ok {
		_ = gt.SetGapless(m.cfg.GaplessAudio)
	}
	if eq, ok := m.player.(equalizerSetter); ok {
		_ = eq.SetEqualizer(m.cfg.EQEnabled, m.cfg.EQBands)
	}
	if sw, ok := m.player.(stereoWideSetter); ok {
		_ = sw.SetStereoWide(m.cfg.StereoWideEnabled, m.cfg.StereoWideAmount)
	}

	var saveCmd tea.Cmd
	if m.configPath == "" {
		saveCmd = func() tea.Msg { return settingsSavedMsg{} }
	} else {
		path := m.configPath
		saveCmd = func() tea.Msg { return settingsSavedMsg{err: config.Save(path, cfg)} }
	}

	// If library dirs or excluded paths changed, update roots and trigger a rescan.
	if !slices.Equal(oldRoots, cfg.MusicDirectories) || !slices.Equal(oldExcluded, cfg.ExcludedPaths) {
		m.libraryRoots = cfg.MusicDirectories
		rescanModel, rescanCmd, _ := m.handleLibraryKeyAction(actionRescan)
		if rescanCmd != nil {
			return rescanModel.(Model), tea.Batch(saveCmd, rescanCmd)
		}
	}

	return m, saveCmd
}

// handleSettingsSavedMsg shows a notification after config.Save completes.
func (m Model) handleSettingsSavedMsg(msg settingsSavedMsg) Model {
	if msg.err != nil {
		m.statusBar.notify("Settings save failed: " + msg.err.Error())
	} else {
		m.statusBar.notification = newNotificationFor(
			"Settings saved",
			time.Duration(m.cfg.TransientDurationSecs)*time.Second,
		)
	}
	return m
}
