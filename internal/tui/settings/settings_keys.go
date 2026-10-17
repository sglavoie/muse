package settings

import (
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/tui/overlay"
)

// ForwardMsg routes a generic tea.Msg to whichever text input is currently
// active (primary-color, excluded-path, library-dir, or EQ-name). Returns false if no input
// is active. This enables paste and other multi-step textinput commands.
func (o SettingsOverlay) ForwardMsg(msg tea.Msg) (SettingsOverlay, tea.Cmd, bool) {
	if o.discard.Active {
		return o, nil, true
	}
	if o.PrimaryInput != nil {
		updated, cmd := o.PrimaryInput.Update(msg)
		o.PrimaryInput = &updated
		return o, cmd, true
	}
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

// settingsKeyFunc applies one non-modal settings key to the overlay.
type settingsKeyFunc func(SettingsOverlay) (SettingsOverlay, Action, tea.Cmd)

// settingsNavKeys maps each key handled outside a modal state to its action.
// Keys absent from the table leave the overlay untouched.
var settingsNavKeys = map[string]settingsKeyFunc{
	"tab":       func(o SettingsOverlay) (SettingsOverlay, Action, tea.Cmd) { return o.switchTab(1) },
	"shift+tab": func(o SettingsOverlay) (SettingsOverlay, Action, tea.Cmd) { return o.switchTab(-1) },
	"[":         func(o SettingsOverlay) (SettingsOverlay, Action, tea.Cmd) { return o.switchTab(-1) },
	"]":         func(o SettingsOverlay) (SettingsOverlay, Action, tea.Cmd) { return o.switchTab(1) },
	"j":         func(o SettingsOverlay) (SettingsOverlay, Action, tea.Cmd) { return o.withCursorMoved(1) },
	"down":      func(o SettingsOverlay) (SettingsOverlay, Action, tea.Cmd) { return o.withCursorMoved(1) },
	"k":         func(o SettingsOverlay) (SettingsOverlay, Action, tea.Cmd) { return o.withCursorMoved(-1) },
	"up":        func(o SettingsOverlay) (SettingsOverlay, Action, tea.Cmd) { return o.withCursorMoved(-1) },
	"h":         func(o SettingsOverlay) (SettingsOverlay, Action, tea.Cmd) { return o.adjustField(-1) },
	"left":      func(o SettingsOverlay) (SettingsOverlay, Action, tea.Cmd) { return o.adjustField(-1) },
	"l":         func(o SettingsOverlay) (SettingsOverlay, Action, tea.Cmd) { return o.adjustField(1) },
	"right":     func(o SettingsOverlay) (SettingsOverlay, Action, tea.Cmd) { return o.adjustField(1) },
	"enter":     SettingsOverlay.beginPrimaryEdit,
	"r":         SettingsOverlay.resetPrimary,
	"a":         SettingsOverlay.beginAdd,
	"s":         SettingsOverlay.beginSavePreset,
	"d":         SettingsOverlay.beginDelete,
	"e":         SettingsOverlay.enterExcludedSubView,
	"esc":       SettingsOverlay.handleEscape,
	"ctrl+s":    func(o SettingsOverlay) (SettingsOverlay, Action, tea.Cmd) { return o, ActionSave, nil },
}

// HandleKey processes a key event and returns the updated overlay and the resulting action.
// ctrl+c returns settingsNone so the caller can fall through to root quit handling.
func (o SettingsOverlay) HandleKey(msg tea.KeyPressMsg) (SettingsOverlay, Action, tea.Cmd) {
	if o.discard.Active {
		var discard bool
		o.discard, discard = o.discard.HandleKey(msg.String())
		if discard {
			return o, ActionCancel, nil
		}
		return o, actionNone, nil
	}
	if o.PrimaryInput != nil {
		return o.handlePrimaryInput(msg)
	}
	if modal, ok := o.modalKeyHandler(); ok {
		return modal(msg)
	}
	if handle, ok := settingsNavKeys[msg.String()]; ok {
		return handle(o)
	}
	return o, actionNone, nil
}

// modalKeyHandler returns the handler that owns every key while a text input
// or a delete confirmation is open, or false when the overlay is in its
// ordinary navigation mode.
func (o SettingsOverlay) modalKeyHandler() (func(tea.KeyPressMsg) (SettingsOverlay, Action, tea.Cmd), bool) {
	inExcluded := o.Tab == tabLibrary && o.libSubView == libSubViewExcluded
	switch {
	case inExcluded && o.ExclPathInput != nil:
		return o.handleExclPathInput, true
	case inExcluded && o.exclConfirmDel:
		return o.handleExclDeleteConfirm, true
	case o.LibDirInput != nil:
		return o.handleLibDirInput, true
	case o.libConfirmDel:
		return o.handleLibDeleteConfirm, true
	case o.EQNameInput != nil:
		return o.handleEQNameInput, true
	case o.EQConfirmDel:
		return o.handleEQDeleteConfirm, true
	}
	return nil, false
}

// switchTab moves to the neighbouring tab, resetting the field cursor and
// returning the library tab to its directory sub-view.
func (o SettingsOverlay) switchTab(delta Tab) (SettingsOverlay, Action, tea.Cmd) {
	o.Tab = (o.Tab + tabCount + delta) % tabCount
	o.cursor = 0
	o.libSubView = libSubViewDirs
	return o, actionNone, nil
}

// withCursorMoved is moveCursor in the shape the key table expects.
func (o SettingsOverlay) withCursorMoved(delta int) (SettingsOverlay, Action, tea.Cmd) {
	o.moveCursor(delta)
	return o, actionNone, nil
}

// beginAdd opens the text input for a new library directory or excluded path.
func (o SettingsOverlay) beginAdd() (SettingsOverlay, Action, tea.Cmd) {
	if o.Tab != tabLibrary {
		return o, actionNone, nil
	}
	if o.libSubView == libSubViewExcluded {
		o.ExclPathInput = NewSettingsInput()
	} else {
		o.LibDirInput = NewSettingsInput()
		o.libDirError = ""
	}
	return o, actionNone, nil
}

// beginSavePreset opens the name prompt for saving the current EQ bands.
func (o SettingsOverlay) beginSavePreset() (SettingsOverlay, Action, tea.Cmd) {
	if o.Tab == TabEqualizer {
		o.EQNameInput = NewSettingsInput()
	}
	return o, actionNone, nil
}

// beginDelete arms the delete confirmation for whatever the current tab shows.
func (o SettingsOverlay) beginDelete() (SettingsOverlay, Action, tea.Cmd) {
	if o.Tab == tabLibrary {
		return o.armLibraryDelete(), actionNone, nil
	}
	if o.Tab == TabEqualizer {
		return o.armPresetDelete(), actionNone, nil
	}
	return o, actionNone, nil
}

// armLibraryDelete arms the confirmation for the focused excluded path or
// library directory; the last remaining directory cannot be removed.
func (o SettingsOverlay) armLibraryDelete() SettingsOverlay {
	if o.libSubView == libSubViewExcluded {
		if len(o.ExcludedPaths) > 0 {
			o.exclConfirmDel = true
		}
		return o
	}
	if len(o.LibraryDirs) > 1 {
		o.libConfirmDel = true
	}
	return o
}

// armPresetDelete arms the confirmation for the focused EQ preset. Built-in
// presets and the "(Custom)" sentinel are not deletable.
func (o SettingsOverlay) armPresetDelete() SettingsOverlay {
	presetIdx := o.Fields[TabEqualizer][1].Value
	if presetIdx < 0 || presetIdx >= len(o.EQPresets) {
		return o
	}
	p := o.EQPresets[presetIdx]
	if !p.Builtin && p.Key != "custom" {
		o.EQConfirmDel = true
	}
	return o
}

// enterExcludedSubView switches the library tab from directories to the
// excluded-path list.
func (o SettingsOverlay) enterExcludedSubView() (SettingsOverlay, Action, tea.Cmd) {
	if o.Tab == tabLibrary && o.libSubView == libSubViewDirs {
		o.libSubView = libSubViewExcluded
		o.exclCursor = 0
	}
	return o, actionNone, nil
}

// handleEscape leaves the excluded-path sub-view if it is open, and otherwise
// cancels the overlay.
func (o SettingsOverlay) handleEscape() (SettingsOverlay, Action, tea.Cmd) {
	if o.Tab == tabLibrary && o.libSubView == libSubViewExcluded {
		o.libSubView = libSubViewDirs
		return o, actionNone, nil
	}
	if o.hasChanges() {
		o.discard = overlay.DiscardPrompt{Active: true}
		return o, actionNone, nil
	}
	return o, ActionCancel, nil
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
		return o, actionNone, nil
	case "esc":
		o.ExclPathInput = nil
		return o, actionNone, nil
	default:
		updated, cmd := o.ExclPathInput.Update(msg)
		o.ExclPathInput = &updated
		return o, actionNone, cmd
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
		return o, actionNone, nil
	case "esc":
		o.exclConfirmDel = false
		return o, actionNone, nil
	default:
		return o, actionNone, nil
	}
}

