package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestRecentMessagesPaletteAndModal(t *testing.T) {
	m := newRichFixture()
	m = m.handleWindowSizeMsg(tea.WindowSizeMsg{Width: 60, Height: 15})
	m.statusBar.Notify("Export saved to /a/very/long/path/" + strings.Repeat("音楽", 45))
	var cmd tea.Cmd
	for _, entry := range m.BuildPaletteCommands() {
		if entry.ID == "messages.show" {
			cmd = entry.Exec()
		}
	}
	if cmd == nil {
		t.Fatal("missing recent messages command")
	}
	next, _ := m.Update(cmd())
	m = next.(Model)
	if !m.hasActiveOverlay() || m.ActiveOverlayName() != "recent_messages" {
		t.Fatal("history is not treated as an overlay")
	}
	screen := ansi.Strip(m.View().Content)
	if !strings.Contains(screen, "Recent messages") || !strings.Contains(screen, "Export saved") {
		t.Fatalf("missing history: %s", screen)
	}
	for _, line := range strings.Split(screen, "\n") {
		if ansi.StringWidth(line) > 60 {
			t.Fatalf("overwide line: %q", line)
		}
	}
	cursor := m.library.Cursor
	m = pressKey(m, "j")
	if m.library.Cursor != cursor {
		t.Fatal("history navigation leaked into library")
	}
	m = pressKey(m, "y") // copy keeps the overlay open; pressKey does not execute the clipboard command
	if m.overlay.messages == nil {
		t.Fatal("history unexpectedly closed")
	}
	m = pressEsc(m)
	if m.hasActiveOverlay() {
		t.Fatal("escape did not dismiss history")
	}
}
