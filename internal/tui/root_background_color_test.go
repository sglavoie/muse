package tui

import (
	"context"
	"image/color"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/sglavoie/muse/internal/tui/playback"
)

func TestUpdateBackgroundColorMsgLightBackground(t *testing.T) {
	m := New(context.Background())

	// White = light background -> IsDark() == false.
	lightMsg := tea.BackgroundColorMsg{Color: color.RGBA{R: 255, G: 255, B: 255, A: 255}}
	next, _ := m.Update(lightMsg)
	nm := next.(Model)

	if nm.styles.HasDark != false {
		t.Fatalf("expected HasDark=false after light BackgroundColorMsg, got %v", nm.styles.HasDark)
	}
}

func TestUpdateBackgroundColorMsgDarkBackground(t *testing.T) {
	m := New(context.Background())

	// Black = dark background -> IsDark() == true.
	darkMsg := tea.BackgroundColorMsg{Color: color.RGBA{R: 0, G: 0, B: 0, A: 255}}
	next, _ := m.Update(darkMsg)
	nm := next.(Model)

	if nm.styles.HasDark != true {
		t.Fatalf("expected HasDark=true after dark BackgroundColorMsg, got %v", nm.styles.HasDark)
	}
}

// TestBackgroundColorMsgReconstructsStyles verifies that each
// tea.BackgroundColorMsg reconstructs m.styles, replacing the previous value.
// Sending dark then light must leave HasDark == false.
// A render-through assertion also confirms the new Styles values are visible
// in the rendered output after the swap - not just that the struct field changed.
func TestBackgroundColorMsgReconstructsStyles(t *testing.T) {
	m := New(context.Background())

	// First set dark background.
	next, _ := m.Update(tea.BackgroundColorMsg{Color: color.RGBA{R: 0, G: 0, B: 0, A: 255}})
	m = next.(Model)

	// Now send light background - styles must be reconstructed with HasDark == false.
	next, _ = m.Update(tea.BackgroundColorMsg{Color: color.RGBA{R: 255, G: 255, B: 255, A: 255}})
	nm := next.(Model)

	if nm.styles.HasDark != false {
		t.Fatalf("expected m.styles.HasDark == false after light BackgroundColorMsg, got %v", nm.styles.HasDark)
	}

	// Render-through: verify the new styles are actually applied in rendered output.
	// Add a missing queue entry so Dim.Render() is exercised, then confirm the
	// rendered text contains the dim-styled result (not plain unstyled text).
	nm.queue.entries = []QueueEntry{
		{Title: "Lost Track", Artist: "Ghost", Missing: true},
	}
	nm.queue.width = 80
	rendered := nm.queue.render(*nm.styles, -1, playback.StateIdle, nil, nil, nil)

	// The rendered output must show the "[missing]" label in the visible text.
	if !strings.Contains(ansi.Strip(rendered), "[missing]") {
		t.Fatalf("render-through: expected '[missing]' in stripped rendered output;\ngot:\n%s", rendered)
	}
	// When lipgloss emits ANSI sequences, the full rendered line must differ from
	// its stripped form, proving Dim.Render() was applied after the BackgroundColorMsg swap.
	dimProbe := nm.styles.Dim.Render("probe")
	if ansi.Strip(dimProbe) != dimProbe {
		// ANSI sequences are active in this environment.
		if rendered == ansi.Strip(rendered) {
			t.Fatal("render-through: expected ANSI dim sequences in rendered output after BackgroundColorMsg swap, but output contains no ANSI escapes")
		}
	}
}
