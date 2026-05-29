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
	cp, _ := NewCommandPalette(testCommands(), nil)
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
	// "Disabled cmd" sorts after "Enabled cmd"; cursor on it.
	cp.cursor = 0 // "Disabled cmd" (alphabetically first: D < E)
	if cp.filtered[0].Name != "Disabled cmd" {
		t.Fatalf("expected Disabled cmd at index 0, got %q", cp.filtered[0].Name)
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
	cp, _ := NewCommandPalette(testCommands(), nil)
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
	cp, _ := NewCommandPalette(testCommands(), nil)
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
	cp, _ := NewCommandPalette(testCommands(), nil)
	cp.cursor = 2 // last item
	cp.filter.Text = "alpha"
	cp.refilter()
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

func TestCommandPaletteRenderNoMatches(t *testing.T) {
	cp, _ := NewCommandPalette(testCommands(), nil)
	cp.filter.Text = "zzz"
	cp.refilter()
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

// --- pinned anchors ---

func pinnedCommands() []PaletteCommand {
	return []PaletteCommand{
		{Name: "Help", Pinned: pinTop},
		{Name: "Quit", Pinned: pinBottom},
		{Name: "Alpha action", Category: "play"},
		{Name: "Middle action", Category: "nav"},
		{Name: "Zebra action", Category: "lib"},
	}
}

func TestCommandPaletteTopPinFirst(t *testing.T) {
	cp, _ := NewCommandPalette(pinnedCommands(), nil)
	if cp.filtered[0].Name != "Help" {
		t.Errorf("filtered[0]: want Help (top pin), got %q", cp.filtered[0].Name)
	}
	if cp.filtered[len(cp.filtered)-1].Name != "Quit" {
		t.Errorf("filtered[last]: want Quit (bottom pin), got %q", cp.filtered[len(cp.filtered)-1].Name)
	}
}

func TestCommandPalettePinsRemainAfterFilter(t *testing.T) {
	cp, _ := NewCommandPalette(pinnedCommands(), nil)
	cp.filter.Text = "alpha"
	cp.refilter()
	// Help (top) + Alpha (body match) + Quit (bottom)
	if got := len(cp.filtered); got != 3 {
		t.Fatalf("filtered length: want 3 (pins + 1 body match), got %d", got)
	}
	if cp.filtered[0].Name != "Help" {
		t.Errorf("filtered[0]: want Help, got %q", cp.filtered[0].Name)
	}
	if cp.filtered[1].Name != "Alpha action" {
		t.Errorf("filtered[1]: want Alpha action, got %q", cp.filtered[1].Name)
	}
	if cp.filtered[2].Name != "Quit" {
		t.Errorf("filtered[2]: want Quit, got %q", cp.filtered[2].Name)
	}
}

func TestCommandPalettePinsRemainWhenBodyEmpty(t *testing.T) {
	cp, _ := NewCommandPalette(pinnedCommands(), nil)
	cp.filter.Text = "zzz"
	cp.refilter()
	// Body has no match but pins still appear.
	if got := len(cp.filtered); got != 2 {
		t.Fatalf("filtered length: want 2 (pins only), got %d", got)
	}
	if cp.filtered[0].Name != "Help" {
		t.Errorf("filtered[0]: want Help, got %q", cp.filtered[0].Name)
	}
	if cp.filtered[1].Name != "Quit" {
		t.Errorf("filtered[1]: want Quit, got %q", cp.filtered[1].Name)
	}
}

func TestCommandPaletteRecentHeaderWhenEmpty(t *testing.T) {
	cp, _ := NewCommandPalette(pinnedCommands(), nil)
	s := styles.NewStyles(true)
	out := ansi.Strip(cp.Render(80, 24, s))
	if !strings.Contains(out, "Recent & frequent") {
		t.Error("empty-query render should contain 'Recent & frequent' header")
	}
}

func TestCommandPaletteRecentHeaderHiddenWithQuery(t *testing.T) {
	cp, _ := NewCommandPalette(pinnedCommands(), nil)
	cp.filter.Text = "alpha"
	cp.refilter()
	s := styles.NewStyles(true)
	out := ansi.Strip(cp.Render(80, 24, s))
	if strings.Contains(out, "Recent & frequent") {
		t.Error("non-empty-query render should not contain 'Recent & frequent' header")
	}
}

// Regression: when the body shrinks under a new query, a cursor that sat past
// the last body item must not silently land on (and confirm) a bottom pin like
// Quit.
func TestCommandPaletteQueryChangeResetsCursorOffBottomPin(t *testing.T) {
	cp, _ := NewCommandPalette(pinnedCommands(), nil)
	// Full layout: [Help, Alpha, Middle, Zebra, Quit] -> cursor on Zebra.
	cp.cursor = 3
	if cp.filtered[cp.cursor].Name != "Zebra action" {
		t.Fatalf("setup: want Zebra action at 3, got %q", cp.filtered[cp.cursor].Name)
	}
	// Narrow body to a single match: layout becomes [Help, Alpha, Quit].
	cp.filter.Text = "alpha"
	cp.refilter()
	// Without the reset, cursor=3 would clamp to 2 == Quit.
	if got := cp.filtered[cp.cursor].Name; got == "Quit" {
		t.Fatalf("cursor accidentally landed on bottom pin %q after narrowing query", got)
	}
	if got := cp.filtered[cp.cursor].Name; got != "Alpha action" {
		t.Errorf("cursor: want Alpha action (first body match), got %q", got)
	}
}

// Regression: when the query changes and no body items match, the cursor must
// stay within bounds and not point at a stale bottom-pin index.
func TestCommandPaletteQueryChangeWithEmptyBodyClampsCursor(t *testing.T) {
	cp, _ := NewCommandPalette(pinnedCommands(), nil)
	cp.cursor = 4 // Quit
	cp.filter.Text = "zzz"
	cp.refilter()
	// Layout is [Help, Quit]; cursor must be in range.
	if cp.cursor < 0 || cp.cursor >= len(cp.filtered) {
		t.Fatalf("cursor out of range after empty-body query change: %d / %d", cp.cursor, len(cp.filtered))
	}
	if cp.filtered[cp.cursor].Name != "Help" {
		t.Errorf("cursor: want Help (reset to 0), got %q", cp.filtered[cp.cursor].Name)
	}
}

// Regression: editing the query should not move the cursor between
// keystrokes when the trimmed query is unchanged (e.g. cursor key handlers
// already moved past it). Ensure refilter is idempotent for same-query calls.
func TestCommandPaletteSameQueryDoesNotResetCursor(t *testing.T) {
	cp, _ := NewCommandPalette(testCommands(), nil)
	cp.cursor = 2
	// Manually mutate text but keep it the same trimmed value across calls.
	cp.filter.Text = ""
	cp.refilter() // same query as initial ""
	if cp.cursor != 2 {
		t.Errorf("cursor should be preserved on same-query refilter: want 2, got %d", cp.cursor)
	}
}

func TestCommandPaletteRenderShowsCategoryPrefix(t *testing.T) {
	cp, _ := NewCommandPalette(pinnedCommands(), nil)
	s := styles.NewStyles(true)
	out := ansi.Strip(cp.Render(80, 24, s))
	for _, cat := range []string{"[play]", "[ nav]", "[ lib]"} {
		if !strings.Contains(out, cat) {
			t.Errorf("rendered output should contain category prefix %q", cat)
		}
	}
}
