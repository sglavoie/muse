package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/tui/styles"
)

func testCommands() []PaletteCommand {
	return []PaletteCommand{
		{Name: "Zebra action"},
		{Name: "Alpha action"},
		{Name: "Middle action"},
	}
}

// --- sorting ---

func TestNewCommandPaletteSortsAlphabetically(t *testing.T) {
	cp, _ := NewCommandPalette(testCommands())
	want := []string{"Alpha action", "Middle action", "Zebra action"}
	if len(cp.filtered) != len(want) {
		t.Fatalf("filtered length: want %d, got %d", len(want), len(cp.filtered))
	}
	for i, w := range want {
		if cp.filtered[i].Name != w {
			t.Errorf("filtered[%d]: want %q, got %q", i, w, cp.filtered[i].Name)
		}
	}
}

// --- cursor navigation ---

func TestCommandPaletteCursorDown(t *testing.T) {
	cp, _ := NewCommandPalette(testCommands())
	cp2, cmd := cp.HandleKey("down", tea.KeyPressMsg{})
	if cmd != nil {
		t.Fatalf("expected nil cmd, got %T", cmd)
	}
	if cp2.cursor != 1 {
		t.Errorf("cursor: want 1, got %d", cp2.cursor)
	}
}

func TestCommandPaletteCursorUp(t *testing.T) {
	cp, _ := NewCommandPalette(testCommands())
	cp.cursor = 2
	cp2, cmd := cp.HandleKey("up", tea.KeyPressMsg{})
	if cmd != nil {
		t.Fatalf("expected nil cmd, got %T", cmd)
	}
	if cp2.cursor != 1 {
		t.Errorf("cursor: want 1, got %d", cp2.cursor)
	}
}

func TestCommandPaletteCursorClampsBottom(t *testing.T) {
	cp, _ := NewCommandPalette(testCommands())
	cp.cursor = 2 // last item
	cp2, _ := cp.HandleKey("down", tea.KeyPressMsg{})
	if cp2.cursor != 2 {
		t.Errorf("cursor: want 2 (clamped), got %d", cp2.cursor)
	}
}

func TestCommandPaletteCursorClampsTop(t *testing.T) {
	cp, _ := NewCommandPalette(testCommands())
	cp2, _ := cp.HandleKey("up", tea.KeyPressMsg{})
	if cp2.cursor != 0 {
		t.Errorf("cursor: want 0 (clamped), got %d", cp2.cursor)
	}
}

func TestCommandPaletteCtrlJDown(t *testing.T) {
	cp, _ := NewCommandPalette(testCommands())
	cp2, _ := cp.HandleKey("ctrl+j", tea.KeyPressMsg{})
	if cp2.cursor != 1 {
		t.Errorf("cursor: want 1, got %d", cp2.cursor)
	}
}

func TestCommandPaletteCtrlKUp(t *testing.T) {
	cp, _ := NewCommandPalette(testCommands())
	cp.cursor = 1
	cp2, _ := cp.HandleKey("ctrl+k", tea.KeyPressMsg{})
	if cp2.cursor != 0 {
		t.Errorf("cursor: want 0, got %d", cp2.cursor)
	}
}

func TestCommandPaletteCtrlNDown(t *testing.T) {
	cp, _ := NewCommandPalette(testCommands())
	cp2, _ := cp.HandleKey("ctrl+n", tea.KeyPressMsg{})
	if cp2.cursor != 1 {
		t.Errorf("cursor: want 1, got %d", cp2.cursor)
	}
}

func TestCommandPaletteCtrlPUp(t *testing.T) {
	cp, _ := NewCommandPalette(testCommands())
	cp.cursor = 1
	cp2, _ := cp.HandleKey("ctrl+p", tea.KeyPressMsg{})
	if cp2.cursor != 0 {
		t.Errorf("cursor: want 0, got %d", cp2.cursor)
	}
}

// --- enter confirms ---

