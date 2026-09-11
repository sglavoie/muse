package autodj

import (
	"errors"
	"strings"
	"testing"

	"github.com/sglavoie/muse/internal/radio"
)

// testRecipes is the stored list every test on this page starts from. The third
// entry is a row this build could not read, which is the case the tab has to
// keep listable without ever letting it reach the draft.
func testRecipes() []Recipe {
	return []Recipe{
		{ID: 1, Name: "Late Night", Settings: radio.Settings{
			DiscoveryPercent: 80, Anchor: radio.AnchorFollow,
			Include: radio.FilterSet{Genres: []string{"jazz"}},
		}},
		{ID: 2, Name: "Deep Focus", Settings: radio.Settings{
			DiscoveryPercent: 20, Anchor: radio.AnchorSeed,
			Exclude: radio.FilterSet{TagIDs: []int64{7}},
		}},
		{ID: 3, Name: "Broken", Invalid: errors.New("unsupported schema version")},
	}
}

// recipesPanel opens the panel on the Recipes tab with the fixture list.
func recipesPanel(t *testing.T, active bool) TunePanel {
	t.Helper()
	p := New(Options{
		Applied: testSettings(), Catalog: testCatalog(),
		Active: active, Recipes: testRecipes(),
	})
	p, _ = press(t, p, "tab", "tab")
	if !p.RecipesActive() {
		t.Fatal("two tabs from Tune is not the Recipes page")
	}
	return p
}

// typeName feeds a name into the open prompt one key at a time.
func typeName(t *testing.T, p TunePanel, name string) TunePanel {
	t.Helper()
	if p.prompt == nil {
		t.Fatal("no prompt is open")
	}
	for _, r := range name {
		key := string(r)
		if r == ' ' {
			key = "space"
		}
		p, _ = press(t, p, key)
	}
	return p
}

// recipesPage renders the Recipes page as plain text.
func recipesPage(p TunePanel) string {
	lines, _ := p.recipeLines()
	var b strings.Builder
	for _, l := range lines {
		b.WriteString(strings.Repeat(" ", l.indent))
		b.WriteString(l.text)
		b.WriteString("\n")
	}
	return b.String()
}

// The tab bar is the only way to the page, and the page must be reachable in
// both directions so a user who overshoots can come back.
func TestRecipesIsTheThirdTabAndWrapsBothWays(t *testing.T) {
	p := New(Options{Applied: testSettings(), Catalog: testCatalog(), Recipes: testRecipes()})
	p, _ = press(t, p, "shift+tab")
	if !p.RecipesActive() {
		t.Fatal("shift+tab from Tune does not reach Recipes")
	}
	p, _ = press(t, p, "tab")
	if p.tab != tabTune {
		t.Fatalf("tab from Recipes landed on %d, want Tune", p.tab)
	}
}

// Loading is the gesture the completion condition is built around: it edits the
// draft and nothing else, so it produces no request for the root model at all.
func TestLoadingARecipeEditsTheDraftAndAsksForNothing(t *testing.T) {
	p := recipesPanel(t, true)
	before := p.AppliedSettings()

	p, action := press(t, p, "enter")
	if action != actionNone {
		t.Fatalf("loading asked the root model for %v; it must write nothing", action)
	}
	if got := p.Draft().DiscoveryPercent; got != 80 {
		t.Fatalf("draft discovery = %d after loading Late Night, want 80", got)
	}
	if !p.AppliedSettings().Equal(before) {
		t.Fatal("loading moved the applied snapshot; only a commit may do that")
	}
	if !p.Dirty() {
		t.Fatal("a loaded recipe is an unapplied edit and must read as dirty")
	}
	if !strings.Contains(recipesPage(p), "Loaded “Late Night”") {
		t.Fatalf("the page does not say what was loaded:\n%s", recipesPage(p))
	}
}

// The draft is a copy. A recipe loaded and then edited must not write back
// through the shared slice into the list the page is still showing.
func TestLoadingCopiesTheStoredSettings(t *testing.T) {
	p := recipesPanel(t, true)
	p, _ = press(t, p, "enter")
	p.draft.Include.Genres[0] = "polka"
	if got := p.recipes[0].Settings.Include.Genres[0]; got != "jazz" {
		t.Fatalf("editing the draft rewrote the stored recipe: %q", got)
	}
}

