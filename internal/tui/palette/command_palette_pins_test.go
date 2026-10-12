package palette

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/tui/styles"
)

func pinnedCommands() []PaletteCommand {
	return []PaletteCommand{
		{Name: "Help", Pinned: PinTop},
		{Name: "Quit", Pinned: PinBottom},
		{Name: "Alpha action", Category: "play"},
		{Name: "Middle action", Category: "nav"},
		{Name: "Zebra action", Category: "lib"},
	}
}

func TestCommandPaletteTopPinFirst(t *testing.T) {
	cp, _ := NewCommandPalette(pinnedCommands(), nil)
	if cp.Filtered[0].Name != "Help" {
		t.Errorf("filtered[0]: want Help (top pin), got %q", cp.Filtered[0].Name)
	}
	if cp.Filtered[len(cp.Filtered)-1].Name != "Quit" {
		t.Errorf("filtered[last]: want Quit (bottom pin), got %q", cp.Filtered[len(cp.Filtered)-1].Name)
	}
}

func TestCommandPalettePinsRemainAfterFilter(t *testing.T) {
	cp, _ := NewCommandPalette(pinnedCommands(), nil)
	cp.Filter.Text = "alpha"
	cp.Refilter()
	// Help (top) + Alpha (body match) + Quit (bottom)
	if got := len(cp.Filtered); got != 3 {
		t.Fatalf("filtered length: want 3 (pins + 1 body match), got %d", got)
	}
	if cp.Filtered[0].Name != "Help" {
		t.Errorf("filtered[0]: want Help, got %q", cp.Filtered[0].Name)
	}
	if cp.Filtered[1].Name != "Alpha action" {
		t.Errorf("filtered[1]: want Alpha action, got %q", cp.Filtered[1].Name)
	}
	if cp.Filtered[2].Name != "Quit" {
		t.Errorf("filtered[2]: want Quit, got %q", cp.Filtered[2].Name)
	}
}

func TestCommandPalettePinsRemainWhenBodyEmpty(t *testing.T) {
	cp, _ := NewCommandPalette(pinnedCommands(), nil)
	cp.Filter.Text = "zzz"
	cp.Refilter()
	// Body has no match but pins still appear.
	if got := len(cp.Filtered); got != 2 {
		t.Fatalf("filtered length: want 2 (pins only), got %d", got)
	}
	if cp.Filtered[0].Name != "Help" {
		t.Errorf("filtered[0]: want Help, got %q", cp.Filtered[0].Name)
	}
	if cp.Filtered[1].Name != "Quit" {
		t.Errorf("filtered[1]: want Quit, got %q", cp.Filtered[1].Name)
	}
}

func TestCommandPaletteRecentHeaderWhenEmpty(t *testing.T) {
	usage := NewUsageStore("")
	usage.Bump(pinnedCommands()[2].Key())
	cp, _ := NewCommandPalette(pinnedCommands(), usage)
	s := styles.NewStyles(true)
	out := ansi.Strip(cp.Render(80, 24, s))
	if !strings.Contains(out, "Recent & frequent") {
		t.Error("empty-query render should contain 'Recent & frequent' header")
	}
}

// TestCommandPaletteRecentHeaderHiddenWithoutHistory guards that an empty
// query does not label the body "Recent & frequent" before anything has been
// used, when the body is simply alphabetical.
func TestCommandPaletteRecentHeaderHiddenWithoutHistory(t *testing.T) {
	s := styles.NewStyles(true)
	for name, usage := range map[string]*UsageStore{"nil store": nil, "empty store": NewUsageStore("")} {
		cp, _ := NewCommandPalette(pinnedCommands(), usage)
		if out := ansi.Strip(cp.Render(80, 24, s)); strings.Contains(out, "Recent & frequent") {
			t.Errorf("%s: header shown with no usage history:\n%s", name, out)
		}
	}
}

