package tui

import (
	tea "charm.land/bubbletea/v2"
)

// undoAction is the interface implemented by every reversible operation.
// description() returns the "press u to undo" status-bar notification shown
// when the action is created. perform() executes the actual undo logic.
type undoAction interface {
	kind() undoKind
	perform(m Model) (Model, tea.Cmd)
	description() string
}
