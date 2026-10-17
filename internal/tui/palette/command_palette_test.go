package palette

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
	cp, _ := NewCommandPalette(testCommands(), nil)
	want := []string{"Alpha action", "Middle action", "Zebra action"}
	if len(cp.Filtered) != len(want) {
		t.Fatalf("filtered length: want %d, got %d", len(want), len(cp.Filtered))
	}
	for i, w := range want {
		if cp.Filtered[i].Name != w {
			t.Errorf("filtered[%d]: want %q, got %q", i, w, cp.Filtered[i].Name)
		}
	}
}

// --- cursor navigation ---

func TestCommandPaletteCursorDown(t *testing.T) {
	cp, _ := NewCommandPalette(testCommands(), nil)
	cp2, cmd := cp.HandleKey("down", tea.KeyPressMsg{})
	if cmd != nil {
		t.Fatalf("expected nil cmd, got %T", cmd)
	}
	if cp2.cursor != 1 {
		t.Errorf("cursor: want 1, got %d", cp2.cursor)
	}
}

func TestCommandPaletteCursorUp(t *testing.T) {
	cp, _ := NewCommandPalette(testCommands(), nil)
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
	cp, _ := NewCommandPalette(testCommands(), nil)
	cp.cursor = 2 // last item
	cp2, _ := cp.HandleKey("down", tea.KeyPressMsg{})
	if cp2.cursor != 2 {
		t.Errorf("cursor: want 2 (clamped), got %d", cp2.cursor)
	}
}

func TestCommandPaletteCursorClampsTop(t *testing.T) {
	cp, _ := NewCommandPalette(testCommands(), nil)
	cp2, _ := cp.HandleKey("up", tea.KeyPressMsg{})
	if cp2.cursor != 0 {
		t.Errorf("cursor: want 0 (clamped), got %d", cp2.cursor)
	}
}

func TestCommandPaletteCtrlJDown(t *testing.T) {
	cp, _ := NewCommandPalette(testCommands(), nil)
	cp2, _ := cp.HandleKey("ctrl+j", tea.KeyPressMsg{})
	if cp2.cursor != 1 {
		t.Errorf("cursor: want 1, got %d", cp2.cursor)
	}
}

func TestCommandPaletteCtrlKUp(t *testing.T) {
	cp, _ := NewCommandPalette(testCommands(), nil)
	cp.cursor = 1
	cp2, _ := cp.HandleKey("ctrl+k", tea.KeyPressMsg{})
	if cp2.cursor != 0 {
		t.Errorf("cursor: want 0, got %d", cp2.cursor)
	}
}

func TestCommandPaletteCtrlNDown(t *testing.T) {
	cp, _ := NewCommandPalette(testCommands(), nil)
	cp2, _ := cp.HandleKey("ctrl+n", tea.KeyPressMsg{})
	if cp2.cursor != 1 {
		t.Errorf("cursor: want 1, got %d", cp2.cursor)
	}
}

func TestCommandPaletteCtrlPUp(t *testing.T) {
	cp, _ := NewCommandPalette(testCommands(), nil)
	cp.cursor = 1
	cp2, _ := cp.HandleKey("ctrl+p", tea.KeyPressMsg{})
	if cp2.cursor != 0 {
		t.Errorf("cursor: want 0, got %d", cp2.cursor)
	}
}

// --- enter confirms ---