func TestCommandPaletteRecentHeaderHiddenWithQuery(t *testing.T) {
	cp, _ := NewCommandPalette(pinnedCommands(), nil)
	cp.Filter.Text = "alpha"
	cp.Refilter()
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
	if cp.Filtered[cp.cursor].Name != "Zebra action" {
		t.Fatalf("setup: want Zebra action at 3, got %q", cp.Filtered[cp.cursor].Name)
	}
	// Narrow body to a single match: layout becomes [Help, Alpha, Quit].
	cp.Filter.Text = "alpha"
	cp.Refilter()
	// Without the reset, cursor=3 would clamp to 2 == Quit.
	if got := cp.Filtered[cp.cursor].Name; got == "Quit" {
		t.Fatalf("cursor accidentally landed on bottom pin %q after narrowing query", got)
	}
	if got := cp.Filtered[cp.cursor].Name; got != "Alpha action" {
		t.Errorf("cursor: want Alpha action (first body match), got %q", got)
	}
}

// Regression: when the query changes and no body items match, the cursor must
// stay within bounds and not point at a stale bottom-pin index.
func TestCommandPaletteQueryChangeWithEmptyBodyClampsCursor(t *testing.T) {
	cp, _ := NewCommandPalette(pinnedCommands(), nil)
	cp.cursor = 4 // Quit
	cp.Filter.Text = "zzz"
	cp.Refilter()
	// Layout is [Help, Quit]; cursor must be in range.
	if cp.cursor < 0 || cp.cursor >= len(cp.Filtered) {
		t.Fatalf("cursor out of range after empty-body query change: %d / %d", cp.cursor, len(cp.Filtered))
	}
	if cp.Filtered[cp.cursor].Name != "Help" {
		t.Errorf("cursor: want Help (reset to 0), got %q", cp.Filtered[cp.cursor].Name)
	}
}

// Regression: editing the query should not move the cursor between
// keystrokes when the trimmed query is unchanged (e.g. cursor key handlers
// already moved past it). Ensure refilter is idempotent for same-query calls.
func TestCommandPaletteSameQueryDoesNotResetCursor(t *testing.T) {
	cp, _ := NewCommandPalette(testCommands(), nil)
	cp.cursor = 2
	// Manually mutate text but keep it the same trimmed value across calls.
	cp.Filter.Text = ""
	cp.Refilter() // same query as initial ""
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

func TestDisabledCommandExplanationFollowsFocus(t *testing.T) {
	enabled := false
	cp, _ := NewCommandPalette([]PaletteCommand{
		{Name: "Clear queue", Enabled: func() bool { return enabled }, DisabledReason: "Queue is empty"},
		{Name: "Show help"},
	}, nil)
	s := styles.NewStyles(true)
	for _, size := range [][2]int{{60, 12}, {100, 27}} {
		out := ansi.Strip(cp.Render(size[0], size[1], s))
		if !strings.Contains(out, "Queue is empty") {
			t.Fatalf("disabled reason missing at %v: %s", size, out)
		}
		lines := strings.Split(out, "\n")
		if len(lines) > size[1] {
			t.Fatalf("reason increased dialog height beyond %v", size)
		}
		for _, line := range lines {
			if ansi.StringWidth(line) > size[0] {
				t.Fatalf("reason overflow at %v: %q", size, line)
			}
		}
		if cmd := cp.runFocused(); cmd != nil {
			t.Fatal("disabled command executed")
		}
		moved := cp.nextCommand()
		if strings.Contains(ansi.Strip(moved.Render(size[0], size[1], s)), "Queue is empty") {
			t.Fatal("stale reason shown for enabled command")
		}
	}
	enabled = true
	if strings.Contains(ansi.Strip(cp.Render(60, 12, s)), "Queue is empty") {
		t.Fatal("reason shown after command became enabled")
	}
	if cp.runFocused() == nil {
		t.Fatal("enabled command did not execute")
	}
}
