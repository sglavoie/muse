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
		cp.Filter.Text = name
		cp.Refilter()
		for i, command := range cp.Filtered {
			if command.Name == name {
				cp.cursor = i
			}
		}
		for _, height := range []int{12, 27, 12} {
			_, cmd := cp.HandleKey("enter", tea.KeyPressMsg{}, height)
			if cmd == nil || cmd().(CommandPalettePickedMsg).Command.Name != name {
				t.Fatalf("resizing changed %s into another action", name)
			}
		}
	}
}

func TestListeningPriorityKeepsUsageAndExplicitRemovalSearch(t *testing.T) {
	usage := NewUsageStore("")
	usage.Bump("next")
	for range 10 {
		usage.Bump("dismiss")
	}
	commands := []PaletteCommand{
		{ID: "enqueue", Name: "Enqueue tracks", Intent: IntentListening},
		{ID: "next", Name: "Play tracks next", Intent: IntentListening},
		{ID: "dismiss", Name: "Dismiss track", Intent: IntentRemoval, Relevant: true},
		{ID: "ordinary", Name: "Create playlist"},
		{ID: "disabled", Name: "Show track info", Intent: IntentListening, Enabled: func() bool { return false }},
	}
	cp, _ := NewCommandPalette(commands, usage)
	want := []string{"next", "enqueue", "ordinary", "dismiss", "disabled"}
	for i, id := range want {
		if cp.Filtered[i].ID != id {
			t.Fatalf("position %d: got %s want %s", i, cp.Filtered[i].ID, id)
		}
	}
	cp.Filter.Text = "dismiss"
	cp.Refilter()
	if len(cp.Filtered) != 1 || cp.Filtered[0].ID != "dismiss" {
		t.Fatal("explicit search lost removal action")
	}
}
