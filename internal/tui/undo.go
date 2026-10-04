package tui

// undoAction is the interface implemented by every reversible operation.
// description() returns the "press u to undo" status-bar notification shown
// when the action is created. perform() executes the actual undo logic;
// a returned error preserves the slot for retry within its original window.
type undoAction interface {
	kind() undoKind
	perform(m Model) (Model, error)
	description() string
}
