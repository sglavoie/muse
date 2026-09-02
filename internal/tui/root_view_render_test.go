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

// TestTitleBarTagsBadgeShowsZero verifies that the Tags view (10th view) shows
// badge "0" matching its actual keybinding, not "10".
func TestTitleBarTagsBadgeShowsZero(t *testing.T) {
	m := New(context.Background())
	s := styles.NewStyles(true)
	m.styles = &s
	m.termWidth = 200 // wide enough for the full layout
	m.ActiveView = viewTags

	out := m.renderTitleBar()
	// The Tags badge must show "0", not "10".
	if strings.Contains(out, "10") {
		t.Errorf("title bar should show badge '0' for Tags, not '10': got %q", out)
	}
}
