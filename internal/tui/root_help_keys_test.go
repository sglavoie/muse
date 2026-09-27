package tui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/tui/keys"
)

// TestHelpOverlayJumpsToTopAndBottom guards that g / G and home / end jump
// the help overlay to its first and last line, as they do in every list view,
// instead of falling through to the viewport's default keymap, which ignores
// them.
func TestHelpOverlayJumpsToTopAndBottom(t *testing.T) {
	m := New(context.Background())
	m.termWidth = 100
	m.termHeight = 20
	next, _ := m.handleKeyAction(keys.ActionHelp)
	m = next.(Model)
	if m.helpViewport.AtBottom() {
		t.Fatal("precondition: help must be taller than the viewport")
	}

	press := func(m Model, msg tea.KeyPressMsg) Model {
		t.Helper()
		m, _, handled := m.handleHelpKeyPress(msg)
		if !handled {
			t.Fatalf("help overlay did not consume %q", msg.String())
		}
		return m
	}
	bottom := []tea.KeyPressMsg{{Code: 'G', Text: "G"}, {Code: tea.KeyEnd}}
	top := []tea.KeyPressMsg{{Code: 'g', Text: "g"}, {Code: tea.KeyHome}}
	for i := range bottom {
		m = press(m, bottom[i])
		if !m.helpViewport.AtBottom() {
			t.Errorf("%q should jump to the bottom of help", bottom[i].String())
		}
		m = press(m, top[i])
		if !m.helpViewport.AtTop() {
			t.Errorf("%q should jump to the top of help", top[i].String())
		}
	}
}
