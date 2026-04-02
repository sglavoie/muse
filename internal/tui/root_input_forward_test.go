package tui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/config"
	"github.com/sglavoie/muse/internal/tui/filter"
)

func TestInputForwardingPrefersHelpSearchOverOverlayInputs(t *testing.T) {
	m := New(context.Background())
	m.helpVisible = true
	m.termWidth = 80
	m.termHeight = 24
	m.helpSearchFilter = filter.New()
	m.helpSearchFilter, _ = m.helpSearchFilter.Activate()
	settings := NewSettingsOverlay(config.Defaults(), 50, nil)
	settings.exclPathInput = newSettingsInput()
	m.overlay.settings = &settings

	next, _, ok := m.forwardToActiveInput(tea.KeyPressMsg{Code: 'x', Text: "x"})
	if !ok {
		t.Fatal("expected active input to consume forwarded msg")
	}

	if next.helpSearchFilter.Text != "x" {
		t.Fatalf("expected help search input to consume msg, got %q", next.helpSearchFilter.Text)
	}
	if got := next.overlay.settings.exclPathInput.Value(); got != "" {
		t.Fatalf("expected settings input to remain untouched, got %q", got)
	}
}

func TestInputForwardingPrefersSettingsOverMetadataEditor(t *testing.T) {
	m := New(context.Background())
	settings := NewSettingsOverlay(config.Defaults(), 50, nil)
	settings.exclPathInput = newSettingsInput()
	m.overlay.settings = &settings
	editor := NewMetadataEditor("hash", "title", "artist", "album", "genre", 0)
	m.overlay.metadataEditor = &editor

	next, _, ok := m.forwardToActiveInput(tea.KeyPressMsg{Code: 'z', Text: "z"})
	if !ok {
		t.Fatal("expected settings input to consume forwarded msg")
	}

	if got := next.overlay.settings.exclPathInput.Value(); got != "z" {
		t.Fatalf("expected settings input to update, got %q", got)
	}
	if got := next.overlay.metadataEditor.Title(); got != "title" {
		t.Fatalf("expected metadata editor to remain unchanged, got %q", got)
	}
}

func TestInputForwardingPrefersMetadataEditorOverViewFilter(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewLibrary
	m.library.filter = filter.New()
	m.library.filter, _ = m.library.filter.Activate()
	editor := NewMetadataEditor("hash", "title", "artist", "album", "genre", 0)
	m.overlay.metadataEditor = &editor

	next, _, ok := m.forwardToActiveInput(tea.KeyPressMsg{Code: 'm', Text: "m"})
	if !ok {
		t.Fatal("expected metadata editor to consume forwarded msg")
	}

	if got := next.overlay.metadataEditor.Title(); got != "titlem" {
		t.Fatalf("expected metadata editor title to update, got %q", got)
	}
	if got := next.library.filter.Text; got != "" {
		t.Fatalf("expected view filter to remain unchanged, got %q", got)
	}
}

func TestInputForwardingFallsBackToQueueSaveInput(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewQueue
	m.queue = m.queue.startSaveAsPlaylist(false)

	next, _, ok := m.forwardToActiveInput(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if !ok {
		t.Fatal("expected queue save input to consume forwarded msg")
	}

	if got := next.queue.saveInput.Value(); got != "q" {
		t.Fatalf("expected queue save input to update, got %q", got)
	}
}