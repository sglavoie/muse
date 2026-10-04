package palette

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/tui/styles"
)

func TestPaletteStateUsesDetailLine(t *testing.T) {
	for _, width := range []int{60, 100} {
		cmd := PaletteCommand{ID: "toggle", Name: "Toggle queue follow", CurrentState: "On"}
		cp, _ := NewCommandPalette([]PaletteCommand{cmd}, nil)
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
		disabled := ansi.Strip(cp.Render(width, 15, styles.NewStyles(true)))
		if strings.Contains(disabled, "Current:") || !strings.Contains(disabled, cmd.DisabledReason) {
			t.Fatal("disabled reason must take precedence over state")
		}
		if strings.Count(out, "\n") != strings.Count(disabled, "\n") {
			t.Fatal("state changed the dialog height")
		}
	}
}
