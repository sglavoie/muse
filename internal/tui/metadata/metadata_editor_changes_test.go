package metadata

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestMetadataChangeMarkersFollowEditsAndReverts(t *testing.T) {
	for _, bulk := range []bool{false, true} {
		ed := NewEditor("h1", "Song", "Artist", "Album", "Genre", 183.25)
		if bulk {
			ed = NewBulkEditor([]string{"h1", "h2"}, "Song", "Artist", "Album", "Genre", "3:03")
		}
		ed, _, _ = ed.HandleKey(tea.KeyPressMsg{Code: 'x', Text: "x"})
		out := ansi.Strip(ed.Render(60, 15, newTestStyles()))
		if !strings.Contains(out, "*Title:") || !strings.Contains(out, "1 field changed") {
			t.Fatal(out)
		}
		ed, _, _ = ed.HandleKey(tea.KeyPressMsg{Code: tea.KeyTab})
		ed, _, _ = ed.HandleKey(tea.KeyPressMsg{Code: 'x', Text: "x"})
		out = ansi.Strip(ed.Render(60, 15, newTestStyles()))
		if !strings.Contains(out, "*Artist:") || !strings.Contains(out, "2 fields changed") {
			t.Fatal(out)
		}
		for _, line := range strings.Split(out, "\n") {
			if ansi.StringWidth(line) > 60 {
				t.Fatalf("marker overflows narrow terminal: %q", line)
			}
		}
		if len(strings.Split(strings.TrimRight(out, "\n"), "\n")) > 15 {
			t.Fatalf("editor exceeds terminal height:\n%s", out)
		}
		ed, _, _ = ed.HandleKey(tea.KeyPressMsg{Code: tea.KeyBackspace})
		ed, _, _ = ed.HandleKey(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
		ed, _, _ = ed.HandleKey(tea.KeyPressMsg{Code: tea.KeyBackspace})
		out = ansi.Strip(ed.Render(60, 15, newTestStyles()))
		if strings.Contains(out, "field changed") || strings.Contains(out, "fields changed") || strings.Contains(out, "*Title:") || strings.Contains(out, "*Artist:") {
			t.Fatalf("reverted edits still marked:\n%s", out)
		}
	}
}
