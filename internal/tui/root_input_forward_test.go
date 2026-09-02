package tui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/config"
	"github.com/sglavoie/muse/internal/tui/filter"
	"github.com/sglavoie/muse/internal/tui/metadata"
	"github.com/sglavoie/muse/internal/tui/palette"
	"github.com/sglavoie/muse/internal/tui/settings"
)

func TestInputForwardingPrefersHelpSearchOverOverlayInputs(t *testing.T) {
	m := New(context.Background())
	m.helpVisible = true
	m.termWidth = 80
	m.termHeight = 24
	m.helpSearchFilter = filter.New()
	m.helpSearchFilter, _ = m.helpSearchFilter.Activate()
	overlaySettings := settings.NewSettingsOverlay(config.Defaults(), 50, nil)
	overlaySettings.ExclPathInput = settings.NewSettingsInput()
	m.overlay.settings = &overlaySettings

	next, _, ok := m.forwardToActiveInput(tea.KeyPressMsg{Code: 'x', Text: "x"})
	if !ok {
		t.Fatal("expected active input to consume forwarded msg")
	}

	if next.helpSearchFilter.Text != "x" {
		t.Fatalf("expected help search input to consume msg, got %q", next.helpSearchFilter.Text)
	}
	if got := next.overlay.settings.ExclPathInput.Value(); got != "" {
		t.Fatalf("expected settings input to remain untouched, got %q", got)
	}
}

func TestInputForwardingPrefersSettingsOverMetadataEditor(t *testing.T) {
	m := New(context.Background())
	overlaySettings := settings.NewSettingsOverlay(config.Defaults(), 50, nil)
	overlaySettings.ExclPathInput = settings.NewSettingsInput()
	m.overlay.settings = &overlaySettings
	editor := metadata.NewEditor("hash", "title", "artist", "album", "genre", 0)
	m.overlay.metadataEditor = &editor

	next, _, ok := m.forwardToActiveInput(tea.KeyPressMsg{Code: 'z', Text: "z"})
	if !ok {
		t.Fatal("expected settings input to consume forwarded msg")
	}

	if got := next.overlay.settings.ExclPathInput.Value(); got != "z" {
		t.Fatalf("expected settings input to update, got %q", got)
	}
	if got := next.overlay.metadataEditor.Title(); got != "title" {
		t.Fatalf("expected metadata editor to remain unchanged, got %q", got)
	}
}

func TestInputForwardingPrefersMetadataEditorOverViewFilter(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewLibrary
	m.library.Filter = filter.New()
	m.library.Filter, _ = m.library.Filter.Activate()
	editor := metadata.NewEditor("hash", "title", "artist", "album", "genre", 0)
	m.overlay.metadataEditor = &editor

	next, _, ok := m.forwardToActiveInput(tea.KeyPressMsg{Code: 'm', Text: "m"})
	if !ok {
		t.Fatal("expected metadata editor to consume forwarded msg")
	}

	if got := next.overlay.metadataEditor.Title(); got != "titlem" {
		t.Fatalf("expected metadata editor title to update, got %q", got)
	}
	if got := next.library.Filter.Text; got != "" {
		t.Fatalf("expected view filter to remain unchanged, got %q", got)
	}
}

func TestInputForwardingIncludesCommandPaletteFilter(t *testing.T) {
	m := New(context.Background())
	cp, _ := palette.NewCommandPalette([]palette.PaletteCommand{
		{Name: "Reveal in Finder"},
		{Name: "Copy track info"},
	}, nil)
	m.overlay.commandPalette = &cp

	next, _, ok := m.forwardToActiveInput(tea.KeyPressMsg{Code: 'v', Text: "v"})
	if !ok {
		t.Fatal("expected command palette filter to consume forwarded msg")
	}
	if next.overlay.commandPalette == nil {
		t.Fatal("expected command palette to remain open")
	}

	got := next.overlay.commandPalette.Filter.Text
	if got != "v" {
		t.Fatalf("expected command palette filter text %q, got %q", "v", got)
	}
	filtered := next.overlay.commandPalette.Filtered
	if len(filtered) != 1 || filtered[0].Name != "Reveal in Finder" {
		t.Fatalf("expected filtered command Reveal in Finder, got %+v", filtered)
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
