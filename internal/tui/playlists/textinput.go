package playlists

import "strings"

// TextInput is a minimal single-line text input component. It holds a rune
// buffer and renders as a plain string with a block cursor appended at the
// end, suitable for embedding as an in-place row replacement.
type TextInput struct {
	buf []rune
}

// NewTextInput returns a TextInput pre-populated with the given initial value.
func NewTextInput(initial string) TextInput {
	return TextInput{buf: []rune(initial)}
}

// AppendRune appends a single rune to the buffer.
func (t TextInput) AppendRune(r rune) TextInput {
	t.buf = append(append([]rune(nil), t.buf...), r)
	return t
}

// Backspace removes the last rune from the buffer. It is a no-op on an empty
// buffer.
func (t TextInput) Backspace() TextInput {
	if len(t.buf) == 0 {
		return t
	}
	t.buf = append([]rune(nil), t.buf[:len(t.buf)-1]...)
	return t
}

// Value returns the current buffer contents as a string.
func (t TextInput) Value() string {
	return string(t.buf)
}

// IsEmpty reports whether the trimmed value is empty.
func (t TextInput) IsEmpty() bool {
	return strings.TrimSpace(t.Value()) == ""
}

// Render returns the display string for the component: the buffer contents
// followed by a block cursor character. The caller is responsible for any
// surrounding highlight or padding to fit within a list row.
func (t TextInput) Render() string {
	return t.Value() + "█"
}
