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
	// Excluded path input mode.
	if o.tab == settingsTabLibrary && o.libSubView == libSubViewExcluded && o.exclPathInput != nil {
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

	// Excluded path delete confirmation mode.
	if o.tab == settingsTabLibrary && o.libSubView == libSubViewExcluded && o.exclConfirmDel {
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

	// Library add mode: route all keys to the path text input.
	if o.libDirInput != nil {
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

	// Library delete confirmation mode.
	if o.libConfirmDel {
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

	// Text input mode: all keys route to the name input.
	if o.eqNameInput != nil {
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

	// Delete confirmation mode.
	if o.eqConfirmDel {
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
		if o.tab == settingsTabLibrary {
			if o.libSubView == libSubViewExcluded {
				if n := len(o.excludedPaths); n > 0 {
					o.exclCursor = (o.exclCursor + 1) % n
				}
			} else {
				if n := len(o.libraryDirs); n > 0 {
					o.libCursor = (o.libCursor + 1) % n
				}
			}
		} else {
			if n := len(o.fields[o.tab]); n > 0 {
				o.cursor = (o.cursor + 1) % n
			}
		}
	case "k", "up":
		if o.tab == settingsTabLibrary {
			if o.libSubView == libSubViewExcluded {
				if n := len(o.excludedPaths); n > 0 {
					o.exclCursor = (o.exclCursor + n - 1) % n
				}
			} else {
				if n := len(o.libraryDirs); n > 0 {
					o.libCursor = (o.libCursor + n - 1) % n
				}
			}
		} else {
			if n := len(o.fields[o.tab]); n > 0 {
				o.cursor = (o.cursor + n - 1) % n
			}
		}
	case "h", "left":
		if o.tab != settingsTabLibrary && len(o.fields[o.tab]) > 0 {
			f := &o.fields[o.tab][o.cursor]
			if f.Value > f.Min {
				f.Value -= f.Step
				if f.Value < f.Min {
					f.Value = f.Min
				}
				o.dirty = true
				if o.tab == settingsTabEqualizer {
					return o, o.applyEQFieldChange(), nil
				}
				if o.tab == settingsTabPlayback && (o.cursor == 6 || o.cursor == 7) {
					return o, settingsStereoLive, nil
				}
			}
		}
	case "l", "right":
		if o.tab != settingsTabLibrary && len(o.fields[o.tab]) > 0 {
			f := &o.fields[o.tab][o.cursor]
			if f.Value < f.Max {
				f.Value += f.Step
				if f.Value > f.Max {
					f.Value = f.Max
				}
				o.dirty = true
				if o.tab == settingsTabEqualizer {
					return o, o.applyEQFieldChange(), nil
				}
				if o.tab == settingsTabPlayback && (o.cursor == 6 || o.cursor == 7) {
					return o, settingsStereoLive, nil
				}
			}
		}
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
