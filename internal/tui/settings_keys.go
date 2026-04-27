package tui

import (
	"os"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// ForwardMsg routes a generic tea.Msg to whichever text input is currently
// active (excluded-path, library-dir, or EQ-name). Returns false if no input
// is active. This enables paste and other multi-step textinput commands.
func (o SettingsOverlay) ForwardMsg(msg tea.Msg) (SettingsOverlay, tea.Cmd, bool) {
	if o.exclPathInput != nil {
		updated, cmd := o.exclPathInput.Update(msg)
		o.exclPathInput = &updated
		return o, cmd, true
	}
	if o.libDirInput != nil {
		updated, cmd := o.libDirInput.Update(msg)
		o.libDirInput = &updated
		return o, cmd, true
	}
	if o.eqNameInput != nil {
		updated, cmd := o.eqNameInput.Update(msg)
		o.eqNameInput = &updated
		return o, cmd, true
	}
	return o, nil, false
}

// HandleKey processes a key event and returns the updated overlay and the resulting action.
// ctrl+c returns settingsNone so the caller can fall through to root quit handling.
func (o SettingsOverlay) HandleKey(msg tea.KeyPressMsg) (SettingsOverlay, settingsAction, tea.Cmd) {
	// Modal states: text inputs and delete confirmations consume all keys.
	if o.tab == settingsTabLibrary && o.libSubView == libSubViewExcluded && o.exclPathInput != nil {
		return o.handleExclPathInput(msg)
	}
	if o.tab == settingsTabLibrary && o.libSubView == libSubViewExcluded && o.exclConfirmDel {
		return o.handleExclDeleteConfirm(msg)
	}
	if o.libDirInput != nil {
		return o.handleLibDirInput(msg)
	}
	if o.libConfirmDel {
		return o.handleLibDeleteConfirm(msg)
	}
	if o.eqNameInput != nil {
		return o.handleEQNameInput(msg)
	}
	if o.eqConfirmDel {
		return o.handleEQDeleteConfirm(msg)
	}

	switch msg.String() {
	case "[":
		o.tab = (o.tab + settingsTabCount - 1) % settingsTabCount
		o.cursor = 0
		o.libSubView = libSubViewDirs
	case "]":
		o.tab = (o.tab + 1) % settingsTabCount
		o.cursor = 0
		o.libSubView = libSubViewDirs
	case "j", "down":
		o.moveCursor(1)
	case "k", "up":
		o.moveCursor(-1)
	case "h", "left":
		return o.adjustField(-1)
	case "l", "right":
		return o.adjustField(1)
	case "a":
		if o.tab == settingsTabLibrary {
			if o.libSubView == libSubViewExcluded {
				o.exclPathInput = newSettingsInput()
			} else {
				o.libDirInput = newSettingsInput()
			}
			return o, settingsNone, nil
		}
	case "s":
		if o.tab == settingsTabEqualizer {
			o.eqNameInput = newSettingsInput()
			return o, settingsNone, nil
		}
	case "d":
		if o.tab == settingsTabLibrary {
			if o.libSubView == libSubViewExcluded {
				if len(o.excludedPaths) > 0 {
					o.exclConfirmDel = true
				}
			} else {
				if len(o.libraryDirs) > 1 {
					o.libConfirmDel = true
				}
			}
			return o, settingsNone, nil
		}
		if o.tab == settingsTabEqualizer {
			presetIdx := o.fields[settingsTabEqualizer][1].Value
			if presetIdx >= 0 && presetIdx < len(o.eqPresets) {
				p := o.eqPresets[presetIdx]
				if !p.Builtin && p.Key != "custom" {
					o.eqConfirmDel = true
					return o, settingsNone, nil
				}
			}
		}
	case "e":
		if o.tab == settingsTabLibrary && o.libSubView == libSubViewDirs {
			o.libSubView = libSubViewExcluded
			o.exclCursor = 0
			return o, settingsNone, nil
		}
	case "esc":
		if o.tab == settingsTabLibrary && o.libSubView == libSubViewExcluded {
			o.libSubView = libSubViewDirs
			return o, settingsNone, nil
		}
		return o, settingsCancel, nil
	case "ctrl+s":
		return o, settingsSave, nil
	}
	return o, settingsNone, nil
}

// handleExclPathInput processes keys while the excluded-path text input is active.
func (o SettingsOverlay) handleExclPathInput(msg tea.KeyPressMsg) (SettingsOverlay, settingsAction, tea.Cmd) {
	switch msg.String() {
	case "enter":
		pattern := strings.TrimSpace(o.exclPathInput.Value())
		o.exclPathInput = nil
		if pattern != "" {
			if !slices.Contains(o.excludedPaths, pattern) {
				o.excludedPaths = append(o.excludedPaths, pattern)
				o.exclCursor = len(o.excludedPaths) - 1
				o.dirty = true
			}
		}
		return o, settingsNone, nil
	case "esc":
		o.exclPathInput = nil
		return o, settingsNone, nil
	default:
		updated, cmd := o.exclPathInput.Update(msg)
		o.exclPathInput = &updated
		return o, settingsNone, cmd
	}
}

// handleExclDeleteConfirm processes keys while confirming excluded-path deletion.
func (o SettingsOverlay) handleExclDeleteConfirm(msg tea.KeyPressMsg) (SettingsOverlay, settingsAction, tea.Cmd) {
	switch msg.String() {
	case "d", "enter":
		if o.exclCursor < len(o.excludedPaths) {
			o.excludedPaths = append(o.excludedPaths[:o.exclCursor], o.excludedPaths[o.exclCursor+1:]...)
			if o.exclCursor >= len(o.excludedPaths) && o.exclCursor > 0 {
				o.exclCursor = len(o.excludedPaths) - 1
			}
			o.dirty = true
		}
		o.exclConfirmDel = false
		return o, settingsNone, nil
	case "esc":
		o.exclConfirmDel = false
		return o, settingsNone, nil
	default:
		return o, settingsNone, nil
	}
}

// handleLibDirInput processes keys while the library-directory text input is active.
func (o SettingsOverlay) handleLibDirInput(msg tea.KeyPressMsg) (SettingsOverlay, settingsAction, tea.Cmd) {
	switch msg.String() {
	case "enter":
		path := o.libDirInput.Value()
		o.libDirInput = nil
		if path != "" {
			expanded := expandTildeSettings(path)
			if _, err := os.Stat(expanded); err == nil {
				o.libraryDirs = append(o.libraryDirs, expanded)
				o.libCursor = len(o.libraryDirs) - 1
				o.dirty = true
			}
		}
		return o, settingsNone, nil
	case "esc":
		o.libDirInput = nil
		return o, settingsNone, nil
	default:
		updated, cmd := o.libDirInput.Update(msg)
		o.libDirInput = &updated
		return o, settingsNone, cmd
	}
}

// handleLibDeleteConfirm processes keys while confirming library-directory deletion.
func (o SettingsOverlay) handleLibDeleteConfirm(msg tea.KeyPressMsg) (SettingsOverlay, settingsAction, tea.Cmd) {
	switch msg.String() {
	case "d", "enter":
		if len(o.libraryDirs) > 1 {
			o.libraryDirs = append(o.libraryDirs[:o.libCursor], o.libraryDirs[o.libCursor+1:]...)
			if o.libCursor >= len(o.libraryDirs) {
				o.libCursor = len(o.libraryDirs) - 1
			}
			o.dirty = true
		}
		o.libConfirmDel = false
		return o, settingsNone, nil
	case "esc":
		o.libConfirmDel = false
		return o, settingsNone, nil
	default:
		return o, settingsNone, nil
	}
}

// handleEQNameInput processes keys while the EQ preset name text input is active.
func (o SettingsOverlay) handleEQNameInput(msg tea.KeyPressMsg) (SettingsOverlay, settingsAction, tea.Cmd) {
	switch msg.String() {
	case "enter":
		name := o.eqNameInput.Value()
		o.eqNameInput = nil
		if name != "" && o.addCustomPreset(name) {
			return o, settingsEQSave, nil
		}
		return o, settingsNone, nil
	case "esc":
		o.eqNameInput = nil
		return o, settingsNone, nil
	default:
		updated, cmd := o.eqNameInput.Update(msg)
		o.eqNameInput = &updated
		return o, settingsNone, cmd
	}
}

// handleEQDeleteConfirm processes keys while confirming EQ preset deletion.
func (o SettingsOverlay) handleEQDeleteConfirm(msg tea.KeyPressMsg) (SettingsOverlay, settingsAction, tea.Cmd) {
	switch msg.String() {
	case "d", "enter":
		o.deleteCurrentPreset()
		o.eqConfirmDel = false
		return o, settingsEQDelete, nil
	case "esc":
		o.eqConfirmDel = false
		return o, settingsNone, nil
	default:
		return o, settingsNone, nil
	}
}

// moveCursor advances the active list cursor by delta (+1 for down, -1 for up),
// wrapping around the list boundaries.
func (o *SettingsOverlay) moveCursor(delta int) {
	if o.tab == settingsTabLibrary {
		if o.libSubView == libSubViewExcluded {
			if n := len(o.excludedPaths); n > 0 {
				o.exclCursor = (o.exclCursor + n + delta) % n
			}
		} else {
			if n := len(o.libraryDirs); n > 0 {
				o.libCursor = (o.libCursor + n + delta) % n
			}
		}
		return
	}
	if n := len(o.fields[o.tab]); n > 0 {
		o.cursor = (o.cursor + n + delta) % n
	}
}

// adjustField changes the focused slider field by one step in the given direction
// (sign: -1 for decrease, +1 for increase), clamping to [Min, Max].
func (o SettingsOverlay) adjustField(sign int) (SettingsOverlay, settingsAction, tea.Cmd) {
	if o.tab == settingsTabLibrary || len(o.fields[o.tab]) == 0 {
		return o, settingsNone, nil
	}
	f := &o.fields[o.tab][o.cursor]
	prev := f.Value
	f.Value += sign * f.Step
	if f.Value < f.Min {
		f.Value = f.Min
	}
	if f.Value > f.Max {
		f.Value = f.Max
	}
	if f.Value == prev {
		return o, settingsNone, nil
	}
	o.dirty = true
	if o.tab == settingsTabEqualizer {
		return o, o.applyEQFieldChange(), nil
	}
	if o.tab == settingsTabPlayback && (o.cursor == 6 || o.cursor == 7) {
		return o, settingsStereoLive, nil
	}
	return o, settingsNone, nil
}
