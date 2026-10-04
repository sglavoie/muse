package filter

import (
	"slices"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

// State holds the filter bar's active/text state. All methods use value
// receivers and return modified copies so callers can write
// v.filter = v.filter.Activate() without pointer aliasing concerns.
//
// Text is kept as a public field for backward compatibility; it is always
// synced with the underlying textinput value after Activate/Update/Commit/Deactivate.
type State struct {
	Origin      Position // browsing position before this filter session
	Active      bool
	Text        string          // current filter text; synced with input on every Update
	input       textinput.Model // drives cursor, paste, and Bash keybindings when Active
	initialized bool            // true once Activate() has seeded the textinput
	history     []string        // oldest first; immutable across State copies
	historyBack int             // zero is the unfinished query
	draft       string
}

const historyLimit = 20

// WithHistory supplies the session's committed queries without sharing mutable slices.
func (s State) WithHistory(history []string) State {
	s.history = slices.Clone(history)
	return s
}

func (s State) History() []string { return slices.Clone(s.history) }

// New returns a properly-initialized State with an empty filter.
func New() State {
	return State{input: newInput("")}
}

// Activate enters filter input mode, preserving any existing text.
// The textinput is re-initialized so the cursor starts at the end of the
// current text. Returns a cmd to start the cursor blink animation.
func (s State) Activate() (State, tea.Cmd) {
	s.historyBack = 0
	s.input = newInput(s.Text)
	s.Text = s.input.Value()
	s.input.CursorEnd()
	s.Active = true
	s.initialized = true
	return s, s.input.Focus()
}

// Commit exits input mode while keeping the current text visible.
// When Activate() was never called (e.g. in tests constructing State directly),
// Text is preserved as-is rather than being overwritten by the zero-value input.
func (s State) Commit() State {
	if s.initialized {
		if s.input.Value() != s.Text {
			s.input = newInput(s.Text)
		}
		s.Text = s.input.Value()
		s.input.Blur()
	}
	s.Active = false
	if strings.TrimSpace(s.Text) != "" {
		history := make([]string, 0, len(s.history)+1)
		for _, query := range s.history {
			if query != s.Text {
				history = append(history, query)
			}
		}
		history = append(history, s.Text)
		s.history = history[max(0, len(history)-historyLimit):]
	}
	s.historyBack = 0
	return s
}

// Deactivate exits input mode and clears the text.
func (s State) Deactivate() State {
	s.Origin = Position{}
	s.Active = false
	s.historyBack = 0
	s.Text = ""
	s.input.SetValue("")
	s.input.Blur()
	return s
}

func newInput(value string) textinput.Model {
	ti := textinput.New()
	ti.Prompt = ""
	ti.CharLimit = 0
	ti.SetValue(value)
	return ti
}

// cloneInput gives each returned State its own text buffer; textinput.Model
// contains slices, so plain struct copies can otherwise alias hidden state.
func cloneInput(src textinput.Model) textinput.Model {
	ti := newInput(src.Value())
	ti.SetCursor(src.Position())
	if src.Focused() {
		_ = ti.Focus()
	}
	return ti
}

// seed lazily initialises the textinput from Text when the State was
// constructed directly (e.g. State{Active:true, Text:"AB"}) without going
// through Activate(), or when public Text was changed while inactive.
func (s State) seed() State {
	if s.initialized && s.input.Value() == s.Text {
		if !s.input.Focused() {
			s.input.CursorEnd()
			_ = s.input.Focus()
		}
		return s
	}
	s.input = newInput(s.Text)
	s.input.CursorEnd()
	_ = s.input.Focus() // discard cursor-blink cmd; cursor rendering is best-effort here
	s.initialized = true
	return s
}

// Update handles a key press while the filter is active, delegating to the
// embedded textinput for full paste, cursor, and Bash keybinding support.
// It is a no-op when the filter is not active.
func (s State) Update(msg tea.KeyPressMsg) (State, tea.Cmd) {
	if !s.Active {
		return s, nil
	}
	s = s.seed()
	s.input = cloneInput(s.input)
	if s.recallHistory(msg.String()) {
		return s, nil
	}
	before := s.Text
	if s.deleteWordForwardAtFinalRune(msg) {
		s.historyBack = 0
		return s, nil
	}
	var cmd tea.Cmd
	s.input, cmd = s.input.Update(msg)
	s.Text = s.input.Value()
	if s.Text != before {
		s.historyBack = 0
	}
	return s, cmd
}

// UpdateMsg is like Update but accepts a generic tea.Msg so that non-key
// messages (e.g. paste results) can reach the underlying textinput.
func (s State) UpdateMsg(msg tea.Msg) (State, tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		return s.Update(key)
	}
	if !s.Active {
		return s, nil
	}
	s = s.seed()
	s.input = cloneInput(s.input)
	before := s.Text
	var cmd tea.Cmd
	s.input, cmd = s.input.Update(msg)
	s.Text = s.input.Value()
	if s.Text != before {
		s.historyBack = 0
	}
	return s, cmd
}

func (s *State) recallHistory(key string) bool {
	if key != "up" && key != "down" {
		return false
	}
	if len(s.history) == 0 || (key == "down" && s.historyBack == 0) {
		return true
	}
	if s.historyBack == 0 {
		s.draft = s.Text
	}
	if key == "up" {
		s.historyBack = min(s.historyBack+1, len(s.history))
	} else {
		s.historyBack--
	}
	s.Text = s.draft
	if s.historyBack > 0 {
		s.Text = s.history[len(s.history)-s.historyBack]
	}
	s.input.SetValue(s.Text)
	s.input.CursorEnd()
	return true
}

func (s *State) deleteWordForwardAtFinalRune(msg tea.KeyPressMsg) bool {
	switch msg.Keystroke() {
	case "alt+d", "alt+delete":
	default:
		return false
	}
	runes := []rune(s.input.Value())
	pos := s.input.Position()
	if len(runes) == 0 || pos != len(runes)-1 {
		return false
	}
	s.input.SetValue(string(runes[:pos]))
	s.input.SetCursor(pos)
	s.Text = s.input.Value()
	return true
}

// View returns the rendered filter text.
// While active, it renders the underlying textinput (with cursor).
// While inactive, it returns Text directly so external Text updates are reflected.
func (s State) View() string {
	if !s.Active {
		return s.Text
	}
	s = s.seed()
	return s.input.View()
}
