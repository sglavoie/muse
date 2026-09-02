package settings

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
	if o.ExclPathInput != nil {
		updated, cmd := o.ExclPathInput.Update(msg)
		o.ExclPathInput = &updated
		return o, cmd, true
	}
	if o.LibDirInput != nil {
		updated, cmd := o.LibDirInput.Update(msg)
		o.LibDirInput = &updated
		return o, cmd, true
	}
	if o.EQNameInput != nil {
		updated, cmd := o.EQNameInput.Update(msg)
		o.EQNameInput = &updated
		return o, cmd, true
	}
	return o, nil, false
}

// HandleKey processes a key event and returns the updated overlay and the resulting action.
// ctrl+c returns settingsNone so the caller can fall through to root quit handling.
func (o SettingsOverlay) HandleKey(msg tea.KeyPressMsg) (SettingsOverlay, Action, tea.Cmd) {
	// Modal states: text inputs and delete confirmations consume all keys.
	if o.Tab == TabLibrary && o.libSubView == libSubViewExcluded && o.ExclPathInput != nil {
		return o.handleExclPathInput(msg)
	}
	if o.Tab == TabLibrary && o.libSubView == libSubViewExcluded && o.exclConfirmDel {
		return o.handleExclDeleteConfirm(msg)
	}
	if o.LibDirInput != nil {
		return o.handleLibDirInput(msg)
	}
	if o.libConfirmDel {
		return o.handleLibDeleteConfirm(msg)
	}
	if o.EQNameInput != nil {
		return o.handleEQNameInput(msg)
	}
	if o.EQConfirmDel {
		return o.handleEQDeleteConfirm(msg)
	}

	switch msg.String() {
	case "[":
		o.Tab = (o.Tab + TabCount - 1) % TabCount
		o.cursor = 0
		o.libSubView = libSubViewDirs
	case "]":
		o.Tab = (o.Tab + 1) % TabCount
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
		if o.Tab == TabLibrary {
			if o.libSubView == libSubViewExcluded {
				o.ExclPathInput = NewSettingsInput()
			} else {
				o.LibDirInput = NewSettingsInput()
			}
			return o, ActionNone, nil
		}
	case "s":
		if o.Tab == TabEqualizer {
			o.EQNameInput = NewSettingsInput()
			return o, ActionNone, nil
		}
	case "d":
		if o.Tab == TabLibrary {
			if o.libSubView == libSubViewExcluded {
				if len(o.ExcludedPaths) > 0 {
					o.exclConfirmDel = true
				}
			} else {
				if len(o.LibraryDirs) > 1 {
					o.libConfirmDel = true
				}
			}
			return o, ActionNone, nil
		}
		if o.Tab == TabEqualizer {
			presetIdx := o.Fields[TabEqualizer][1].Value
			if presetIdx >= 0 && presetIdx < len(o.EQPresets) {
				p := o.EQPresets[presetIdx]
				if !p.Builtin && p.Key != "custom" {
					o.EQConfirmDel = true
					return o, ActionNone, nil
				}
			}
		}
	case "e":
		if o.Tab == TabLibrary && o.libSubView == libSubViewDirs {
			o.libSubView = libSubViewExcluded
			o.exclCursor = 0
			return o, ActionNone, nil
		}
	case "esc":
		if o.Tab == TabLibrary && o.libSubView == libSubViewExcluded {
			o.libSubView = libSubViewDirs
			return o, ActionNone, nil
		}
		return o, ActionCancel, nil
	case "ctrl+s":
		return o, ActionSave, nil
	}
	return o, ActionNone, nil
}

// handleExclPathInput processes keys while the excluded-path text input is active.
func (o SettingsOverlay) handleExclPathInput(msg tea.KeyPressMsg) (SettingsOverlay, Action, tea.Cmd) {
	switch msg.String() {
	case "enter":
		pattern := strings.TrimSpace(o.ExclPathInput.Value())
		o.ExclPathInput = nil
		if pattern != "" {
			if !slices.Contains(o.ExcludedPaths, pattern) {
				o.ExcludedPaths = append(o.ExcludedPaths, pattern)
				o.exclCursor = len(o.ExcludedPaths) - 1
				o.dirty = true
			}
		}
		return o, ActionNone, nil
	case "esc":
		o.ExclPathInput = nil
		return o, ActionNone, nil
	default:
		updated, cmd := o.ExclPathInput.Update(msg)
		o.ExclPathInput = &updated
		return o, ActionNone, cmd
	}
}

// handleExclDeleteConfirm processes keys while confirming excluded-path deletion.
func (o SettingsOverlay) handleExclDeleteConfirm(msg tea.KeyPressMsg) (SettingsOverlay, Action, tea.Cmd) {
	switch msg.String() {
	case "d", "enter":
		if o.exclCursor < len(o.ExcludedPaths) {
			o.ExcludedPaths = append(o.ExcludedPaths[:o.exclCursor], o.ExcludedPaths[o.exclCursor+1:]...)
			if o.exclCursor >= len(o.ExcludedPaths) && o.exclCursor > 0 {
				o.exclCursor = len(o.ExcludedPaths) - 1
			}
			o.dirty = true
		}
		o.exclConfirmDel = false
		return o, ActionNone, nil
	case "esc":
		o.exclConfirmDel = false
		return o, ActionNone, nil
	default:
		return o, ActionNone, nil
	}
}

