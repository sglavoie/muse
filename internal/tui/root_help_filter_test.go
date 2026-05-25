package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestQuestionMarkTogglesHelpFiltered(t *testing.T) {
	m := New(context.Background())

	// Open help — defaults to filtered view.
	next, _ := m.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	nm := next.(Model)
	if !nm.helpFiltered {
		t.Fatal("helpFiltered should be true when help first opens")
	}

	// Press ? again to toggle to unfiltered (all sections).
	next, _ = nm.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	nm = next.(Model)
	if nm.helpFiltered {
		t.Fatal("helpFiltered should be false after first ? toggle")
	}

	// Press ? again to toggle back to filtered.
	next, _ = nm.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	nm = next.(Model)
	if !nm.helpFiltered {
		t.Fatal("helpFiltered should be true after second ? toggle")
	}
}

func TestHelpFilteredShowsViewSections(t *testing.T) {
	m := New(context.Background())
	m.termWidth = 80
	m.termHeight = 100

	// Switch to Queue.
	next, _ := m.Update(tea.KeyPressMsg{Code: '3', Text: "3"})
	nm := next.(Model)

	// Open help — defaults to filtered view.
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

	// Open help — defaults to filtered view.
	next, _ := m.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	nm := next.(Model)
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
