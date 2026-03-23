package tui

import (
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/sglavoie/muse/internal/player"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	player.Muted = true
	goleak.VerifyTestMain(m,
		// bubbletea Tick goroutine used in TUI tests; it blocks on a channel
		// receive and is not stopped when the test model is discarded.
		goleak.IgnoreTopFunction("charm.land/bubbletea/v2.Tick.func1"),
	)
}

// styledText returns text wrapped with the given lipgloss style, producing
// the same string that production code generates via Style.Render.
func styledText(style lipgloss.Style, text string) string {
	return style.Render(text)
}
