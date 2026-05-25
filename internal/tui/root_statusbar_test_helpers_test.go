package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func updateModel(m Model, msg tea.Msg) (Model, tea.Cmd) {
	next, cmd := m.Update(msg)
	return next.(Model), cmd
}

func viewContent(m Model) string {
	return m.View().Content
}

func renderedStatusBar(m Model) string {
	return ansi.Strip(m.renderStatusBar())
}

func assertViewContains(t *testing.T, m Model, want string) {
	t.Helper()
	content := viewContent(m)
	if !strings.Contains(content, want) {
		t.Fatalf("expected %q in view:\n%s", want, content)
	}
}

func assertViewNotContains(t *testing.T, m Model, unwanted string) {
	t.Helper()
	content := viewContent(m)
	if strings.Contains(content, unwanted) {
		t.Fatalf("did not expect %q in view:\n%s", unwanted, content)
	}
}
