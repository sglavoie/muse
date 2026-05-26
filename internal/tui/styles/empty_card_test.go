package styles_test

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/sglavoie/muse/internal/tui/styles"
)

func TestRenderEmptyCardClampsToViewportWidth(t *testing.T) {
	out := styles.NewStyles(true).RenderEmptyCard(12, 0, "Queue is empty", []styles.EmptyAction{
		{Key: "ctrl+shift+s", Label: "scan folders"},
		{Key: "?", Label: "help"},
	})

	for i, line := range strings.Split(out, "\n") {
		if w := lipgloss.Width(line); w > 12 {
			t.Fatalf("line %d width = %d, want <= 12; line=%q", i, w, line)
		}
	}
}
