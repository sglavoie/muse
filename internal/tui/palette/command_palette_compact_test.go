package palette

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/tui/styles"
)

func TestCompactPaletteSearchShowsSeveralResultsAndPages(t *testing.T) {
	commands := pinnedCommands()
	for i := range 12 {
		commands = append(commands, PaletteCommand{Name: fmt.Sprintf("Sort field %02d", i)})
	}
	cp, _ := NewCommandPalette(commands, nil)
	cp.Filter.Text = "sort"
	cp.Refilter()
	out := ansi.Strip(cp.Render(60, 12, styles.NewStyles(true)))
	if strings.Count(out, "Sort field") < 4 {
		t.Fatalf("compact search must show at least four results:\n%s", out)
	}
	if strings.Contains(out, "Help") || strings.Contains(out, "Quit") {
		t.Fatal("unmatched pins crowded search results")
	}
	cp, _ = cp.HandleKey("pgdown", tea.KeyPressMsg{}, 12)
	if cp.cursor != 6 {
		t.Fatalf("page down focused %d, want next six-row page", cp.cursor)
	}
	out = ansi.Strip(cp.Render(60, 12, styles.NewStyles(true)))
	if !strings.Contains(out, "7/12") || !strings.Contains(out, cp.Filtered[cp.cursor].Name) {
		t.Fatal(out)
	}
	cp, _ = cp.HandleKey("pgup", tea.KeyPressMsg{}, 12)
	if cp.cursor != 0 {
		t.Fatal("page up did not return to first result")
	}
	for range 10 {
		cp, _ = cp.HandleKey("pgdown", tea.KeyPressMsg{}, 12)
	}
	if cp.cursor != 11 {
		t.Fatal("page down must clamp to last result")
	}
	// Resizing changes the page size to match the visible results.
	cp, _ = cp.HandleKey("pgup", tea.KeyPressMsg{}, 24)
	if cp.cursor != 0 {
		t.Fatal("page navigation did not use the resized viewport")
	}
}
