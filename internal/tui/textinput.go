package tui

// textInput is a minimal single-line text editor for inline preset naming.
type textInput struct {
	buf    []rune
	cursor int
}

// HandleKey processes a key string and updates the text input state.
func (t *textInput) HandleKey(key string) {
	switch key {
	case "left":
		if t.cursor > 0 {
			t.cursor--
		}
	case "right":
		if t.cursor < len(t.buf) {
			t.cursor++
		}
	case "backspace", "ctrl+h":
		if t.cursor > 0 {
			tail := make([]rune, len(t.buf[t.cursor:]))
			copy(tail, t.buf[t.cursor:])
			t.buf = append(t.buf[:t.cursor-1], tail...)
			t.cursor--
		}
	case "delete":
		if t.cursor < len(t.buf) {
			tail := make([]rune, len(t.buf[t.cursor+1:]))
			copy(tail, t.buf[t.cursor+1:])
			t.buf = append(t.buf[:t.cursor], tail...)
		}
	case "space":
		// bubbletea v2 reports the spacebar as "space" rather than " ".
		t.insertRune(' ')
	default:
		runes := []rune(key)
		if len(runes) == 1 && runes[0] >= 32 {
			t.insertRune(runes[0])
		}
	}
}

// insertRune inserts r at the current cursor position and advances the cursor.
func (t *textInput) insertRune(r rune) {
	tail := make([]rune, len(t.buf[t.cursor:]))
	copy(tail, t.buf[t.cursor:])
	t.buf = append(t.buf[:t.cursor], append([]rune{r}, tail...)...)
	t.cursor++
}

// Value returns the current text content.
func (t *textInput) Value() string {
	return string(t.buf)
}

// Render returns a display string showing the text with a cursor indicator.
func (t *textInput) Render(_ int) string {
	before := string(t.buf[:t.cursor])
	after := string(t.buf[t.cursor:])
	return "[" + before + "|" + after + "]"
}
