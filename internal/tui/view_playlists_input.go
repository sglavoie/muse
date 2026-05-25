package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	pltui "github.com/sglavoie/muse/internal/tui/playlists"
)

// isCreating reports whether the inline create input is active.
func (v PlaylistsView) isCreating() bool {
	return v.creating
}

// startCreate activates the inline create input with an empty buffer.
func (v PlaylistsView) startCreate() PlaylistsView {
	view := pltui.View{Creating: v.creating, CreateInput: v.createInput}.StartCreate()
	v.creating = view.Creating
	v.createInput = view.CreateInput
	return v
}

// cancelCreate dismisses the inline create input without persisting.
func (v PlaylistsView) cancelCreate() PlaylistsView {
	view := pltui.View{Creating: v.creating, CreateInput: v.createInput}.CancelCreate()
	v.creating = view.Creating
	v.createInput = view.CreateInput
	return v
}

// updateCreateInput forwards a key message to the Bubbles textinput model.
func (v PlaylistsView) updateCreateInput(msg tea.KeyPressMsg) (PlaylistsView, tea.Cmd) {
	var cmd tea.Cmd
	v.createInput, cmd = v.createInput.Update(msg)
	return v, cmd
}

// updateCreateInputMsg forwards a generic tea.Msg to the textinput, enabling
// paste and other multi-step commands.
func (v PlaylistsView) updateCreateInputMsg(msg tea.Msg) (PlaylistsView, tea.Cmd) {
	var cmd tea.Cmd
	v.createInput, cmd = v.createInput.Update(msg)
	return v, cmd
}

// createInputValue returns the current create input buffer, trimmed of
// leading/trailing whitespace to prevent names like "  hello  ".
func (v PlaylistsView) createInputValue() string {
	return strings.TrimSpace(v.createInput.Value())
}

// createInputIsEmpty reports whether the trimmed create input buffer is empty.
func (v PlaylistsView) createInputIsEmpty() bool {
	return strings.TrimSpace(v.createInput.Value()) == ""
}

// isRenaming reports whether the inline rename input is active.
func (v PlaylistsView) isRenaming() bool {
	return v.renaming
}

// startRename activates the inline rename input pre-populated with currentName.
func (v PlaylistsView) startRename(currentName string) PlaylistsView {
	view := pltui.View{Renaming: v.renaming, RenameInput: v.renameInput}.StartRename(currentName)
	v.renaming = view.Renaming
	v.renameInput = view.RenameInput
	return v
}

// cancelRename dismisses the inline rename input without persisting.
func (v PlaylistsView) cancelRename() PlaylistsView {
	view := pltui.View{Renaming: v.renaming, RenameInput: v.renameInput}.CancelRename()
	v.renaming = view.Renaming
	v.renameInput = view.RenameInput
	return v
}

// updateRenameInput forwards a key message to the Bubbles textinput model.
func (v PlaylistsView) updateRenameInput(msg tea.KeyPressMsg) (PlaylistsView, tea.Cmd) {
	var cmd tea.Cmd
	v.renameInput, cmd = v.renameInput.Update(msg)
	return v, cmd
}

// updateRenameInputMsg forwards a generic tea.Msg to the textinput, enabling
// paste and other multi-step commands.
func (v PlaylistsView) updateRenameInputMsg(msg tea.Msg) (PlaylistsView, tea.Cmd) {
	var cmd tea.Cmd
	v.renameInput, cmd = v.renameInput.Update(msg)
	return v, cmd
}

// renameInputValue returns the current rename input buffer, trimmed of
// leading/trailing whitespace to prevent names like "  hello  ".
func (v PlaylistsView) renameInputValue() string {
	return strings.TrimSpace(v.renameInput.Value())
}

// renameInputIsEmpty reports whether the trimmed rename input buffer is empty.
func (v PlaylistsView) renameInputIsEmpty() bool {
	return strings.TrimSpace(v.renameInput.Value()) == ""
}
