package palette

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/tui/styles"
)

func TestShortlistKeepsAllCommandsSearchable(t *testing.T) {
	commands := []PaletteCommand{
		{ID: "listen", Name: "Play next", Intent: IntentListening, Detail: "Next: Song — Artist"},
		{ID: "current", Name: "Rename playlist", Relevant: true},
		{ID: "remove", Name: "Clear queue", Intent: IntentRemoval},
		{ID: "disabled", Name: "Unavailable action", Enabled: func() bool { return false }, DisabledReason: "Select a track"},
		{ID: "tag.enqueue.1", Name: "Enqueue tag: jazz"},
	}
	for i := range 30 {
		commands = append(commands, PaletteCommand{ID: fmt.Sprint(i), Name: fmt.Sprintf("Action %02d", i)})
	}
	cp, _ := NewCommandPalette(commands, nil)
	if len(cp.Filtered) != 8 || cp.Filtered[0].ID != "listen" || cp.Filtered[1].ID != "current" {
		t.Fatal(cp.Filtered)
	}
	for _, size := range [][2]int{{60, 12}, {100, 27}} {
		for i := range cp.Filtered {
			cp.cursor = i
			out := ansi.Strip(cp.Render(size[0], size[1], styles.NewStyles(true)))
			if !strings.Contains(out, cp.Filtered[i].Name) || !strings.Contains(out, "Type to search all") {
				t.Fatal(out)
			}
			if len(strings.Split(strings.TrimSuffix(out, "\n"), "\n")) > size[1] {
				t.Fatal(out)
			}
		}
	}
	for _, command := range commands {
		cp.Filter.Text = command.Name
		cp.Refilter()
		if len(cp.Filtered) == 0 || cp.Filtered[0].ID != command.ID {
			t.Fatalf("cannot find %s", command.ID)
		}
	}
	cp.Filter.Text = ""
	cp.Refilter()
	if cp.cursor != 0 || len(cp.Filtered) != 8 {
		t.Fatal("clear did not restore shortlist")
	}
	out := ansi.Strip(cp.Render(100, 27, styles.NewStyles(true)))
	for _, want := range []string{"Listening", "This view", "Next: Song — Artist"} {
		if !strings.Contains(out, want) {
			t.Fatal(out)
		}
	}
}
