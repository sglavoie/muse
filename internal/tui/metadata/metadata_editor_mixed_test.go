package metadata

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestMixedMetadataPlaceholderIsDisplayOnly(t *testing.T) {
	ed := NewBulkEditor([]string{"h1", "h2"}, "", "Artist", "", "", "").
		WithMixedValues([]string{"One", "Two"}, []string{"Artist", "Artist"}, []string{"", ""})
	for _, size := range [][2]int{{100, 30}, {60, 15}} {
		for _, focus := range []Field{fieldTitle, fieldArtist} {
			ed.focused = focus
			out := ansi.Strip(ed.Render(size[0], size[1], newTestStyles()))
			if strings.Count(out, mixedValuePlaceholder) != 1 {
				t.Fatalf("expected only the mixed title to have a placeholder:\n%s", out)
			}
			for _, line := range strings.Split(out, "\n") {
				if ansi.StringWidth(line) > size[0] {
					t.Fatalf("line exceeds terminal width: %q", line)
				}
			}
		}
	}
	if ed.Title() != "" || ed.Album() != "" {
		t.Fatal("placeholder became a field value")
	}
	title, artist, album, genre, duration := ed.DirtyFields()
	if title != nil || artist != nil || album != nil || genre != nil || duration != nil {
		t.Fatal("opening mixed fields must not mark them dirty")
	}

	ed.focused = fieldTitle
	ed, _, _ = ed.HandleKey(tea.KeyPressMsg{Code: 'X', Text: "X"})
	title, _, _, _, _ = ed.DirtyFields()
	if title == nil || *title != "X" || strings.Contains(ansi.Strip(ed.Render(100, 30, newTestStyles())), mixedValuePlaceholder) {
		t.Fatal("typing should replace the placeholder with an ordinary dirty value")
	}
	ed, _, _ = ed.HandleKey(tea.KeyPressMsg{Code: tea.KeyBackspace})
	title, _, _, _, _ = ed.DirtyFields()
	if title != nil || !strings.Contains(ansi.Strip(ed.Render(100, 30, newTestStyles())), mixedValuePlaceholder) {
		t.Fatal("reverting the input should restore unchanged mixed values")
	}
}
