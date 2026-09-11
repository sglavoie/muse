package autodj

import (
	"strings"
	"testing"

	"github.com/sglavoie/muse/internal/tui/styles"
)

// renderRecipes draws the Recipes page at the given terminal size.
func renderRecipes(t *testing.T, p TunePanel, w, h int) string {
	t.Helper()
	return p.Render(w, h, styles.NewStyles(true), nil, Preview{})
}

func TestRecipesPageFitsTheSmallestSupportedTerminal(t *testing.T) {
	out := renderRecipes(t, recipesPanel(t, true), minWidth, minOverlayHeight)
	assertFitsBox(t, out, minWidth, minOverlayHeight)
	if !strings.Contains(plain(out), "Recipes") {
		t.Fatalf("the tab bar does not mark the page:\n%s", out)
	}
}

// A long list on a short terminal has to keep the focused recipe on screen,
// because the keys that act on it all address whatever the cursor is on.
func TestTheWindowFollowsTheCursorThroughALongList(t *testing.T) {
	p := recipesPanel(t, true)
	many := make([]Recipe, 0, 30)
	for i := range 30 {
		many = append(many, Recipe{ID: int64(i + 10), Name: "Recipe " + string(rune('A'+i%26))})
	}
	p.recipes = many

	p, _ = press(t, p, "up") // wrap to the last row, the furthest from the top
	out := plain(renderRecipes(t, p, 80, 12))
	if !strings.Contains(out, "▸ "+many[len(many)-1].Name) {
		t.Fatalf("the focused recipe scrolled off the page:\n%s", out)
	}
}

func TestAnEmptyListSaysHowToStartOne(t *testing.T) {
	p := New(Options{Applied: testSettings(), Catalog: testCatalog()})
	p, _ = press(t, p, "tab", "tab")
	if !strings.Contains(plain(renderRecipes(t, p, 80, 24)), "No saved recipes yet") {
		t.Fatalf("an empty page does not say what to do:\n%s", renderRecipes(t, p, 80, 24))
	}
}

// The name box is modal: it takes the whole overlay so the arrow keys cannot
// appear to still be moving a cursor the box has taken over.
func TestThePromptTakesTheWholeOverlay(t *testing.T) {
	p := recipesPanel(t, true)
	p, _ = press(t, p, "s")
	out := plain(renderRecipes(t, p, 80, 24))

	if !strings.Contains(out, "Save recipe as") {
		t.Fatalf("the prompt does not name itself:\n%s", out)
	}
	if strings.Contains(out, "Late Night") {
		t.Fatalf("the list is still visible behind the modal prompt:\n%s", out)
	}
	if !strings.Contains(out, "type a name…") {
		t.Fatalf("the empty box gives no placeholder:\n%s", out)
	}
}

func TestThePromptEchoesWhatWasTyped(t *testing.T) {
	p := recipesPanel(t, true)
	p, _ = press(t, p, "n")
	p, _ = press(t, p, "ctrl+u")
	p = typeName(t, p, "New Name")

	out := plain(renderRecipes(t, p, 80, 24))
	if !strings.Contains(out, "Rename recipe") {
		t.Fatalf("the rename box does not name itself:\n%s", out)
	}
	if !strings.Contains(out, "New Name") {
		t.Fatalf("the box does not echo the typed name:\n%s", out)
	}
}

// Backspace removes one character, including a multi-byte one.
func TestBackspaceDeletesOneRune(t *testing.T) {
	p := recipesPanel(t, true)
	p, _ = press(t, p, "s")
	p = typeName(t, p, "é")
	p, _ = press(t, p, "backspace")
	if p.prompt.value != "" {
		t.Fatalf("backspace left %q", p.prompt.value)
	}
}

// Each page advertises the keys it actually has, and the tab hint points at
// where tab now goes.
func TestEachPageAdvertisesItsOwnKeys(t *testing.T) {
	p := recipesPanel(t, true)
	if hint := plain(renderRecipes(t, p, 100, 30)); !strings.Contains(hint, "s save as") ||
		!strings.Contains(hint, "d delete") {
		t.Fatalf("the Recipes hint reads:\n%s", hint)
	}
	if hint := previewHint(false, "Apply"); !strings.Contains(hint, "tab recipes") {
		t.Fatalf("the Preview hint still points at Tune: %q", hint)
	}
}
