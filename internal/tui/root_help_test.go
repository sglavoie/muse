package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestUpdateQuestionMarkSetsHelpVisible(t *testing.T) {
	m := New(context.Background())
	// Default active view is Library.
	if m.activeView != viewLibrary {
		t.Fatalf("precondition: expected viewLibrary, got %d", m.activeView)
	}

	next, _ := m.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	nm := next.(Model)

	if !nm.helpVisible {
		t.Fatal("'?' key: expected helpVisible==true")
	}
	if nm.prevActiveView != viewLibrary {
		t.Fatalf("'?' key: expected prevActiveView==viewLibrary (%d), got %d", viewLibrary, nm.prevActiveView)
	}
}

func TestViewShowsHelpOverlayWhenVisible(t *testing.T) {
	m := New(context.Background())
	m.termWidth = 80
	m.termHeight = 100
	next, _ := m.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	nm := next.(Model)

	v := nm.View()
	if !strings.Contains(ansi.Strip(v.Content), "Keybindings") {
		t.Fatalf("View() with helpVisible=true: expected overlay content containing \"Keybindings\", got:\n%s", v.Content)
	}
}

func TestUpdateEscClearsHelpVisibleAndRestoresView(t *testing.T) {
	m := New(context.Background())

	// Switch to Playlists first.
	next, _ := m.Update(tea.KeyPressMsg{Code: '2', Text: "2"})
	nm := next.(Model)
	if nm.activeView != viewPlaylists {
		t.Fatalf("precondition: expected viewPlaylists, got %d", nm.activeView)
	}

	// Open help overlay.
	next, _ = nm.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	nm = next.(Model)
	if !nm.helpVisible {
		t.Fatal("precondition: expected helpVisible==true after '?'")
	}

	// Esc should clear the overlay and restore Playlists.
	next, _ = nm.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	nm = next.(Model)

	if nm.helpVisible {
		t.Fatal("Esc while helpVisible: expected helpVisible==false")
	}
	if nm.activeView != viewPlaylists {
		t.Fatalf("Esc while helpVisible: expected activeView==viewPlaylists (%d), got %d", viewPlaylists, nm.activeView)
	}
}

func TestUpdateEscWhenHelpNotVisibleDoesNotQuit(t *testing.T) {
	m := New(context.Background())
	// helpVisible defaults to false — Esc must not quit and must not error.
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if cmd != nil {
		t.Fatal("Esc with helpVisible=false must not produce a command (e.g. quit)")
	}
}

// TestViewHelpOverlayContainsSectionHeaders verifies that when the help overlay
// is visible, View() renders all expected section headers.
func TestViewHelpOverlayContainsSectionHeaders(t *testing.T) {
	m := New(context.Background())
	m.termWidth = 80
	m.termHeight = 100
	next, _ := m.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	nm := next.(Model)

	v := nm.View()
	for _, header := range []string{"Global", "Library", "Playlists", "Queue", "Modals"} {
		if !strings.Contains(v.Content, header) {
			t.Errorf("View() with helpVisible=true: missing section header %q", header)
		}
	}
}

// TestEscRestoringViewDoesNotAlterPlaybackOrScanState verifies that dismissing
// the help overlay with Esc does not change playbackState, repeatMode, shuffle,
// or the scanning flag.
func TestEscRestoringViewDoesNotAlterPlaybackOrScanState(t *testing.T) {
	m := New(context.Background())
	m.playbackState = PlaybackPlaying
	m.repeatMode = RepeatAll
	m.shuffle = true
	m.scanning = true

	// Open the overlay.
	next, _ := m.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	nm := next.(Model)

	// Dismiss with Esc.
	next, _ = nm.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	nm = next.(Model)

	if nm.playbackState != PlaybackPlaying {
		t.Fatalf("Esc: playbackState changed; expected PlaybackPlaying, got %v", nm.playbackState)
	}
	if nm.repeatMode != RepeatAll {
		t.Fatalf("Esc: repeatMode changed; expected RepeatAll, got %v", nm.repeatMode)
	}
	if !nm.shuffle {
		t.Fatal("Esc: shuffle changed; expected true, got false")
	}
	if !nm.scanning {
		t.Fatal("Esc: scanning changed; expected true, got false")
	}
}

func TestQuestionMarkTogglesHelpFiltered(t *testing.T) {
	m := New(context.Background())

	// Open help.
	next, _ := m.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	nm := next.(Model)
	if nm.helpFiltered {
		t.Fatal("helpFiltered should be false when help first opens")
	}

	// Press ? again to toggle to filtered.
	next, _ = nm.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	nm = next.(Model)
	if !nm.helpFiltered {
		t.Fatal("helpFiltered should be true after first ? toggle")
	}

	// Press ? again to toggle back.
	next, _ = nm.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	nm = next.(Model)
	if nm.helpFiltered {
		t.Fatal("helpFiltered should be false after second ? toggle")
	}
}

func TestHelpFilteredShowsViewSections(t *testing.T) {
	m := New(context.Background())
	m.termWidth = 80
	m.termHeight = 100

	// Switch to Queue.
	next, _ := m.Update(tea.KeyPressMsg{Code: '3', Text: "3"})
	nm := next.(Model)

	// Open help.
	next, _ = nm.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	nm = next.(Model)

	// Toggle to filtered.
	next, _ = nm.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	nm = next.(Model)

	v := nm.View()
	stripped := ansi.Strip(v.Content)
	// The viewport pads lines to its width, so trim trailing spaces per line.
	var trimmedLines []string
	for _, line := range strings.Split(stripped, "\n") {
		trimmedLines = append(trimmedLines, strings.TrimRight(line, " "))
	}
	stripped = strings.Join(trimmedLines, "\n")
	if !strings.Contains(stripped, "\nQueue\n") {
		t.Error("filtered help for Queue view should contain Queue section header")
	}
	if strings.Contains(stripped, "\nLibrary\n") {
		t.Error("filtered help for Queue view should not contain Library section header")
	}
	if strings.Contains(stripped, "\nPlaylists\n") {
		t.Error("filtered help for Queue view should not contain Playlists section header")
	}
}

func TestEscResetsHelpFiltered(t *testing.T) {
	m := New(context.Background())

	// Open help and toggle filter on.
	next, _ := m.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	nm := next.(Model)
	next, _ = nm.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	nm = next.(Model)
	if !nm.helpFiltered {
		t.Fatal("precondition: helpFiltered should be true")
	}

	// Esc should reset it.
	next, _ = nm.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	nm = next.(Model)
	if nm.helpFiltered {
		t.Fatal("helpFiltered should be false after Esc")
	}
}