// handleLibDirInput processes keys while the library-directory text input is active.
func (o SettingsOverlay) handleLibDirInput(msg tea.KeyPressMsg) (SettingsOverlay, Action, tea.Cmd) {
	switch msg.String() {
	case "enter":
		path, err := validateLibraryDir(o.LibDirInput.Value(), o.LibraryDirs)
		if err != nil {
			o.libDirError = err.Error()
			return o, actionNone, nil
		}
		o.LibDirInput = nil
		o.libDirError = ""
		o.LibraryDirs = append(o.LibraryDirs, path)
		o.libCursor = len(o.LibraryDirs) - 1
		o.dirty = true
		return o, actionNone, nil
	case "esc":
		o.LibDirInput = nil
		o.libDirError = ""
		return o, actionNone, nil
	default:
		updated, cmd := o.LibDirInput.Update(msg)
		o.LibDirInput = &updated
		return o, actionNone, cmd
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
		return o, actionNone, nil
	case "esc":
		o.libConfirmDel = false
		return o, actionNone, nil
	default:
		return o, actionNone, nil
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
		return o, actionNone, nil
	case "esc":
		o.EQNameInput = nil
		return o, actionNone, nil
	default:
		updated, cmd := o.EQNameInput.Update(msg)
		o.EQNameInput = &updated
		return o, actionNone, cmd
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
		return o, actionNone, nil
	default:
		return o, actionNone, nil
	}
}

