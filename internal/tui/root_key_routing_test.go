package tui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestKeyRoutingPrefersHelpOverlayOverPreview(t *testing.T) {
	m := New(context.Background())
	m.helpVisible = true
	m.prevActiveView = viewQueue
	m.overlay.preview = &PreviewOverlay{}

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	nm := next.(Model)

	if nm.helpVisible {
		t.Fatal("expected help overlay to consume esc and close first")
	}
	if nm.overlay.preview == nil {
		t.Fatal("expected preview overlay to remain open when help overlay has priority")
	}
	if nm.activeView != viewQueue {
		t.Fatalf("expected esc to restore prevActiveView=%d, got %d", viewQueue, nm.activeView)
	}
}

func TestKeyRoutingFallsBackToPreviewWhenNoHigherOverlayConsumes(t *testing.T) {
	m := New(context.Background())
	m.overlay.preview = &PreviewOverlay{}

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	nm := next.(Model)

	if nm.overlay.preview != nil {
		t.Fatal("expected preview overlay to consume esc when no higher-priority overlay is active")
	}
}

func TestKeyRoutingLeavesViewActionsAvailableWithoutInterceptors(t *testing.T) {
	m := New(context.Background())

	next, _ := m.Update(tea.KeyPressMsg{Code: '2', Text: "2"})
	nm := next.(Model)

	if nm.activeView != viewPlaylists {
		t.Fatalf("expected unhandled keypress to fall through to view switching, got view %d", nm.activeView)
	}
}
