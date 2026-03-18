package tui

import (
	"os"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/sglavoie/muse/internal/player"
)

func TestMain(m *testing.M) {
	player.Muted = true
	os.Exit(m.Run())
}

// styledText returns text wrapped with the given lipgloss style, producing
// the same string that production code generates via Style.Render.
func styledText(style lipgloss.Style, text string) string {
	return style.Render(text)
}