func TestCommandPaletteEnterConfirms(t *testing.T) {
	cp, _ := NewCommandPalette(testCommands())
	cp.cursor = 1 // "Middle action" (sorted)
	_, cmd := cp.HandleKey("enter", tea.KeyPressMsg{})
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}
	msg := cmd()
	picked, ok := msg.(CommandPalettePickedMsg)
	if !ok {
		t.Fatalf("expected CommandPalettePickedMsg, got %T", msg)
	}
	if picked.Command.Name != "Middle action" {
		t.Errorf("expected Middle action, got %q", picked.Command.Name)
	}
}

// --- esc dismisses ---

func TestCommandPaletteEscDismisses(t *testing.T) {
	cp, _ := NewCommandPalette(testCommands())
	_, cmd := cp.HandleKey("esc", tea.KeyPressMsg{})
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}
	msg := cmd()
	if _, ok := msg.(CommandPaletteDismissedMsg); !ok {
		t.Fatalf("expected CommandPaletteDismissedMsg, got %T", msg)
	}
}

// --- filtering ---

func TestCommandPaletteFilterNarrows(t *testing.T) {
	cp, _ := NewCommandPalette(testCommands())
	// Manually set the filter text and refilter (since HandleKey delegates to
	// the textinput which requires a real key event).
	cp.filter.Text = "alpha"
	cp.refilter()
	if len(cp.filtered) != 1 {
		t.Fatalf("filtered length: want 1, got %d", len(cp.filtered))
	}
	if cp.filtered[0].Name != "Alpha action" {
		t.Errorf("filtered[0]: want Alpha action, got %q", cp.filtered[0].Name)
	}
}

func TestCommandPaletteFilterCaseInsensitive(t *testing.T) {
	cp, _ := NewCommandPalette(testCommands())
	cp.filter.Text = "MIDDLE"
	cp.refilter()
	if len(cp.filtered) != 1 {
		t.Fatalf("filtered length: want 1, got %d", len(cp.filtered))
	}
	if cp.filtered[0].Name != "Middle action" {
		t.Errorf("filtered[0]: want Middle action, got %q", cp.filtered[0].Name)
	}
}

func TestCommandPaletteFilterEmptyShowsAll(t *testing.T) {
	cp, _ := NewCommandPalette(testCommands())
	cp.filter.Text = "zzz"
	cp.refilter()
	if len(cp.filtered) != 0 {
		t.Fatalf("filtered should be empty, got %d", len(cp.filtered))
	}
	// Clear filter.
	cp.filter.Text = ""
	cp.refilter()
	if len(cp.filtered) != 3 {
		t.Errorf("filtered length: want 3, got %d", len(cp.filtered))
	}
}

func TestCommandPaletteFilterClampsCursor(t *testing.T) {
	cp, _ := NewCommandPalette(testCommands())
	cp.cursor = 2 // last item
	cp.filter.Text = "alpha"
	cp.refilter()
	if cp.cursor != 0 {
		t.Errorf("cursor: want 0 (clamped), got %d", cp.cursor)
	}
}

// --- render ---

func TestCommandPaletteRenderContainsTitle(t *testing.T) {
	cp, _ := NewCommandPalette(testCommands())
	s := styles.NewStyles(true)
	out := cp.Render(80, 24, s)
	if !strings.Contains(ansi.Strip(out), "Command Palette") {
		t.Error("rendered output should contain 'Command Palette' title")
	}
}

func TestCommandPaletteRenderContainsCommands(t *testing.T) {
	cp, _ := NewCommandPalette(testCommands())
	s := styles.NewStyles(true)
	out := cp.Render(80, 24, s)
	for _, name := range []string{"Alpha action", "Middle action", "Zebra action"} {
		if !strings.Contains(out, name) {
			t.Errorf("rendered output should contain %q", name)
		}
	}
}

func TestCommandPaletteRenderNoMatches(t *testing.T) {
	cp, _ := NewCommandPalette(testCommands())
	cp.filter.Text = "zzz"
	cp.refilter()
	s := styles.NewStyles(true)
	out := cp.Render(80, 24, s)
	if !strings.Contains(out, "(no matches)") {
		t.Error("rendered output should contain '(no matches)' when filter has no results")
	}
}