// handleLibDirInput processes keys while the library-directory text input is active.
func (o SettingsOverlay) handleLibDirInput(msg tea.KeyPressMsg) (SettingsOverlay, Action, tea.Cmd) {
	switch msg.String() {
	case "enter":
		path := o.LibDirInput.Value()
		o.LibDirInput = nil
		if path != "" {
			expanded := expandTildeSettings(path)
			if _, err := os.Stat(expanded); err == nil {
				o.LibraryDirs = append(o.LibraryDirs, expanded)
				o.libCursor = len(o.LibraryDirs) - 1
				o.dirty = true
			}
		}
		return o, ActionNone, nil
	case "esc":
		o.LibDirInput = nil
		return o, ActionNone, nil
	default:
		updated, cmd := o.LibDirInput.Update(msg)
		o.LibDirInput = &updated
		return o, ActionNone, cmd
	}
}

// handleLibDeleteConfirm processes keys while confirming library-directory deletion.
func (o SettingsOverlay) handleLibDeleteConfirm(msg tea.KeyPressMsg) (SettingsOverlay, Action, tea.Cmd) {
	switch msg.String() {
	case "d", "enter":
		if len(o.LibraryDirs) > 1 {
			o.LibraryDirs = append(o.LibraryDirs[:o.libCursor], o.LibraryDirs[o.libCursor+1:]...)
			if o.libCursor >= len(o.LibraryDirs) {
				o.libCursor = len(o.LibraryDirs) - 1
			}
			o.dirty = true
		}
		o.libConfirmDel = false
		return o, ActionNone, nil
	case "esc":
		o.libConfirmDel = false
		return o, ActionNone, nil
	default:
		return o, ActionNone, nil
	}
}

// handleEQNameInput processes keys while the EQ preset name text input is active.
func (o SettingsOverlay) handleEQNameInput(msg tea.KeyPressMsg) (SettingsOverlay, Action, tea.Cmd) {
	switch msg.String() {
	case "enter":
		name := o.EQNameInput.Value()
		o.EQNameInput = nil
		if name != "" && o.addCustomPreset(name) {
			return o, ActionEQSave, nil
		}
		return o, ActionNone, nil
	case "esc":
		o.EQNameInput = nil
		return o, ActionNone, nil
	default:
		updated, cmd := o.EQNameInput.Update(msg)
		o.EQNameInput = &updated
		return o, ActionNone, cmd
	}
}

// handleEQDeleteConfirm processes keys while confirming EQ preset deletion.
func (o SettingsOverlay) handleEQDeleteConfirm(msg tea.KeyPressMsg) (SettingsOverlay, Action, tea.Cmd) {
	switch msg.String() {
	case "d", "enter":
		o.deleteCurrentPreset()
		o.EQConfirmDel = false
		return o, ActionEQDelete, nil
	case "esc":
		o.EQConfirmDel = false
		return o, ActionNone, nil
	default:
		return o, ActionNone, nil
	}
}

// moveCursor advances the active list cursor by delta (+1 for down, -1 for up),
// wrapping around the list boundaries.
func (o *SettingsOverlay) moveCursor(delta int) {
	if o.Tab == TabLibrary {
		if o.libSubView == libSubViewExcluded {
			if n := len(o.ExcludedPaths); n > 0 {
				o.exclCursor = (o.exclCursor + n + delta) % n
			}
		} else {
			if n := len(o.LibraryDirs); n > 0 {
				o.libCursor = (o.libCursor + n + delta) % n
			}
		}
		return
	}
	if n := len(o.Fields[o.Tab]); n > 0 {
		o.cursor = (o.cursor + n + delta) % n
	}
}

// adjustField changes the focused slider field by one step in the given direction
// (sign: -1 for decrease, +1 for increase), clamping to [Min, Max].
func (o SettingsOverlay) adjustField(sign int) (SettingsOverlay, Action, tea.Cmd) {
	if o.Tab == TabLibrary || len(o.Fields[o.Tab]) == 0 {
		return o, ActionNone, nil
	}
	f := &o.Fields[o.Tab][o.cursor]
	prev := f.Value
	f.Value += sign * f.Step
	if f.Value < f.Min {
		f.Value = f.Min
	}
	if f.Value > f.Max {
		f.Value = f.Max
	}
	if f.Value == prev {
		return o, ActionNone, nil
	}
	o.dirty = true
	if o.Tab == TabEqualizer {
		return o, o.applyEQFieldChange(), nil
	}
	if o.Tab == TabPlayback && (o.cursor == 6 || o.cursor == 7) {
		return o, ActionStereoLive, nil
	}
	return o, ActionNone, nil
}
