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

func TestRenderEmptyCardCentersWrappedCardVertically(t *testing.T) {
	s := styles.NewStyles(true)
	actions := []styles.EmptyAction{
		{Key: "ctrl+shift+s", Label: "scan folders"},
		{Key: "?", Label: "help"},
	}

	cardOnly := s.RenderEmptyCard(12, 0, "Queue is empty", actions)
	out := s.RenderEmptyCard(12, 14, "Queue is empty", actions)

	wantTopPad := (14 - len(strings.Split(cardOnly, "\n"))) / 2
	gotTopPad := 0
	for _, line := range strings.Split(out, "\n") {
		if line != "" {
			break
		}
		gotTopPad++
	}
	if gotTopPad != wantTopPad {
		t.Fatalf("leading blank lines = %d, want %d", gotTopPad, wantTopPad)
	}
}