// A recipe holding a tag the catalog no longer offers is the "preserve
// unresolved references and support repair before activation" case: the
// reference survives the load, the panel refuses to commit it, and r repairs it.
func TestLoadingPreservesUnresolvedTagsAndBlocksUntilRepaired(t *testing.T) {
	p := recipesPanel(t, true)
	p, _ = press(t, p, "down", "enter")

	if got := p.Draft().Exclude.TagIDs; len(got) != 1 || got[0] != 7 {
		t.Fatalf("draft exclusions = %v, want the stored [7] kept verbatim", got)
	}
	if missing := p.Unresolved(); len(missing) != 1 || missing[0] != 7 {
		t.Fatalf("unresolved = %v, want [7]", missing)
	}
	if _, action := press(t, p, "ctrl+s"); action != actionNone {
		t.Fatal("a draft with an unresolved tag was allowed to commit")
	}

	p, _ = press(t, p, "shift+tab", "shift+tab") // back to Tune, where r repairs
	p, _ = press(t, p, "r")
	if missing := p.Unresolved(); len(missing) != 0 {
		t.Fatalf("r left %v unresolved", missing)
	}
	if _, action := press(t, p, "ctrl+s"); action != ActionCommit {
		t.Fatal("the repaired draft still will not commit")
	}
}

// The page warns about a stale reference before the recipe is ever loaded, so
// the repair is a decision rather than a surprise.
func TestThePageNamesUnresolvedReferencesBeforeLoading(t *testing.T) {
	page := recipesPage(recipesPanel(t, true))
	if !strings.Contains(page, "references 1 tag that no longer exists") {
		t.Fatalf("the page does not warn about the stale tag:\n%s", page)
	}
}

// An unreadable row holds zero settings, so loading it would replace the user's
// filters with none and renaming it would write that emptiness back.
func TestAnUnreadableRecipeCanOnlyBeDeleted(t *testing.T) {
	p := recipesPanel(t, true)
	p, _ = press(t, p, "down", "down")

	for _, key := range []string{"enter", "u", "n"} {
		before := p.Draft()
		next, action := press(t, p, key)
		if action != actionNone {
			t.Fatalf("%q on an unreadable recipe asked for %v", key, action)
		}
		if !next.Draft().Equal(before) {
			t.Fatalf("%q on an unreadable recipe changed the draft", key)
		}
		if !strings.Contains(next.recipeNote.Text, "could not be read") {
			t.Fatalf("%q was refused without saying why: %q", key, next.recipeNote.Text)
		}
	}

	_, action := press(t, p, "d")
	if action != ActionRecipe {
		t.Fatalf("deleting an unreadable recipe asked for %v, want ActionRecipe", action)
	}
}

// Saving stores the draft under a new name and touches nothing else.
func TestSavingSendsTheDraftUnderTheTypedName(t *testing.T) {
	p := recipesPanel(t, true)
	p, _ = press(t, p, "shift+tab", "shift+tab") // Tune
	p, _ = press(t, p, "down", "right")          // move discovery off the applied value
	draft := p.Draft()
	p, _ = press(t, p, "tab", "tab") // back to Recipes

	p, _ = press(t, p, "s")
	p = typeName(t, p, "Sunday Morning")
	p, action := press(t, p, "enter")

	if action != ActionRecipe {
		t.Fatalf("confirming the name asked for %v", action)
	}
	req := p.PendingRecipe()
	if req.Op != RecipeSave || req.ID != 0 {
		t.Fatalf("request = %+v, want a save with no id", req)
	}
	if req.Name != "Sunday Morning" {
		t.Fatalf("name = %q — the space bar did not reach the prompt", req.Name)
	}
	if !req.Settings.Equal(draft) {
		t.Fatalf("saved settings = %+v, want the draft %+v", req.Settings, draft)
	}
	if p.prompt != nil {
		t.Fatal("the prompt is still open after a confirmed name")
	}
}

// A rename must resend the recipe's own settings: storage rewrites the whole
// row, so sending the draft here would silently re-save a recipe the user only
// meant to relabel.
func TestRenamingResendsTheStoredSettingsNotTheDraft(t *testing.T) {
	p := recipesPanel(t, true)
	p, _ = press(t, p, "n")
	if got := p.prompt.value; got != "Late Night" {
		t.Fatalf("the rename box opened on %q, want the current name", got)
	}
	p, _ = press(t, p, "ctrl+u")
	p = typeName(t, p, "Very Late")
	p, action := press(t, p, "enter")

	if action != ActionRecipe {
		t.Fatalf("confirming the rename asked for %v", action)
	}
	req := p.PendingRecipe()
	if req.Op != RecipeRename || req.ID != 1 || req.Name != "Very Late" {
		t.Fatalf("request = %+v", req)
	}
	if !req.Settings.Equal(testRecipes()[0].Settings) {
		t.Fatalf("rename carried %+v, want the recipe's stored settings", req.Settings)
	}
}

