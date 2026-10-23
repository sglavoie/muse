package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestInlineEditorsKeepPurposeCursorAndMatchingHints(t *testing.T) {
	for _, view := range []string{"2", "6"} {
		for _, action := range []string{"c", "r"} {
			m := newRichFixture()
			m = m.handleWindowSizeMsg(tea.WindowSizeMsg{Width: 60, Height: 15})
			for _, key := range []string{view, action} {
				next, cmd := m.Update(tea.KeyPressMsg{Code: rune(key[0]), Text: key})
				m = next.(Model)
				if key == action && cmd != nil {
					next, _ = m.Update(cmd())
					m = next.(Model)
				}
			}
			screen := ansi.Strip(m.View().Content)
			purpose := "New playlist:"
			if view == "6" {
				purpose = "New tag:"
			}
			if action == "r" {
				purpose = strings.Replace(purpose, "New", "Rename", 1)
			}
			if !strings.Contains(screen, purpose) || !strings.Contains(screen, "enter save") || !strings.Contains(screen, "esc cancel") || strings.Contains(screen, "enter open") {
				t.Fatal(screen)
			}
			// Type past the right edge: the end of the draft and overflow marker must
			// stay visible; cancellation restores normal browsing hints.
			for _, r := range strings.Repeat("全角", 40) + "END" {
				next, _ := m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
				m = next.(Model)
			}
			screen = ansi.Strip(m.View().Content)
			if !strings.Contains(screen, "END") || !strings.Contains(screen, "‹") {
				t.Fatal(screen)
			}
			next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
			m = next.(Model)
			if hint := ansi.Strip(m.renderFooterHint()); !strings.Contains(hint, "enter open") {
				t.Fatal(hint)
			}
		}
	}
}

func TestLibraryHelpFirstScreenCoversEverydayActions(t *testing.T) {
	m := newRichFixture()
	m = m.handleWindowSizeMsg(tea.WindowSizeMsg{Width: 60, Height: 15})
	next, _ := m.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	m = next.(Model)
	screen := ansi.Strip(m.View().Content)
	for _, want := range []string{"Enqueue", "Play / Pause", "Filter tracks", "Select track", "Undo"} {
		if !strings.Contains(screen, want) {
			t.Fatalf("missing %q: %s", want, screen)
		}
	}
}

func TestFocusedDetailsUseShortWindowsWithoutDisplacingRows(t *testing.T) {
	for _, height := range []int{15, 23, 24} {
		m := newNarrowFixture()
		m = m.handleWindowSizeMsg(tea.WindowSizeMsg{Width: 60, Height: height})
		screen := ansi.Strip(m.View().Content)
		if !strings.Contains(screen, "Focused ·") || !strings.Contains(screen, "3 tracks") || !strings.Contains(screen, "5:21") {
			t.Fatal(screen)
		}
		for _, line := range strings.Split(screen, "\n") {
			if ansi.StringWidth(line) > 60 {
				t.Fatal(line)
			}
		}
		if strings.Count(screen, "\n")+1 > height {
			t.Fatalf("exceeded height %d", height)
		}
		m = newDenseFixture()
		m = m.handleWindowSizeMsg(tea.WindowSizeMsg{Width: 60, Height: height})
		screen = ansi.Strip(m.View().Content)
		if strings.Contains(screen, "Focused ·") {
			t.Fatalf("detail stole space from crowded table: %s", screen)
		}
	}
}
