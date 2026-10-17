package tui

import (
	"context"
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/tui/palette"
)

func TestPaletteUndoAvailabilityAndDescription(t *testing.T) {
	m := New(context.Background())
	find := func() palette.PaletteCommand {
		t.Helper()
		for _, c := range m.BuildPaletteCommands() {
			if c.ID == "app.undo" {
				return c
			}
		}
		t.Fatal("undo missing")
		return palette.PaletteCommand{}
	}
	if c := find(); c.IsEnabled() || c.DisabledReason != "Nothing to undo" {
		t.Fatal("empty undo should explain unavailability")
	}
	m.undo.slot = undoSlot{action: undoClearAction{}, expiresAt: time.Now().Add(time.Minute)}
	if c := find(); !c.IsEnabled() || c.Detail != "Revert: Queue cleared" {
		t.Fatalf("unexpected undo: %+v", c)
	}
	m.undo.slot.expiresAt = time.Now().Add(-time.Second)
	if !find().IsEnabled() {
		t.Fatal("undo remains available after its banner fades")
	}
	// The action remains executable after the banner fades.
	m = m.handleUndo()
	if m.canUndo() {
		t.Fatal("successful undo should consume the action")
	}
}