// Updating is the only overwrite the tab offers, and it names its target.
func TestUpdatingOverwritesTheFocusedRecipeWithTheDraft(t *testing.T) {
	p := recipesPanel(t, true)
	p, action := press(t, p, "u")
	if action != ActionRecipe {
		t.Fatalf("u asked for %v", action)
	}
	req := p.PendingRecipe()
	if req.Op != RecipeUpdate || req.ID != 1 || req.Name != "Late Night" {
		t.Fatalf("request = %+v", req)
	}
	if !req.Settings.Equal(p.Draft()) {
		t.Fatal("an update must carry the draft")
	}
}

// A name that already belongs to another recipe is refused where the panel can
// see it, without a round trip and without overwriting anything.
func TestSavingOverAnExistingNameIsRefusedVisibly(t *testing.T) {
	p := recipesPanel(t, true)
	p, _ = press(t, p, "s")
	p = typeName(t, p, "deep focus") // storage folds case, so the panel must too
	p, action := press(t, p, "enter")

	if action != actionNone {
		t.Fatalf("a colliding name produced %v; nothing may be written", action)
	}
	if !errors.Is(errorFromNote(p.recipeNote.Text), ErrRecipeNameTaken) {
		t.Fatalf("the refusal reads %q", p.recipeNote.Text)
	}
	if p.recipeNote.Tone != ToneError {
		t.Fatal("the refusal is not styled as one")
	}
}

// Renaming a recipe to its own name is not a collision — it is a no-op the user
// is entitled to make.
func TestRenamingToItsOwnNameIsNotACollision(t *testing.T) {
	p := recipesPanel(t, true)
	p, _ = press(t, p, "n")
	_, action := press(t, p, "enter")
	if action != ActionRecipe {
		t.Fatalf("re-confirming a recipe's own name produced %v", action)
	}
}

// An empty name is refused: every gesture on the page addresses a recipe by
// name, so an unnamed one would be unreachable.
func TestAnEmptyNameIsRefused(t *testing.T) {
	p := recipesPanel(t, true)
	p, _ = press(t, p, "s")
	p = typeName(t, p, "  ")
	p, action := press(t, p, "enter")
	if action != actionNone {
		t.Fatalf("a blank name produced %v", action)
	}
	if !strings.Contains(p.recipeNote.Text, "needs a name") {
		t.Fatalf("the refusal reads %q", p.recipeNote.Text)
	}
}

// The name box is a text field, so the panel-wide keys must not fire through it.
func TestThePromptSwallowsTheOverlayKeys(t *testing.T) {
	p := recipesPanel(t, true)
	p, _ = press(t, p, "s")

	for _, key := range []string{"tab", "shift+tab", "ctrl+s"} {
		next, action := press(t, p, key)
		if action != actionNone {
			t.Fatalf("%q escaped the prompt as %v", key, action)
		}
		if !next.RecipesActive() || next.prompt == nil {
			t.Fatalf("%q closed or paged away from the open prompt", key)
		}
	}

	p, action := press(t, p, "esc")
	if action != actionNone {
		t.Fatalf("esc in the prompt produced %v, want the box to close only", action)
	}
	if p.prompt != nil {
		t.Fatal("esc left the prompt open")
	}
	if !p.RecipesActive() {
		t.Fatal("esc closed the overlay instead of the prompt")
	}
}

// A refusal from storage reopens the box with the rejected name, which is the
// point of refusing rather than overwriting: the user is told, and is left
// holding what they need to fix it.
func TestARefusedSaveReopensThePromptWithTheName(t *testing.T) {
	p := recipesPanel(t, true)
	p, _ = press(t, p, "s")
	p = typeName(t, p, "Sunday")
	p, _ = press(t, p, "enter")

	p = p.AckRecipe(RecipeOutcome{Op: RecipeSave, Name: "Sunday", Err: ErrRecipeNameTaken})
	if p.prompt == nil {
		t.Fatal("the prompt did not reopen after a refusal")
	}
	if p.prompt.value != "Sunday" {
		t.Fatalf("the reopened box holds %q", p.prompt.value)
	}
	if p.recipeNote.Tone != ToneError {
		t.Fatal("the refusal is not reported as an error")
	}
}

// A refused delete has no box to reopen, and must not invent one.
func TestARefusedDeleteReopensNothing(t *testing.T) {
	p := recipesPanel(t, true)
	p, _ = press(t, p, "d")
	p = p.AckRecipe(RecipeOutcome{Op: RecipeDelete, Name: "Late Night", Err: errors.New("gone")})
	if p.prompt != nil {
		t.Fatal("a refused delete opened a name box")
	}
}

