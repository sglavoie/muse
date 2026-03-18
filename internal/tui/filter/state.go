package filter

// State holds the filter bar's active/text state. All methods use value
// receivers and return modified copies so callers can write
// v.filter = v.filter.Activate() without pointer aliasing concerns.
type State struct {
	Active bool
	Text   string
}

// Activate enters filter input mode, preserving any existing text.
func (s State) Activate() State {
	s.Active = true
	return s
}

// Commit exits input mode while keeping the current text visible.
func (s State) Commit() State {
	s.Active = false
	return s
}

// Deactivate exits input mode and clears the text.
func (s State) Deactivate() State {
	s.Active = false
	s.Text = ""
	return s
}

// AppendChar appends ch to the text. Returns the new state and true if the
// text changed. It is a no-op when the filter is not active.
func (s State) AppendChar(ch rune) (State, bool) {
	if !s.Active {
		return s, false
	}
	s.Text += string(ch)
	return s, true
}

// Backspace removes the last rune from the text. Returns the new state and
// true if the text changed. It is a no-op when the filter is not active or
// the text is already empty.
func (s State) Backspace() (State, bool) {
	if !s.Active || len(s.Text) == 0 {
		return s, false
	}
	runes := []rune(s.Text)
	s.Text = string(runes[:len(runes)-1])
	return s, true
}
