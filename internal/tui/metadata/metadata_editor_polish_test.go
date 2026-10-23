package metadata

import (
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"strings"
	"testing"
)

func TestMetadataDraftPreviewAndSaveAlias(t *testing.T) {
	title := "Opening words of a long title whose ending still needs to remain visible while editing"
	ed := NewEditor("h", title, "Artist", "Album", "Jazz", 90)
	ed, _, _ = ed.HandleKey(tea.KeyPressMsg{Code: '!', Text: "!"})
	out := ansi.Strip(ed.Render(80, 21, newTestStyles()))
	for _, want := range []string{"Draft:", "Opening words", "editing!", "Updates muse only; audio files unchanged.", "ctrl+s/enter save"} {
		if !strings.Contains(out, want) {
			t.Fatal(out)
		}
	}
	for _, size := range [][2]int{{60, 12}, {80, 21}, {100, 27}} {
		out = ansi.Strip(ed.Render(size[0], size[1], newTestStyles()))
		assertEditorFits(t, out, size[0], size[1])
		if !strings.Contains(out, "files unchanged") || !strings.Contains(out, "ctrl+s/enter save") {
			t.Fatal(out)
		}
	}
	for _, key := range []tea.KeyPressMsg{{Code: tea.KeyEnter}, {Code: 's', Mod: tea.ModCtrl}} {
		next, action, _ := ed.HandleKey(key)
		if action != ActionSave || next.Fields[fieldTitle].Value() != title+"!" {
			t.Fatal("save changed draft or failed to submit")
		}
	}
}
