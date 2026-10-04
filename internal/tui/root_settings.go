package tui

import (
	"fmt"
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/config"
	"github.com/sglavoie/muse/internal/tui/keys"
	"github.com/sglavoie/muse/internal/tui/settings"
	"github.com/sglavoie/muse/internal/tui/statusbar"
)

// settingsSavedMsg is dispatched after config.Save completes.
type settingsSavedMsg struct{ err error }

type playerApplyStep struct {
	name     string
	apply    func() error
	rollback func() error
}

func cloneSettingsOverlay(o settings.SettingsOverlay) settings.SettingsOverlay {
	clone := o
	for i := range o.Fields {
		if o.Fields[i] != nil {
			clone.Fields[i] = append([]settings.Field(nil), o.Fields[i]...)
		}
	}
	if o.EQPresets != nil {
		clone.EQPresets = append([]config.EQPresetDef(nil), o.EQPresets...)
	}
	if o.LibraryDirs != nil {
		clone.LibraryDirs = append([]string(nil), o.LibraryDirs...)
	}
	if o.ExcludedPaths != nil {
		clone.ExcludedPaths = append([]string(nil), o.ExcludedPaths...)
	}
	if o.LibDirInput != nil {
		input := *o.LibDirInput
		clone.LibDirInput = &input
	}
	if o.ExclPathInput != nil {
		input := *o.ExclPathInput
		clone.ExclPathInput = &input
	}
	if o.EQNameInput != nil {
		input := *o.EQNameInput
		clone.EQNameInput = &input
	}
	if o.PrimaryInput != nil {
		input := *o.PrimaryInput
		clone.PrimaryInput = &input
	}
	return clone
}

func applyPlayerStep(step *playerApplyStep) error {
	if step == nil || step.apply == nil {
		return nil
	}
	if err := step.apply(); err != nil {
		if step.rollback != nil {
			_ = step.rollback()
		}
		return err
	}
	return nil
}

func applyPlayerSteps(steps ...*playerApplyStep) error {
	applied := make([]*playerApplyStep, 0, len(steps))
	for _, step := range steps {
		if step == nil || step.apply == nil {
			continue
		}
		if err := step.apply(); err != nil {
			if step.rollback != nil {
				_ = step.rollback()
			}
			for i := len(applied) - 1; i >= 0; i-- {
				if applied[i].rollback != nil {
					_ = applied[i].rollback()
				}
			}
			return fmt.Errorf("%s: %w", step.name, err)
		}
		applied = append(applied, step)
	}
	return nil
}

func (m Model) normalizeApplyStep(from, to bool) *playerApplyStep {
	if from == to {
		return nil
	}
	ns, ok := m.player.(audioNormalizer)
	if !ok {
		return nil
	}
	return &playerApplyStep{
		name: "volume normalization",
		apply: func() error {
			return ns.SetNormalize(to)
		},
		rollback: func() error {
			return ns.SetNormalize(from)
		},
	}
}

func (m Model) gaplessApplyStep(from, to bool) *playerApplyStep {
	if from == to {
		return nil
	}
	gt, ok := m.player.(gaplessToggler)
	if !ok {
		return nil
	}
	return &playerApplyStep{
		name: "gapless playback",
		apply: func() error {
			return gt.SetGapless(to)
		},
		rollback: func() error {
			return gt.SetGapless(from)
		},
	}
}

func (m Model) equalizerApplyStep(fromEnabled bool, fromBands [10]int, toEnabled bool, toBands [10]int) *playerApplyStep {
	if fromEnabled == toEnabled && fromBands == toBands {
		return nil
	}
	eq, ok := m.player.(equalizerSetter)
	if !ok {
		return nil
	}
	return &playerApplyStep{
		name: "equalizer",
		apply: func() error {
			return eq.SetEqualizer(toEnabled, toBands)
		},
		rollback: func() error {
			return eq.SetEqualizer(fromEnabled, fromBands)
		},
	}
}

