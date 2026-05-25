package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestHelpSearchFilterActivatesOnSlash(t *testing.T) {
	m := New(context.Background())
	// Open help.
	next, _ := m.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	nm := next.(Model)

	// Press / to activate search filter.
	next, _ = nm.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	nm = next.(Model)

	if !nm.helpSearchFilter.Active {
		t.Fatal("expected helpSearchFilter.Active==true after pressing /")
	}
}

func TestHelpSearchFilterTypingFiltersEntries(t *testing.T) {
	m := New(context.Background())
	m.termWidth = 80
	m.termHeight = 100

	// Open help, activate filter, type "queue".
	next, _ := m.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	nm := next.(Model)
	next, _ = nm.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	nm = next.(Model)
	for _, ch := range "queue" {
		next, _ = nm.Update(tea.KeyPressMsg{Code: ch, Text: string(ch)})
		nm = next.(Model)
	}

	v := nm.View()
	content := v.Content
	if !strings.Contains(strings.ToLower(content), "queue") {
		t.Error("filtered help should contain entries matching \"queue\"")
	}
	// Library section should not appear because no Library entry matches "queue".
	if strings.Contains(content, "\nLibrary\n") {
		t.Error("filtered help should not contain Library section for query \"queue\"")
	}
}

func TestHelpSearchFilterEscDeactivates(t *testing.T) {
	m := New(context.Background())
	m.termWidth = 80
	m.termHeight = 100

	// Open help, activate filter, type "vol".
	next, _ := m.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	nm := next.(Model)
	next, _ = nm.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	nm = next.(Model)
	for _, ch := range "vol" {
		next, _ = nm.Update(tea.KeyPressMsg{Code: ch, Text: string(ch)})
		nm = next.(Model)
	}

	// Esc should deactivate and clear text.
	next, _ = nm.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	nm = next.(Model)

	if nm.helpSearchFilter.Active {
		t.Fatal("expected helpSearchFilter.Active==false after esc")
	}
	if nm.helpSearchFilter.Text != "" {
		t.Fatalf("expected helpSearchFilter.Text==\"\", got %q", nm.helpSearchFilter.Text)
	}
	// Help should still be visible with all sections.
	if !nm.helpVisible {
		t.Fatal("help should still be visible after esc from filter input")
	}
}

func TestHelpSearchFilterEnterCommits(t *testing.T) {
	m := New(context.Background())
	m.termWidth = 80
	m.termHeight = 100

	// Open help, activate filter, type "play", press enter.
	next, _ := m.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	nm := next.(Model)
	next, _ = nm.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	nm = next.(Model)
	for _, ch := range "play" {
		next, _ = nm.Update(tea.KeyPressMsg{Code: ch, Text: string(ch)})
		nm = next.(Model)
	}
	next, _ = nm.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	nm = next.(Model)

	if nm.helpSearchFilter.Active {
		t.Fatal("expected helpSearchFilter.Active==false after enter (committed)")
	}
	if nm.helpSearchFilter.Text != "play" {
		t.Fatalf("expected helpSearchFilter.Text==\"play\", got %q", nm.helpSearchFilter.Text)
	}
	// Filtered content should still show.
	v := nm.View()
	if !strings.Contains(strings.ToLower(v.Content), "play") {
		t.Error("committed filter should still show filtered content")
	}
}

func TestHelpSearchFilterEscWithCommittedTextClearsThenCloses(t *testing.T) {
	m := New(context.Background())
	m.termWidth = 80
	m.termHeight = 100

	// Open help, filter "vol", commit with enter.
	next, _ := m.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	nm := next.(Model)
	next, _ = nm.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	nm = next.(Model)
	for _, ch := range "vol" {
		next, _ = nm.Update(tea.KeyPressMsg{Code: ch, Text: string(ch)})
		nm = next.(Model)
	}
	next, _ = nm.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	nm = next.(Model)

	// First esc: clears committed text, help stays open.
	next, _ = nm.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	nm = next.(Model)
	if !nm.helpVisible {
		t.Fatal("first esc should not close help, only clear search text")
	}
	if nm.helpSearchFilter.Text != "" {
		t.Fatalf("first esc should clear search text, got %q", nm.helpSearchFilter.Text)
	}

	// Second esc: closes help overlay entirely.
	next, _ = nm.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	nm = next.(Model)
	if nm.helpVisible {
		t.Fatal("second esc should close help overlay")
	}
}

func TestHelpSearchFilterSlashCommits(t *testing.T) {
	m := New(context.Background())
	m.termWidth = 80
	m.termHeight = 100

	// Open help, activate filter, type "seek", press / to commit.
	next, _ := m.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	nm := next.(Model)
	next, _ = nm.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	nm = next.(Model)
	for _, ch := range "seek" {
		next, _ = nm.Update(tea.KeyPressMsg{Code: ch, Text: string(ch)})
		nm = next.(Model)
	}
	// Press / again to commit.
	next, _ = nm.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	nm = next.(Model)

	if nm.helpSearchFilter.Active {
		t.Fatal("expected helpSearchFilter.Active==false after / commit")
	}
	if nm.helpSearchFilter.Text != "seek" {
		t.Fatalf("expected helpSearchFilter.Text==\"seek\", got %q", nm.helpSearchFilter.Text)
	}
}

func TestHelpSearchFilterCombinesWithViewFilter(t *testing.T) {
	m := New(context.Background())
	m.termWidth = 80
	m.termHeight = 100

	// Switch to Queue, open help — defaults to filtered view.
	next, _ := m.Update(tea.KeyPressMsg{Code: '3', Text: "3"})
	nm := next.(Model)
	next, _ = nm.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	nm = next.(Model)
	if !nm.helpFiltered {
		t.Fatal("precondition: helpFiltered should be true")
	}

	// Activate search and type "remove".
	next, _ = nm.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	nm = next.(Model)
	for _, ch := range "remove" {
		next, _ = nm.Update(tea.KeyPressMsg{Code: ch, Text: string(ch)})
		nm = next.(Model)
	}

	v := nm.View()
	content := strings.ToLower(v.Content)
	if !strings.Contains(content, "remove") {
		t.Error("combined filter should show entries matching \"remove\"")
	}
	// Library and Playlists should not appear (view filter excludes them,
	// and "remove" doesn't appear in their entries anyway for Queue context).
	if strings.Contains(v.Content, "\nLibrary\n") {
		t.Error("combined filter should not include Library section")
	}
}

// TestHelpSearchFilterNoMatchesShowsNoMatchesMessage verifies that when the
// help search filter matches no entries, the overlay shows "(no matches)".
func TestHelpSearchFilterNoMatchesShowsNoMatchesMessage(t *testing.T) {
	m := New(context.Background())
	m.termWidth = 80
	m.termHeight = 100

	// Open help, activate filter, type a nonsense query.
	next, _ := m.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	nm := next.(Model)
	next, _ = nm.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	nm = next.(Model)
	for _, ch := range "zzzznonexistent" {
		next, _ = nm.Update(tea.KeyPressMsg{Code: ch, Text: string(ch)})
		nm = next.(Model)
	}

	v := nm.View()
	if !strings.Contains(v.Content, "(no matches)") {
		t.Fatalf("help overlay with no matching entries should show \"(no matches)\"; got:\n%s", ansi.Strip(v.Content))
	}
}