// The acknowledgement replaces the list, and a cursor left past the end of the
// shorter one has to come back inside it.
func TestAcknowledgingADeleteShrinksTheListAndClampsTheCursor(t *testing.T) {
	p := recipesPanel(t, true)
	p, _ = press(t, p, "down", "down")
	p, _ = press(t, p, "d")

	p = p.AckRecipe(RecipeOutcome{Op: RecipeDelete, Name: "Broken", Recipes: testRecipes()[:1]})
	if len(p.recipes) != 1 {
		t.Fatalf("the list holds %d recipes", len(p.recipes))
	}
	if p.recipeCursor != 0 {
		t.Fatalf("cursor = %d, want it clamped into the shorter list", p.recipeCursor)
	}
	if !strings.Contains(p.recipeNote.Text, "Deleted “Broken”") {
		t.Fatalf("the note reads %q", p.recipeNote.Text)
	}
}

// A failed re-read leaves the list alone rather than blanking the page, which
// would read as "you have saved none".
func TestAFailedReloadKeepsTheListOnScreen(t *testing.T) {
	p := recipesPanel(t, true)
	p, _ = press(t, p, "s")
	p = typeName(t, p, "Sunday")
	p, _ = press(t, p, "enter")

	p = p.AckRecipe(RecipeOutcome{Op: RecipeSave, Name: "Sunday", Err: errors.New("disk on fire")})
	if len(p.recipes) != 3 {
		t.Fatalf("the list holds %d recipes after a failed save", len(p.recipes))
	}
	if p.recipeNote.Tone != ToneError {
		t.Fatal("the failure was not reported")
	}
}

// An answer to a request this panel never made is dropped, for the same reason a
// late commit acknowledgement is: it would put a note about work the user cannot
// see — and a list they never asked to be reloaded — onto the page.
func TestAnUnrequestedOutcomeIsIgnored(t *testing.T) {
	p := recipesPanel(t, true)

	p = p.AckRecipe(RecipeOutcome{Op: RecipeDelete, Name: "Late Night", Recipes: nil})

	if p.recipeNote.Text != "" {
		t.Fatalf("an unrequested answer left the note %q", p.recipeNote.Text)
	}
	if len(p.recipes) != 3 {
		t.Fatalf("an unrequested answer changed the list to %d recipes", len(p.recipes))
	}
}

// An unreadable recipe store is said out loud for the same reason.
func TestAnUnreadableStoreIsReportedRatherThanShownEmpty(t *testing.T) {
	p := New(Options{
		Applied: testSettings(), Catalog: testCatalog(),
		RecipesErr: errors.New("database is locked"),
	})
	p, _ = press(t, p, "tab", "tab")
	page := recipesPage(p)
	if !strings.Contains(page, "Saved recipes could not be read: database is locked") {
		t.Fatalf("the page does not report the failure:\n%s", page)
	}
}

// The page summarizes each recipe well enough to tell two apart without loading
// either of them.
func TestThePageSummarizesEachRecipe(t *testing.T) {
	page := recipesPage(recipesPanel(t, true))
	assertContains(t, page,
		"▸ Late Night · Explore · 80% discovery",
		"Deep Focus · Familiar · 20% discovery",
		"Broken · unreadable",
		"1 include",
		"1 exclude")
}

// The label on the page and on the commit button agree about what a load leads
// to, which differs between a running session and a stopped one.
func TestThePageNamesTheCommitVerb(t *testing.T) {
	if page := recipesPage(recipesPanel(t, false)); !strings.Contains(page, "which you then start") {
		t.Fatalf("a stopped session's page reads:\n%s", page)
	}
	if page := recipesPage(recipesPanel(t, true)); !strings.Contains(page, "which you then apply") {
		t.Fatalf("a running session's page reads:\n%s", page)
	}
}

// errorFromNote re-wraps a rendered note so a test can ask which error it came
// from. The note is the error's own text, so a match is exact.
func errorFromNote(note string) error {
	if strings.HasPrefix(note, ErrRecipeNameTaken.Error()) {
		return ErrRecipeNameTaken
	}
	return errors.New(note)
}

// Storage lists recipes by name, so a save reorders the page. The cursor has to
// follow the recipe that was just written, or the next key would act on whatever
// row slid into its place.
func TestTheCursorFollowsAFreshlySavedRecipe(t *testing.T) {
	p := recipesPanel(t, true)
	p, _ = press(t, p, "s")
	p = typeName(t, p, "Aardvark")
	p, _ = press(t, p, "enter")

	// The reload comes back name-ordered, with the new recipe first.
	saved := append([]Recipe{{ID: 9, Name: "Aardvark"}}, testRecipes()...)
	p = p.AckRecipe(RecipeOutcome{Op: RecipeSave, Name: "Aardvark", Recipes: saved})

	if got, _ := p.focusedRecipe(); got.Name != "Aardvark" {
		t.Fatalf("the cursor is on %q, want the recipe just saved", got.Name)
	}
}
