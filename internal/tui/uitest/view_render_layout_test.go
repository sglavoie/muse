package uitest

// These four tests were carved out of internal/tui/root_view_render_test.go
// when uitest_screen.go moved here. They assert the too-small-terminal path and
// the minimum supported size through PlainScreen and DetectLayoutIssues, which
// package tui can no longer reach without an import cycle. They drive the model
// entirely through tui's exported API.

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/tui"
)

func TestViewRendersTerminalTooSmallMessageBelowMinimumWidth(t *testing.T) {
	m := tui.New(context.Background())
	next, _ := m.Update(tea.WindowSizeMsg{Width: tui.MinTerminalWidth - 1, Height: tui.MinTerminalHeight})
	m = next.(tui.Model)

	content := PlainScreen(m.View().Content)
	if !strings.Contains(content, "Terminal window is too small") {
		t.Fatalf("expected too-small message, got:\n%s", content)
	}
	if strings.Contains(content, "Library") {
		t.Fatalf("too-small view should not render normal content:\n%s", content)
	}
}

func TestViewRendersTerminalTooSmallMessageBelowMinimumHeight(t *testing.T) {
	m := tui.New(context.Background())
	next, _ := m.Update(tea.WindowSizeMsg{Width: tui.MinTerminalWidth, Height: tui.MinTerminalHeight - 1})
	m = next.(tui.Model)

	content := PlainScreen(m.View().Content)
	if !strings.Contains(content, "Terminal window is too small") {
		t.Fatalf("expected too-small message, got:\n%s", content)
	}
	if strings.Contains(content, "Library") {
		t.Fatalf("too-small view should not render normal content:\n%s", content)
	}
}

func TestViewRendersNormalContentAtMinimumSize(t *testing.T) {
	m := tui.New(context.Background())
	next, _ := m.Update(tea.WindowSizeMsg{Width: tui.MinTerminalWidth, Height: tui.MinTerminalHeight})
	m = next.(tui.Model)

	content := PlainScreen(m.View().Content)
	if strings.Contains(content, "Terminal window is too small") {
		t.Fatalf("minimum supported size should render normal content:\n%s", content)
	}
	if !strings.Contains(content, "Library") {
		t.Fatalf("expected normal library content at minimum size, got:\n%s", content)
	}
}

func TestTerminalTooSmallMessageFitsTinyTerminal(t *testing.T) {
	m := tui.New(context.Background())
	next, _ := m.Update(tea.WindowSizeMsg{Width: 20, Height: 3})
	m = next.(tui.Model)

	raw := m.View().Content
	plain := PlainScreen(raw)
	if strings.TrimSpace(plain) == "" {
		t.Fatal("too-small message should not render blank output")
	}
	if !strings.Contains(strings.ToLower(plain), "too small") {
		t.Fatalf("tiny terminal should still say it is too small, got:\n%s", plain)
	}
	if issues := DetectLayoutIssues(raw, plain, 20, 3); len(issues) != 0 {
		t.Fatalf("too-small message should fit tiny terminal, got %+v:\n%s", issues, plain)
	}
}
