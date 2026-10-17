package settings

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/config"
	"github.com/sglavoie/muse/internal/tui/styles"
)

func TestTabAliasesCycleAndRespectEditors(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), []string{"/music"})
	forward := tea.KeyPressMsg{Code: tea.KeyTab}
	back := tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
	for _, tab := range []Tab{TabPlayback, TabEqualizer, TabDisplay, tabLibrary} {
		o, _, _ = o.HandleKey(forward)
		if o.Tab != tab {
			t.Fatalf("Tab: got %v, want %v", o.Tab, tab)
		}
	}
	o, _, _ = o.HandleKey(back)
	if o.Tab != TabDisplay {
		t.Fatal("Shift+Tab should wrap to Display")
	}
	o.Tab = tabLibrary
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: 'a', Text: "a"})
	for _, key := range []tea.KeyPressMsg{forward, back} {
		o, _, _ = o.HandleKey(key)
		if o.Tab != tabLibrary || o.LibDirInput == nil {
			t.Fatal("tab navigation escaped the directory editor")
		}
	}
}

func TestEverySettingsTabKeepsFocusAndExitControlsAtMinimumHeight(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), []string{"/one", "/two", "/three", "/four", "/five", "/six", "/seven"})
	s := styles.NewStyles(false)
	// The root reserves three lines, leaving twelve for the overlay at 60x15.
	for tab := tabLibrary; tab < tabCount; tab++ {
		o.Tab = tab
		for range 15 {
			o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: 'j', Text: "j"})
			out := strings.TrimRight(ansi.Strip(o.Render(60, 12, s)), "\n")
			if len(strings.Split(out, "\n")) > 12 {
				t.Fatalf("tab %d overflows:\n%s", tab, out)
			}
			for _, want := range []string{"▎ ", "ctrl+s save", "esc cancel"} {
				if !strings.Contains(out, want) {
					t.Fatalf("tab %d missing %q:\n%s", tab, want, out)
				}
			}
		}
	}
}
