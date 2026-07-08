package tui

import (
	"fmt"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
)

// namedKeyCodes maps scenario key names to Bubble Tea special key codes. The
// names match the strings produced by tea.KeyPressMsg.String() so the parsed
// message routes through the same registry the real TUI uses. A few ergonomic
// aliases ("return", "escape") map onto their canonical codes.
var namedKeyCodes = map[string]rune{
	"enter":     tea.KeyEnter,
	"return":    tea.KeyEnter,
	"esc":       tea.KeyEscape,
	"escape":    tea.KeyEscape,
	"tab":       tea.KeyTab,
	"space":     tea.KeySpace,
	"backspace": tea.KeyBackspace,
	"up":        tea.KeyUp,
	"down":      tea.KeyDown,
	"left":      tea.KeyLeft,
	"right":     tea.KeyRight,
	"home":      tea.KeyHome,
	"end":       tea.KeyEnd,
	"pgup":      tea.KeyPgUp,
	"pgdown":    tea.KeyPgDown,
	"delete":    tea.KeyDelete,
	"insert":    tea.KeyInsert,
}

var modifierNames = map[string]tea.KeyMod{
	"ctrl":  tea.ModCtrl,
	"alt":   tea.ModAlt,
	"shift": tea.ModShift,
}

// ParseKey converts a scenario key name into a tea.KeyPressMsg equivalent to
// what Bubble Tea would deliver for that key. Supported forms:
//
//   - printable single runes ("a", "F", "/", "?", "=", "+")
//   - named special keys ("enter"/"return", "esc"/"escape", "tab", "space",
//     "backspace", arrows, "home"/"end"/"pgup"/"pgdown"/"delete"/"insert")
//   - modifier combinations using "ctrl+", "alt+", and "shift+" prefixes
//     (e.g. "ctrl+c", "shift+tab", "shift+left")
//
// The resulting message's String() matches the key strings used by the
// application's key registry.
func ParseKey(name string) (tea.KeyPressMsg, error) {
	if name == "" {
		return tea.KeyPressMsg{}, fmt.Errorf("key name is empty")
	}

	// A single rune is always a literal printable key, even when it is a
	// separator character like "+" that would otherwise confuse the split.
	if utf8.RuneCountInString(name) == 1 {
		r, _ := utf8.DecodeRuneInString(name)
		return printableKey(r, 0), nil
	}

	segments := strings.Split(name, "+")
	base := segments[len(segments)-1]
	var mod tea.KeyMod
	for _, seg := range segments[:len(segments)-1] {
		m, ok := modifierNames[strings.ToLower(seg)]
		if !ok {
			return tea.KeyPressMsg{}, fmt.Errorf("unknown modifier %q in key %q", seg, name)
		}
		mod |= m
	}

	if base == "" {
		return tea.KeyPressMsg{}, fmt.Errorf("key %q has no base key", name)
	}

	if code, ok := namedKeyCodes[strings.ToLower(base)]; ok {
		// Space is a printable key: an unmodified space carries its Text so it
		// routes as "space" and forwards a literal " " to text inputs.
		if code == tea.KeySpace && mod == 0 {
			return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}, nil
		}
		return tea.KeyPressMsg{Code: code, Mod: mod}, nil
	}

	if utf8.RuneCountInString(base) == 1 {
		r, _ := utf8.DecodeRuneInString(base)
		return printableKey(r, mod), nil
	}

	return tea.KeyPressMsg{}, fmt.Errorf("unrecognized key %q", name)
}

// printableKey builds a key press for a single printable rune. Without
// modifiers, Text is populated so String() echoes the rune (mirroring a real
// key press); with modifiers, Text is left empty so String() renders the
// modifier prefix via Keystroke(). Space always carries its Text so it routes
// as "space".
func printableKey(r rune, mod tea.KeyMod) tea.KeyPressMsg {
	if r == tea.KeySpace && mod == 0 {
		return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	}
	if mod == 0 {
		return tea.KeyPressMsg{Code: r, Text: string(r)}
	}
	return tea.KeyPressMsg{Code: r, Mod: mod}
}
