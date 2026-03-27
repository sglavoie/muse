package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// pressCtrlG sends ctrl+g to a Model and returns the updated Model.
func pressCtrlG(t *testing.T, m Model) Model {
	t.Helper()
	next, _ := m.Update(tea.KeyPressMsg{Code: 'g', Mod: tea.ModCtrl})
	return next.(Model)
}

// TestDebugToggle verifies that ctrl+g opens the debug panel and esc closes it.
func TestDebugToggle(t *testing.T) {
	m := New(context.Background())

	// Open debug panel.
	nm := pressCtrlG(t, m)
	if !nm.debugVisible {
		t.Fatal("ctrl+g: expected debugVisible==true")
	}

	// Esc should close it.
	next, _ := nm.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	nm = next.(Model)
	if nm.debugVisible {
		t.Fatal("esc: expected debugVisible==false")
	}
}

// TestDebugRestoresView verifies that opening the debug panel from the
// Playlists view and pressing esc returns to Playlists.
func TestDebugRestoresView(t *testing.T) {
	m := New(context.Background())

	// Switch to Playlists.
	next, _ := m.Update(tea.KeyPressMsg{Code: '2', Text: "2"})
	nm := next.(Model)
	if nm.activeView != viewPlaylists {
		t.Fatalf("precondition: expected viewPlaylists, got %d", nm.activeView)
	}

	// Open debug panel.
	nm = pressCtrlG(t, nm)
	if !nm.debugVisible {
		t.Fatal("precondition: expected debugVisible==true after ctrl+g")
	}
	if nm.debugPrevView != viewPlaylists {
		t.Fatalf("expected debugPrevView==viewPlaylists (%d), got %d", viewPlaylists, nm.debugPrevView)
	}

	// Esc should restore Playlists.
	next, _ = nm.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	nm = next.(Model)
	if nm.debugVisible {
		t.Fatal("esc: expected debugVisible==false")
	}
	if nm.activeView != viewPlaylists {
		t.Fatalf("esc: expected activeView==viewPlaylists (%d), got %d", viewPlaylists, nm.activeView)
	}
}

// TestDebugViewContent verifies that View() contains all expected section
// headers when the debug panel is visible.
func TestDebugViewContent(t *testing.T) {
	m := New(context.Background())
	m.termWidth = 80
	m.termHeight = 40

	nm := pressCtrlG(t, m)

	v := nm.View()
	content := ansi.Strip(v.Content)
	for _, header := range []string{"Player", "Playback", "Library", "Runtime", "TUI"} {
		if !strings.Contains(content, header) {
			t.Errorf("debug panel View(): missing section header %q", header)
		}
	}
}

// TestDebugPlaybackStopped verifies that the debug panel displays "stopped"
// for the PlaybackStopped state instead of falling through to "idle".
func TestDebugPlaybackStopped(t *testing.T) {
	m := New(context.Background())
	m.playback.state = PlaybackStopped
	p := collectDebugInfo(m)
	if p.playbackState != "stopped" {
		t.Errorf("playbackState = %q; want %q", p.playbackState, "stopped")
	}
}

// TestDebugNotOpenWhenOverlayActive verifies that ctrl+g is a no-op when
// another overlay (settings) is already active.
func TestDebugNotOpenWhenOverlayActive(t *testing.T) {
	m := New(context.Background())

	// Open settings overlay.
	next, _ := m.Update(tea.KeyPressMsg{Code: ',', Mod: tea.ModCtrl})
	nm := next.(Model)
	if nm.overlay.settings == nil {
		t.Fatal("precondition: expected settings overlay to be open")
	}

	// Press ctrl+g — debug panel must not open.
	nm2 := pressCtrlG(t, nm)
	if nm2.debugVisible {
		t.Fatal("ctrl+g while settings overlay is open: expected debugVisible==false")
	}
}
