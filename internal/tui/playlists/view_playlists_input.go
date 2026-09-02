package playlists

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

// IsCreating reports whether the inline create input is active.
func (v PlaylistsView) IsCreating() bool {
	return v.Creating
}

// StartCreate activates the inline create input with an empty buffer.
func (v PlaylistsView) StartCreate() PlaylistsView {
	view := View{Creating: v.Creating, CreateInput: v.createInput}.StartCreate()
	v.Creating = view.Creating
	v.createInput = view.CreateInput
	return v
}

// CancelCreate dismisses the inline create input without persisting.
func (v PlaylistsView) CancelCreate() PlaylistsView {
	view := View{Creating: v.Creating, CreateInput: v.createInput}.CancelCreate()
	v.Creating = view.Creating
	v.createInput = view.CreateInput
	return v
}

// UpdateCreateInput forwards a key message to the Bubbles textinput model.
func (v PlaylistsView) UpdateCreateInput(msg tea.KeyPressMsg) (PlaylistsView, tea.Cmd) {
	var cmd tea.Cmd
	v.createInput, cmd = v.createInput.Update(msg)
	return v, cmd
}

// UpdateCreateInputMsg forwards a generic tea.Msg to the textinput, enabling
// paste and other multi-step commands.
func (v PlaylistsView) UpdateCreateInputMsg(msg tea.Msg) (PlaylistsView, tea.Cmd) {
	var cmd tea.Cmd
	v.createInput, cmd = v.createInput.Update(msg)
	return v, cmd
}

// CreateInputValue returns the current create input buffer, trimmed of
// leading/trailing whitespace to prevent names like "  hello  ".
func (v PlaylistsView) CreateInputValue() string {
	return strings.TrimSpace(v.createInput.Value())
}

// CreateInputIsEmpty reports whether the trimmed create input buffer is empty.
func (v PlaylistsView) CreateInputIsEmpty() bool {
	return strings.TrimSpace(v.createInput.Value()) == ""
}

// IsRenaming reports whether the inline rename input is active.
func (v PlaylistsView) IsRenaming() bool {
	return v.Renaming
}

// StartRename activates the inline rename input pre-populated with currentName.
func (v PlaylistsView) StartRename(currentName string) PlaylistsView {
	view := View{Renaming: v.Renaming, RenameInput: v.RenameInput}.StartRename(currentName)
	v.Renaming = view.Renaming
	v.RenameInput = view.RenameInput
	return v
}

// CancelRename dismisses the inline rename input without persisting.
func (v PlaylistsView) CancelRename() PlaylistsView {
	view := View{Renaming: v.Renaming, RenameInput: v.RenameInput}.CancelRename()
	v.Renaming = view.Renaming
	v.RenameInput = view.RenameInput
	return v
}

// UpdateRenameInput forwards a key message to the Bubbles textinput model.
func (v PlaylistsView) UpdateRenameInput(msg tea.KeyPressMsg) (PlaylistsView, tea.Cmd) {
	var cmd tea.Cmd
	v.RenameInput, cmd = v.RenameInput.Update(msg)
	return v, cmd
}

// UpdateRenameInputMsg forwards a generic tea.Msg to the textinput, enabling
// paste and other multi-step commands.
func (v PlaylistsView) UpdateRenameInputMsg(msg tea.Msg) (PlaylistsView, tea.Cmd) {
	var cmd tea.Cmd
	v.RenameInput, cmd = v.RenameInput.Update(msg)
	return v, cmd
}

// RenameInputValue returns the current rename input buffer, trimmed of
// leading/trailing whitespace to prevent names like "  hello  ".
func (v PlaylistsView) RenameInputValue() string {
	return strings.TrimSpace(v.RenameInput.Value())
}

// RenameInputIsEmpty reports whether the trimmed rename input buffer is empty.
func (v PlaylistsView) RenameInputIsEmpty() bool {
	return strings.TrimSpace(v.RenameInput.Value()) == ""
}
