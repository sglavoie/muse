package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestParseKeyStringRoundTrip(t *testing.T) {
	// Each parsed key must stringify to the same name the app registry uses so
	// scenario keys route through the real key bindings.
	cases := map[string]string{
		"a":           "a",
		"F":           "F",
		"/":           "/",
		"?":           "?",
		"=":           "=",
		"-":           "-",
		"+":           "+",
		"1":           "1",
		"enter":       "enter",
		"return":      "enter",
		"esc":         "esc",
		"escape":      "esc",
		"tab":         "tab",
		"space":       "space",
		"backspace":   "backspace",
		"up":          "up",
		"down":        "down",
		"left":        "left",
		"right":       "right",
		"ctrl+c":      "ctrl+c",
		"ctrl+,":      "ctrl+,",
		"ctrl+r":      "ctrl+r",
		"shift+tab":   "shift+tab",
		"shift+left":  "shift+left",
		"shift+enter": "shift+enter",
	}
	for input, want := range cases {
		msg, err := ParseKey(input)
		if err != nil {
			t.Fatalf("ParseKey(%q) unexpected error: %v", input, err)
		}
		if got := msg.String(); got != want {
			t.Errorf("ParseKey(%q).String() = %q, want %q", input, got, want)
		}
	}
}

func TestParseKeySpaceCarriesText(t *testing.T) {
	msg, err := ParseKey("space")
	if err != nil {
		t.Fatalf("ParseKey(space) error: %v", err)
	}
	if msg.Text != " " {
		t.Errorf("space Text = %q, want %q", msg.Text, " ")
	}
}

func TestParseKeyPrintableCarriesText(t *testing.T) {
	msg, err := ParseKey("j")
	if err != nil {
		t.Fatalf("ParseKey(j) error: %v", err)
	}
	if msg.Text != "j" || msg.Code != 'j' {
		t.Errorf("ParseKey(j) = %+v, want Code 'j' Text \"j\"", msg)
	}
}

func TestParseKeyModifiedHasNoText(t *testing.T) {
	// Modified printable keys must leave Text empty so String() renders the
	// modifier prefix instead of echoing the bare rune.
	msg, err := ParseKey("ctrl+c")
	if err != nil {
		t.Fatalf("ParseKey(ctrl+c) error: %v", err)
	}
	if msg.Text != "" {
		t.Errorf("ctrl+c Text = %q, want empty", msg.Text)
	}
	if msg.Mod&tea.ModCtrl == 0 {
		t.Errorf("ctrl+c missing ModCtrl: %v", msg.Mod)
	}
}

func TestParseKeyErrors(t *testing.T) {
	for _, input := range []string{"", "ctrl+", "meta+a", "notakey", "hyper+shift+z"} {
		if _, err := ParseKey(input); err == nil {
			t.Errorf("ParseKey(%q) expected error, got nil", input)
		}
	}
}