// moveCursor advances the active list cursor by delta (+1 for down, -1 for up),
// wrapping around the list boundaries.
func (o *SettingsOverlay) moveCursor(delta int) {
	if o.Tab == tabLibrary {
		o.moveLibraryCursor(delta)
		return
	}
	if n := len(o.Fields[o.Tab]); n > 0 {
		o.cursor = (o.cursor + n + delta) % n
	}
}

// moveLibraryCursor wraps the cursor of whichever list the Library tab is
// currently showing.
func (o *SettingsOverlay) moveLibraryCursor(delta int) {
	if o.libSubView == libSubViewExcluded {
		if n := len(o.ExcludedPaths); n > 0 {
			o.exclCursor = (o.exclCursor + n + delta) % n
		}
		return
	}
	if n := len(o.LibraryDirs); n > 0 {
		o.libCursor = (o.libCursor + n + delta) % n
	}
}

// adjustField changes the focused slider field by one step in the given direction
// (sign: -1 for decrease, +1 for increase), clamping to [Min, Max].
func (o SettingsOverlay) adjustField(sign int) (SettingsOverlay, Action, tea.Cmd) {
	if o.Tab == tabLibrary || len(o.Fields[o.Tab]) == 0 {
		return o, actionNone, nil
	}
	if o.Tab == TabDisplay && o.cursor == displayPrimary {
		return o.beginPrimaryEdit()
	}
	f := &o.Fields[o.Tab][o.cursor]
	prev := f.Value
	f.Value = max(f.min, min(f.max, f.Value+sign*f.step))
	if f.Value == prev {
		return o, actionNone, nil
	}
	o.dirty = true
	return o, o.fieldChangeAction(), nil
}

func (o *SettingsOverlay) fieldChangeAction() Action {
	switch o.Tab {
	case TabDisplay:
		if o.cursor == displayTheme {
			return ActionThemeLive
		}
	case TabEqualizer:
		return o.applyEQFieldChange()
	case TabPlayback:
		if o.cursor == 6 || o.cursor == 7 {
			return ActionStereoLive
		}
	}
	return actionNone
}