func (m Model) stereoWideApplyStep(fromEnabled bool, fromAmount int, toEnabled bool, toAmount int) *playerApplyStep {
	if fromEnabled == toEnabled && fromAmount == toAmount {
		return nil
	}
	sw, ok := m.player.(stereoWideSetter)
	if !ok {
		return nil
	}
	return &playerApplyStep{
		name: "stereo widening",
		apply: func() error {
			return sw.SetStereoWide(toEnabled, toAmount)
		},
		rollback: func() error {
			return sw.SetStereoWide(fromEnabled, fromAmount)
		},
	}
}

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
	previous := cloneSettingsOverlay(*m.overlay.settings)
	updated, action, cmd := m.overlay.settings.HandleKey(msg)
	m.overlay.settings = &updated
	switch action {
	case settings.ActionCancel:
		prevEQEnabled, prevEQBands := previous.EQState()
		prevStereoEnabled, prevStereoAmount := previous.StereoWideState()
		if err := applyPlayerSteps(
			m.equalizerApplyStep(prevEQEnabled, prevEQBands, m.cfg.EQEnabled, m.cfg.EQBands),
			m.stereoWideApplyStep(prevStereoEnabled, prevStereoAmount, m.cfg.StereoWideEnabled, m.cfg.StereoWideAmount),
		); err != nil {
			restored := previous
			m.overlay.settings = &restored
			m.statusBar.NotifyError("Settings cancel failed: " + err.Error())
			return m, cmd, true
		}
		m.overlay.settings = nil
		m = m.applyTheme()
		return m, cmd, true
	case settings.ActionSave:
		saved, saveCmd := m.handleSettingsSave()
		return saved, tea.Batch(cmd, saveCmd), true
	case settings.ActionThemeLive:
		return m.applyTheme(), cmd, true
	case settings.ActionEQLive:
		prevEnabled, prevBands := previous.EQState()
		nextEnabled, nextBands := updated.EQState()
		if err := applyPlayerStep(m.equalizerApplyStep(prevEnabled, prevBands, nextEnabled, nextBands)); err != nil {
			restored := previous
			m.overlay.settings = &restored
			m.statusBar.NotifyError("Equalizer preview failed: " + err.Error())
		}
		return m, cmd, true
	case settings.ActionStereoLive:
		prevEnabled, prevAmount := previous.StereoWideState()
		nextEnabled, nextAmount := updated.StereoWideState()
		if err := applyPlayerStep(m.stereoWideApplyStep(prevEnabled, prevAmount, nextEnabled, nextAmount)); err != nil {
			restored := previous
			m.overlay.settings = &restored
			m.statusBar.NotifyError("Stereo widening preview failed: " + err.Error())
		}
		return m, cmd, true
	case settings.ActionEQSave:
		return m, cmd, true
	case settings.ActionEQDelete:
		prevEnabled, prevBands := previous.EQState()
		nextEnabled, nextBands := updated.EQState()
		step := m.equalizerApplyStep(prevEnabled, prevBands, nextEnabled, nextBands)
		if step == nil {
			if eq, ok := m.player.(equalizerSetter); ok {
				step = &playerApplyStep{
					name: "equalizer",
					apply: func() error {
						return eq.SetEqualizer(nextEnabled, nextBands)
					},
					rollback: func() error {
						return eq.SetEqualizer(prevEnabled, prevBands)
					},
				}
			}
		}
		if err := applyPlayerStep(step); err != nil {
			restored := previous
			restored.EQConfirmDel = false
			m.overlay.settings = &restored
			m.statusBar.NotifyError("Equalizer preview failed: " + err.Error())
		}
		return m, cmd, true
	}
	return m, cmd, true
}

// handleSettingsOpen creates a new SettingsOverlay from the current config.
func (m Model) handleSettingsOpen() (tea.Model, tea.Cmd) {
	overlay := settings.NewSettingsOverlay(m.cfg, m.playback.volume, m.cfg.MusicDirectories)
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
	oldCfg := m.cfg
	oldRoots := m.libraryRoots
	oldExcluded := oldCfg.ExcludedPaths
	cfg := m.overlay.settings.ToConfig(m.cfg)
	if err := applyPlayerSteps(
		m.normalizeApplyStep(oldCfg.NormalizeVolume, cfg.NormalizeVolume),
		m.gaplessApplyStep(oldCfg.GaplessAudio, cfg.GaplessAudio),
		m.equalizerApplyStep(oldCfg.EQEnabled, oldCfg.EQBands, cfg.EQEnabled, cfg.EQBands),
		m.stereoWideApplyStep(oldCfg.StereoWideEnabled, oldCfg.StereoWideAmount, cfg.StereoWideEnabled, cfg.StereoWideAmount),
	); err != nil {
		m.statusBar.NotifyError("Settings save failed: " + err.Error())
		return m, nil
	}
	m.cfg = cfg
	m.statusBar.TransientDur = time.Duration(cfg.TransientDurationSecs) * time.Second
	// Keep the runtime mirror in step with the Display tab's Track notify row,
	// otherwise the saved setting would not take effect until the next launch.
	m.notifyOnTrackChange = cfg.NotifyOnTrackChange
	m.overlay.settings = nil
	m = m.applyTheme()

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
		rescanModel, rescanCmd, _ := m.handleLibraryKeyAction(keys.ActionRescan)
		if rescanCmd != nil {
			return rescanModel.(Model), tea.Batch(saveCmd, rescanCmd)
		}
	}

	return m, saveCmd
}

// handleSettingsSavedMsg shows a notification after config.Save completes.
func (m Model) handleSettingsSavedMsg(msg settingsSavedMsg) Model {
	if msg.err != nil {
		m.statusBar.NotifyError("Settings save failed: " + msg.err.Error())
	} else {
		m.statusBar.PushMsg("Settings saved", time.Duration(m.cfg.TransientDurationSecs)*time.Second, statusbar.NotifNormal)
	}
	return m
}
