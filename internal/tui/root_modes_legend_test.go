package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/sglavoie/muse/internal/tui/playback"
)

// pressSemicolon sends the ';' key press and returns the resulting Model.
func pressSemicolon(m Model) Model {
	next, _ := m.Update(tea.KeyPressMsg{Code: ';', Text: ";"})
	return next.(Model)
}

// TestModesLegendKeyOpensOverlay verifies that ';' opens the legend overlay.
func TestModesLegendKeyOpensOverlay(t *testing.T) {
	m := New(context.Background())

	next, cmd := m.Update(tea.KeyPressMsg{Code: ';', Text: ";"})
	nm := next.(Model)

	if cmd != nil {
		t.Fatal("expected nil command from ';' key")
	}
	if nm.overlay.modesLegend == nil {
		t.Fatal("expected modesLegend overlay to be set")
	}
}

// TestModesLegendSnapshotsLiveModeState verifies that opening the legend
// captures the four live mode inputs, including m.radio.enabled which lives
// outside m.playback.
func TestModesLegendSnapshotsLiveModeState(t *testing.T) {
	m := New(context.Background())
	m.playback.repeatMode = playback.RepeatOne
	m.playback.shuffle = true
	m.playback.stopAfterCurrent = true
	m.radio.enabled = true

	nm := pressSemicolon(m)

	legend := nm.overlay.modesLegend
	if legend == nil {
		t.Fatal("expected modesLegend overlay to be set")
	}
	if legend.repeat != playback.RepeatOne {
		t.Errorf("expected repeat RepeatOne, got %v", legend.repeat)
	}
	if !legend.shuffle {
		t.Error("expected shuffle to be captured as true")
	}
	if !legend.stopAfter {
		t.Error("expected stopAfter to be captured as true")
	}
	if !legend.radio {
		t.Error("expected radio to be captured from m.radio.enabled")
	}
}

// TestModesLegendReopensWithUpdatedState verifies that the overlay is rebuilt
// from the model each time it opens rather than reusing a stale snapshot.
func TestModesLegendReopensWithUpdatedState(t *testing.T) {
	m := New(context.Background())

	nm := pressSemicolon(m)
	if nm.overlay.modesLegend == nil || nm.overlay.modesLegend.shuffle {
		t.Fatal("expected an overlay with shuffle off")
	}

	nm = pressSemicolon(nm)
	if nm.overlay.modesLegend != nil {
		t.Fatal("expected ';' to close the overlay")
	}

	nm.playback.shuffle = true
	nm = pressSemicolon(nm)
	if nm.overlay.modesLegend == nil {
		t.Fatal("expected modesLegend overlay to reopen")
	}
	if !nm.overlay.modesLegend.shuffle {
		t.Error("expected the reopened overlay to reflect shuffle=true")
	}
}

// TestModesLegendSemicolonCloses verifies that ';' toggles the overlay closed,
// matching the overlay's own "; or esc to dismiss" hint.
func TestModesLegendSemicolonCloses(t *testing.T) {
	m := New(context.Background())
	m.overlay.modesLegend = &ModesLegendOverlay{}

	nm := pressSemicolon(m)

	if nm.overlay.modesLegend != nil {
		t.Fatal("expected overlay to be nil after ';'")
	}
}

// TestModesLegendEscCloses verifies that Esc dismisses the overlay.
func TestModesLegendEscCloses(t *testing.T) {
	m := New(context.Background())
	m.overlay.modesLegend = &ModesLegendOverlay{}

	nm := pressEsc(m)

	if nm.overlay.modesLegend != nil {
		t.Fatal("expected overlay to be nil after Esc")
	}
}

// TestModesLegendSwallowsKeys verifies that while the legend is up, other keys
// do not reach the view underneath.
func TestModesLegendSwallowsKeys(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewLibrary
	m.overlay.modesLegend = &ModesLegendOverlay{}

	// '3' switches to the Queue view when no overlay is active.
	nm := pressKey(m, "3")

	if nm.activeView != viewLibrary {
		t.Errorf("expected active view to stay viewLibrary, got %v", nm.activeView)
	}
	if nm.overlay.modesLegend == nil {
		t.Error("expected the overlay to stay open after a swallowed key")
	}
}

// TestModesLegendCtrlCQuits verifies that ctrl+c still quits while the legend
// is up, and clears the overlay on the way out.
func TestModesLegendCtrlCQuits(t *testing.T) {
	m := New(context.Background())
	m.overlay.modesLegend = &ModesLegendOverlay{}

	next, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	nm := next.(Model)

	if cmd == nil {
		t.Fatal("expected a quit command from ctrl+c")
	}
	if nm.overlay.modesLegend != nil {
		t.Error("expected overlay to be cleared before quitting")
	}
}

// TestModesLegendCountsAsActiveOverlay verifies that the legend participates in
// both overlay registries: hasActiveOverlay (which suppresses footer hints and
// the primary body) and activeOverlayName (the uitest snapshot field).
func TestModesLegendCountsAsActiveOverlay(t *testing.T) {
	m := New(context.Background())

	if m.hasActiveOverlay() {
		t.Fatal("expected no active overlay on a fresh model")
	}
	if name := m.activeOverlayName(); name != "" {
		t.Fatalf("expected empty overlay name on a fresh model, got %q", name)
	}

	nm := pressSemicolon(m)

	if !nm.hasActiveOverlay() {
		t.Error("expected hasActiveOverlay to report the modes legend")
	}
	if name := nm.activeOverlayName(); name != "modes_legend" {
		t.Errorf("expected overlay name %q, got %q", "modes_legend", name)
	}
}

// TestModesLegendRendersAsOverlayBody verifies that the legend is reachable
// through renderOverlayBody, so opening it actually paints the box.
func TestModesLegendRendersAsOverlayBody(t *testing.T) {
	m := New(context.Background())
	m.termWidth = 100
	m.termHeight = 30

	nm := pressSemicolon(m)

	body, ok := nm.renderOverlayBody()
	if !ok {
		t.Fatal("expected renderOverlayBody to claim the frame")
	}
	// The title is styled per-character, so compare against the stripped text.
	plain := ansi.Strip(body)
	if !strings.Contains(plain, "Playback Modes") {
		t.Errorf("expected the legend title in the rendered body, got:\n%s", plain)
	}
	if !strings.Contains(plain, "Radio (Auto-DJ)") {
		t.Errorf("expected a legend row in the rendered body, got:\n%s", plain)
	}
}

// TestModesLegendKeyIsNoOpInFilterInput verifies that ';' typed into an active
// filter input is literal text, not a legend toggle. The legend handler sits
// above handleFilterKeyPress in orderedKeyPressHandlers, so this only holds
// because its nil guard declines the key when no overlay is open.
func TestModesLegendKeyIsNoOpInFilterInput(t *testing.T) {
	m := newModelWithTracks()

	m = pressSlash(m)
	if !m.library.filter.Active {
		t.Fatal("expected the library filter to be active after '/'")
	}

	m = typeText(m, "a;b")

	if m.overlay.modesLegend != nil {
		t.Error("expected ';' not to open the legend while a filter input is active")
	}
	if got := m.library.filter.Text; got != "a;b" {
		t.Errorf("expected ';' to be typed into the filter, got %q", got)
	}
}
