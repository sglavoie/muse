package tui

import (
	"strings"
)

// undoAction is the interface implemented by every reversible operation.
// description() returns the "press u to undo" status-bar notification shown
// when the action is created. perform() executes the actual undo logic;
// a returned error preserves the slot for retry.
type undoAction interface {
	kind() undoKind
	perform(m Model) (Model, error)
	description() string
}

func (m Model) canUndo() bool {
	return m.undo.slot.action != nil
}

func (m Model) undoDetail() string {
	description := m.undo.slot.action.description()
	for _, suffix := range []string{" — press u to undo", " · u to undo"} {
		description = strings.TrimSuffix(description, suffix)
	}
	return "Revert: " + description
}
