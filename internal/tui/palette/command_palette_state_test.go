package palette

import (
	tea "charm.land/bubbletea/v2"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/tui/styles"
)

func TestPaletteStateUsesDetailLine(t *testing.T) {
	for _, width := range []int{60, 100} {
		cmd := PaletteCommand{ID: "toggle", Name: "Toggle queue follow", CurrentState: "On"}
		cp, _ := NewCommandPalette([]PaletteCommand{cmd}, nil)
		cp.Filter.Text = "Toggle"
		cp.Refilter()
		out := ansi.Strip(cp.Render(width, 15, styles.NewStyles(true)))
		if !strings.Contains(out, "Current: On") {
			t.Fatalf("missing state at width %d: %s", width, out)
		}
		for _, line := range strings.Split(out, "\n") {
			if ansi.StringWidth(line) > width {
				t.Fatalf("state overflows at width %d", width)
			}
		}
		cmd.Enabled = func() bool { return false }
		cmd.DisabledReason = "Unavailable now"
		cp, _ = NewCommandPalette([]PaletteCommand{cmd}, nil)
		cp.Filter.Text = "Toggle"
		cp.Refilter()
		disabled := ansi.Strip(cp.Render(width, 15, styles.NewStyles(true)))
		if strings.Contains(disabled, "Current:") || !strings.Contains(disabled, cmd.DisabledReason) {
			t.Fatal("disabled reason must take precedence over state")
		}
		if strings.Count(out, "\n") != strings.Count(disabled, "\n") {
			t.Fatal("state changed the dialog height")
		}
	}
}

func TestPaletteUndoDetailExpiresWhileOpen(t *testing.T) {
	enabled := true
	cmd := PaletteCommand{ID: "app.undo", Name: "Undo last change", Detail: "Revert: Queue cleared", DisabledReason: "Nothing to undo", Enabled: func() bool { return enabled }}
	cp, _ := NewCommandPalette([]PaletteCommand{cmd}, nil)
	if out := ansi.Strip(cp.Render(60, 15, styles.NewStyles(true))); !strings.Contains(out, cmd.Detail) {
		t.Fatal("missing undo detail")
	}
	enabled = false
	if out := ansi.Strip(cp.Render(60, 15, styles.NewStyles(true))); !strings.Contains(out, cmd.DisabledReason) || strings.Contains(out, cmd.Detail) {
		t.Fatal("stale detail after expiry")
	}
	_, action := cp.HandleKey("enter", tea.KeyPressMsg{Code: tea.KeyEnter})
	if action != nil {
		t.Fatal("disabled undo must not dispatch")
	}
}
