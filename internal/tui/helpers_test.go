package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
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
		// bubbles cursor blink goroutine spawned when a text input (e.g. the
		// command palette filter) activates. The uitest driver drives models
		// outside a live tea runtime, so the blink command blocks on a channel
		// receive that is never signaled; it is discarded with the model.
		goleak.IgnoreTopFunction("charm.land/bubbles/v2/cursor.(*Model).Blink.func1"),
	)
}

// styledText returns text wrapped with the given lipgloss style, producing
// the same string that production code generates via Style.Render.
func styledText(style lipgloss.Style, text string) string {
	return style.Render(text)
}

// drainCmd recursively executes cmd and any cmds nested in tea.BatchMsg /
// tea.sequenceMsg values returned along the way. Tests use it when the
// production handler returns a batched cmd but the test only cares that the
// underlying side-effect-producing cmds run.
func drainCmd(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			drainCmd(c)
		}
	}
}
