package palette

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/tui/styles"
)

func TestCompactPalettePrioritizesRunnableContextAndPreservesResizeTarget(t *testing.T) {
	commands := pinnedCommands()
	commands = append(commands, PaletteCommand{Name: "A disabled", Relevant: true, Enabled: func() bool { return false }}, PaletteCommand{Name: "Z current view", Relevant: true})
	for i := range 12 {
		commands = append(commands, PaletteCommand{Name: fmt.Sprintf("Action %02d", i)})
	}
	cp, _ := NewCommandPalette(commands, nil)
	cp = cp.WithHeight(12)
	if cp.Filtered[0].Name != "Z current view" {
		t.Fatal(cp.Filtered)
	}
	out := ansi.Strip(cp.Render(60, 12, styles.NewStyles(true)))
	if strings.Count(out, "Action ") < 5 || !strings.Contains(out, "F1 help · ctrl+c quit") {
		t.Fatal(out)
	}
	if len(strings.Split(strings.TrimSuffix(out, "\n"), "\n")) > 12 {
		t.Fatal("compact palette overflow", out)
	}
	for _, height := range []int{27, 12, 30, 12} {
		cp = cp.WithHeight(height)
		_, cmd := cp.HandleKey("enter", tea.KeyPressMsg{}, height)
		if cmd == nil || cmd().(CommandPalettePickedMsg).Command.Name != "Z current view" {
			t.Fatal("resize changed execution target")
		}
	}
	_, help := cp.HandleKey("f1", tea.KeyPressMsg{}, 12)
	if help == nil || help().(CommandPalettePickedMsg).Command.Name != "Help" {
		t.Fatal("help shortcut did not dispatch")
	}
	for _, query := range []string{"help", "quit", "disabled"} {
		cp.Filter.Text = query
		cp.Refilter()
		if len(cp.Filtered) != 1 {
			t.Fatalf("%s no longer discoverable", query)
		}
	}
}

func TestResizePreservesFocusedHelpAndQuit(t *testing.T) {
	for _, name := range []string{"Help", "Quit"} {
		cp, _ := NewCommandPalette(pinnedCommands(), nil)
		for i, command := range cp.Filtered {
			if command.Name == name {
				cp.cursor = i
			}
		}
		for _, height := range []int{12, 27, 12} {
			cp = cp.WithHeight(height)
			_, cmd := cp.HandleKey("enter", tea.KeyPressMsg{}, height)
			if cmd == nil || cmd().(CommandPalettePickedMsg).Command.Name != name {
				t.Fatalf("resizing changed %s into another action", name)
			}
		}
	}
}
