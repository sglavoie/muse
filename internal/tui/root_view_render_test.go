package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/styles"
)

func TestViewReturnsTeaView(t *testing.T) {
	m := New(context.Background())
	v := m.View()
	// Compile-time check: View() returns tea.View (not string).
	_ = v.Content
}

func TestViewRendersTerminalTooSmallMessageBelowMinimumWidth(t *testing.T) {
	m := New(context.Background())
	next, _ := m.Update(tea.WindowSizeMsg{Width: minTerminalWidth - 1, Height: minTerminalHeight})
	m = next.(Model)

	content := PlainScreen(m.View().Content)
	if !strings.Contains(content, "Terminal window is too small") {
		t.Fatalf("expected too-small message, got:\n%s", content)
	}
	if strings.Contains(content, "Library") {
		t.Fatalf("too-small view should not render normal content:\n%s", content)
	}
}

func TestViewRendersTerminalTooSmallMessageBelowMinimumHeight(t *testing.T) {
	m := New(context.Background())
	next, _ := m.Update(tea.WindowSizeMsg{Width: minTerminalWidth, Height: minTerminalHeight - 1})
	m = next.(Model)

	content := PlainScreen(m.View().Content)
	if !strings.Contains(content, "Terminal window is too small") {
		t.Fatalf("expected too-small message, got:\n%s", content)
	}
	if strings.Contains(content, "Library") {
		t.Fatalf("too-small view should not render normal content:\n%s", content)
	}
}

func TestViewRendersNormalContentAtMinimumSize(t *testing.T) {
	m := New(context.Background())
	next, _ := m.Update(tea.WindowSizeMsg{Width: minTerminalWidth, Height: minTerminalHeight})
	m = next.(Model)

	content := PlainScreen(m.View().Content)
	if strings.Contains(content, "Terminal window is too small") {
		t.Fatalf("minimum supported size should render normal content:\n%s", content)
	}
	if !strings.Contains(content, "Library") {
		t.Fatalf("expected normal library content at minimum size, got:\n%s", content)
	}
}

func TestTerminalTooSmallMessageFitsTinyTerminal(t *testing.T) {
	m := New(context.Background())
	next, _ := m.Update(tea.WindowSizeMsg{Width: 20, Height: 3})
	m = next.(Model)

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
	m.activeView = viewTags

	out := m.renderTitleBar()
	// The Tags badge must show "0", not "10".
	if strings.Contains(out, "10") {
		t.Errorf("title bar should show badge '0' for Tags, not '10': got %q", out)
	}
}