func TestCommandPaletteEnterConfirms(t *testing.T) {
	cp, _ := NewCommandPalette(testCommands(), nil)
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

func TestCommandPaletteEnterSkipsDisabledCommand(t *testing.T) {
	cmds := []PaletteCommand{
		{Name: "Enabled cmd"},
		{Name: "Disabled cmd", Enabled: func() bool { return false }},
	}
	cp, _ := NewCommandPalette(cmds, nil)
	// Disabled commands remain discoverable below enabled commands.
	cp.cursor = 1
	if cp.Filtered[1].Name != "Disabled cmd" {
		t.Fatalf("expected Disabled cmd at index 1, got %q", cp.Filtered[1].Name)
	}
	_, cmd := cp.HandleKey("enter", tea.KeyPressMsg{})
	if cmd != nil {
		t.Fatal("expected nil cmd for disabled command, got non-nil")
	}
}

// --- esc dismisses ---

func TestCommandPaletteEscDismisses(t *testing.T) {
	cp, _ := NewCommandPalette(testCommands(), nil)
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
	cp, _ := NewCommandPalette(testCommands(), nil)
	// Manually set the filter text and refilter (since HandleKey delegates to
	// the textinput which requires a real key event).
	cp.Filter.Text = "alpha"
	cp.Refilter()
	if len(cp.Filtered) != 1 {
		t.Fatalf("filtered length: want 1, got %d", len(cp.Filtered))
	}
	if cp.Filtered[0].Name != "Alpha action" {
		t.Errorf("filtered[0]: want Alpha action, got %q", cp.Filtered[0].Name)
	}
}

func TestCommandPaletteFilterCaseInsensitive(t *testing.T) {
	cp, _ := NewCommandPalette(testCommands(), nil)
	cp.Filter.Text = "MIDDLE"
	cp.Refilter()
	if len(cp.Filtered) != 1 {
		t.Fatalf("filtered length: want 1, got %d", len(cp.Filtered))
	}
	if cp.Filtered[0].Name != "Middle action" {
		t.Errorf("filtered[0]: want Middle action, got %q", cp.Filtered[0].Name)
	}
}

func TestCommandPaletteFilterEmptyShowsAll(t *testing.T) {
	cp, _ := NewCommandPalette(testCommands(), nil)
	cp.Filter.Text = "zzz"
	cp.Refilter()
	if len(cp.Filtered) != 0 {
		t.Fatalf("filtered should be empty, got %d", len(cp.Filtered))
	}
	// Clear filter.
	cp.Filter.Text = ""
	cp.Refilter()
	if len(cp.Filtered) != 3 {
		t.Errorf("filtered length: want 3, got %d", len(cp.Filtered))
	}
}

func TestCommandPaletteFilterClampsCursor(t *testing.T) {
	cp, _ := NewCommandPalette(testCommands(), nil)
	cp.cursor = 2 // last item
	cp.Filter.Text = "alpha"
	cp.Refilter()
	if cp.cursor != 0 {
		t.Errorf("cursor: want 0 (clamped), got %d", cp.cursor)
	}
}

// --- render ---

func TestCommandPaletteRenderContainsTitle(t *testing.T) {
	cp, _ := NewCommandPalette(testCommands(), nil)
	s := styles.NewStyles(true)
	out := cp.Render(80, 24, s)
	if !strings.Contains(ansi.Strip(out), "Command Palette") {
		t.Error("rendered output should contain 'Command Palette' title")
	}
}

func TestCommandPaletteRenderContainsCommands(t *testing.T) {
	cp, _ := NewCommandPalette(testCommands(), nil)
	s := styles.NewStyles(true)
	out := cp.Render(80, 24, s)
	for _, name := range []string{"Alpha action", "Middle action", "Zebra action"} {
		if !strings.Contains(out, name) {
			t.Errorf("rendered output should contain %q", name)
		}
	}
}

// boxWidth is the display width of the palette's top border, i.e. the width of
// the whole box.
func boxWidth(t *testing.T, out string) int {
	t.Helper()
	for _, line := range strings.Split(out, "\n") {
		plain := strings.TrimLeft(ansi.Strip(line), " ")
		if strings.HasPrefix(plain, cpCornerTL) {
			return ansi.StringWidth(plain)
		}
	}
	t.Fatalf("no top border in render:\n%s", out)
	return 0
}

// TestCommandPaletteRenderWidthIsStableAcrossFilters pins the width to the full
// command list: narrowing the results to one short name must not shrink the box
// out from under the user as they type.
func TestCommandPaletteRenderWidthIsStableAcrossFilters(t *testing.T) {
	s := styles.NewStyles(true)
	cp, _ := NewCommandPalette([]PaletteCommand{
		{Name: "A really quite long command name", Category: "library", Shortcut: "ctrl+shift+x"},
		{Name: "Tiny"},
	}, nil)
	want := boxWidth(t, cp.Render(120, 24, s))

	for _, query := range []string{"tiny", "zzz"} {
		cp.Filter.Text = query
		cp.Refilter()
		if got := boxWidth(t, cp.Render(120, 24, s)); got != want {
			t.Errorf("box width with filter %q = %d, want %d (unfiltered width)", query, got, want)
		}
	}
}

// TestCommandPaletteRenderAnchorsToTop pins the vertical placement: the box
// starts on the first line so a shrinking result list only changes its height,
// never its position.
func TestCommandPaletteRenderAnchorsToTop(t *testing.T) {
	s := styles.NewStyles(true)
	cp, _ := NewCommandPalette(testCommands(), nil)

	for _, query := range []string{"", "alpha"} {
		cp.Filter.Text = query
		cp.Refilter()
		first := ansi.Strip(strings.Split(cp.Render(80, 40, s), "\n")[0])
		if !strings.HasPrefix(strings.TrimLeft(first, " "), cpCornerTL) {
			t.Errorf("first rendered line with filter %q = %q, want the top border", query, first)
		}
	}
}

func TestCommandPaletteRenderNoMatches(t *testing.T) {
	cp, _ := NewCommandPalette(testCommands(), nil)
	cp.Filter.Text = "zzz"
	cp.Refilter()
	s := styles.NewStyles(true)
	out := cp.Render(80, 24, s)
	if !strings.Contains(out, "(no matches)") {
		t.Error("rendered output should contain '(no matches)' when filter has no results")
	}
}

func TestCommandPaletteRenderTinyWidthKeepsBordersAligned(t *testing.T) {
	cp, _ := NewCommandPalette(testCommands(), nil)
	s := styles.NewStyles(true)
	lines := strings.Split(cp.Render(8, 12, s), "\n")

	want := 0
	for i, line := range lines {
		plain := strings.TrimLeft(ansi.Strip(line), " ")
		if !strings.HasPrefix(plain, cpCornerTL) &&
			!strings.HasPrefix(plain, cpCornerBL) &&
			!strings.HasPrefix(plain, cpCrossL) &&
			!strings.HasPrefix(plain, cpBorderV) {
			continue
		}
		width := ansi.StringWidth(line)
		if want == 0 {
			want = width
			continue
		}
		if width != want {
			t.Fatalf("bordered line %d width = %d, want %d\nline: %q", i, width, want, line)
		}
	}
	if want == 0 {
		t.Fatal("expected bordered lines in command palette render")
	}
}

func TestCommandPaletteRenderTinyWidthWithCategoriesKeepsBordersAligned(t *testing.T) {
	cp, _ := NewCommandPalette(pinnedCommands(), nil)
	s := styles.NewStyles(true)
	lines := strings.Split(cp.Render(8, 12, s), "\n")

	want := 0
	for i, line := range lines {
		plain := strings.TrimLeft(ansi.Strip(line), " ")
		if !strings.HasPrefix(plain, cpCornerTL) &&
			!strings.HasPrefix(plain, cpCornerBL) &&
			!strings.HasPrefix(plain, cpCrossL) &&
			!strings.HasPrefix(plain, cpBorderV) {
			continue
		}
		width := ansi.StringWidth(line)
		if want == 0 {
			want = width
			continue
		}
		if width != want {
			t.Fatalf("bordered line %d width = %d, want %d\nline: %q", i, width, want, line)
		}
	}
	if want == 0 {
		t.Fatal("expected bordered lines in command palette render")
	}
}

// --- pinned anchors ---
