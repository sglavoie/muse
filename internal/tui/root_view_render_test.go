package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/styles"
)

func TestViewReturnsTeaView(t *testing.T) {
	m := New(context.Background())
	v := m.View()
	// Compile-time check: View() returns tea.View (not string).
	_ = v.Content
}

// TestModelRepeatModeDefaultOff verifies that repeat defaults to RepeatOff and
// shuffle defaults to false on a freshly created model.
func TestModelRepeatModeDefaultOff(t *testing.T) {
	m := New(context.Background())
	if m.playback.repeatMode != playback.RepeatOff {
		t.Fatalf("repeatMode must default to RepeatOff, got %v", m.playback.repeatMode)
	}
	if m.playback.shuffle {
		t.Fatal("shuffle must default to false")
	}
}

// The Tags badge must match its actual keybinding.
func TestTitleBarTagsBadge(t *testing.T) {
	m := New(context.Background())
	s := styles.NewStyles(true)
	m.styles = &s
	m.termWidth = 200
	m.ActiveView = viewTags
	if out := m.renderTitleBar(); !strings.Contains(out, "[6 Tags]") {
		t.Fatalf("incorrect Tags shortcut: %s", out)
	}
}
