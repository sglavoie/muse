package tui

import (
	"errors"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/radio"
	"github.com/sglavoie/muse/internal/tui/autodj"
)

// stubRecipeStore is an in-memory stand-in for the recipe half of the database.
// It enforces the one rule the panel depends on storage for — names are unique,
// case-insensitively — and records every call so a test can assert that a
// gesture wrote once and only once.
type stubRecipeStore struct {
	rows    []db.DJRecipe
	nextID  int64
	readErr error
	writes  []string
}

func newRecipeStore(rows ...db.DJRecipe) *stubRecipeStore {
	s := &stubRecipeStore{rows: rows, nextID: 100}
	for _, r := range rows {
		if r.Recipe.ID >= s.nextID {
			s.nextID = r.Recipe.ID + 1
		}
	}
	return s
}

func (s *stubRecipeStore) ReadRecipes() ([]db.DJRecipe, error) {
	if s.readErr != nil {
		return nil, s.readErr
	}
	return append([]db.DJRecipe(nil), s.rows...), nil
}

func (s *stubRecipeStore) taken(name string, skipID int64) bool {
	for _, r := range s.rows {
		if r.Recipe.ID != skipID && strings.EqualFold(r.Recipe.Name, name) {
			return true
		}
	}
	return false
}

func (s *stubRecipeStore) CreateRecipe(recipe radio.Recipe) (int64, error) {
	s.writes = append(s.writes, "create:"+recipe.Name)
	if s.taken(recipe.Name, 0) {
		return 0, db.ErrRecipeNameTaken
	}
	recipe.ID = s.nextID
	s.nextID++
	s.rows = append(s.rows, db.DJRecipe{Recipe: recipe})
	return recipe.ID, nil
}

func (s *stubRecipeStore) UpdateRecipe(recipe radio.Recipe) error {
	s.writes = append(s.writes, "update:"+recipe.Name)
	if s.taken(recipe.Name, recipe.ID) {
		return db.ErrRecipeNameTaken
	}
	for i, r := range s.rows {
		if r.Recipe.ID == recipe.ID {
			s.rows[i] = db.DJRecipe{Recipe: recipe}
			return nil
		}
	}
	return db.ErrRecipeNotFound
}

func (s *stubRecipeStore) DeleteRecipeSoft(id int64) error {
	s.writes = append(s.writes, "delete")
	for i, r := range s.rows {
		if r.Recipe.ID == id {
			s.rows = append(s.rows[:i], s.rows[i+1:]...)
			return nil
		}
	}
	return db.ErrRecipeNotFound
}

// find returns the stored recipe with the given name.
func (s *stubRecipeStore) find(name string) (radio.Recipe, bool) {
	for _, r := range s.rows {
		if r.Recipe.Name == name {
			return r.Recipe, true
		}
	}
	return radio.Recipe{}, false
}

// storedRecipe wraps a settings snapshot as storage holds it.
func storedRecipe(id int64, name string, settings radio.Settings) db.DJRecipe {
	return db.DJRecipe{Recipe: radio.Recipe{ID: id, Name: name, Settings: settings}}
}

// recipeModel opens the Tune panel on the Recipes tab over the given store.
func recipeModel(t *testing.T, store *stubRecipeStore) Model {
	t.Helper()
	m := newDJModel(WithLibraryTracks(djTestTracks()),
		WithDJRecipeReader(store), WithDJRecipeWriter(store))
	m.tags = []db.TagRow{{ID: 1, Name: "chill"}}
	m.tagsView.TagCounts = map[int64]int{1: 3}
	m, _ = m.openAutoDJTunePanel()
	m, _ = djPress(t, m, "tab", "tab")
	if !djPanel(t, m).RecipesActive() {
		t.Fatal("the panel is not on the Recipes page")
	}
	return m
}

// recipePage renders the open overlay with its styling stripped.
func recipePage(t *testing.T, m Model) string {
	t.Helper()
	m.termWidth, m.termHeight = 100, 40
	body, ok := m.renderOverlayBody()
	if !ok {
		t.Fatal("no overlay body was rendered")
	}
	return ansi.Strip(body)
}

// djTypeName types a recipe name into the open prompt.
func djTypeName(t *testing.T, m Model, name string) Model {
	t.Helper()
	for _, r := range name {
		key := string(r)
		if r == ' ' {
			key = "space"
		}
		m, _ = djPress(t, m, key)
	}
	return m
}

// The whole point of routing recipes through their own writer: saving reaches
// storage and nothing else. No session starts, no queue row appears, radio mode
// does not switch on.
func TestSavingARecipeChangesNoPlayback(t *testing.T) {
	store := newRecipeStore()
	m := recipeModel(t, store)
	before := m.Snapshot()

	m, _ = djPress(t, m, "s")
	m = djTypeName(t, m, "Sunday Morning")
	m, cmd := djPress(t, m, "enter")

	if cmd != nil {
		t.Fatal("saving a recipe launched a command; it must do nothing but write")
	}
	saved, ok := store.find("Sunday Morning")
	if !ok {
		t.Fatalf("nothing was stored: %v", store.writes)
	}
	if !saved.Settings.Equal(m.dj.settings) {
		t.Fatalf("stored %+v, want the draft", saved.Settings)
	}
	if m.dj.active || m.radio.enabled {
		t.Fatal("saving started a session")
	}
	if after := m.Snapshot(); after.QueueLength != before.QueueLength ||
		after.CurrentIndex != before.CurrentIndex {
		t.Fatalf("saving moved playback: %+v -> %+v", before, after)
	}
}

// Loading is the other half of the same guarantee, and it must not write either.
func TestLoadingARecipeChangesNoPlaybackAndNoStorage(t *testing.T) {
	store := newRecipeStore(storedRecipe(1, "Explore Hard", exploreSettings()))
	m := recipeModel(t, store)
	applied := m.dj.settings

	m, cmd := djPress(t, m, "enter")
	if cmd != nil {
		t.Fatal("loading a recipe launched a command")
	}
	if len(store.writes) != 0 {
		t.Fatalf("loading wrote to storage: %v", store.writes)
	}
	if !m.dj.settings.Equal(applied) {
		t.Fatal("loading moved the applied settings without a commit")
	}
	if got := djPanel(t, m).Draft(); !got.Equal(exploreSettings()) {
		t.Fatalf("the draft is %+v, want the loaded recipe", got)
	}
	if m.dj.active || m.radio.enabled {
		t.Fatal("loading started a session")
	}
}

// A loaded recipe still has to be committed, and committing it is an ordinary
// Start — which is where the playback change belongs.
func TestCommittingALoadedRecipeIsAnOrdinaryStart(t *testing.T) {
	store := newRecipeStore(storedRecipe(1, "Explore Hard", exploreSettings()))
	m := recipeModel(t, store)
	m.playback.currentHash = "f2"
	m, _ = djPress(t, m, "enter")
	m, _ = djPress(t, m, "ctrl+s")

	if !m.dj.active {
		t.Fatal("ctrl+s after a load started no session")
	}
	// A start captures a fresh seed, and it is the ordinary one: what is
	// playing, not anything the recipe carried.
	if m.dj.seed.TrackHash != "f2" {
		t.Fatalf("seed = %+v, want a fresh capture of the current track", m.dj.seed)
	}
	if !m.dj.settings.Equal(exploreSettings()) {
		t.Fatalf("the session runs on %+v, want the loaded recipe", m.dj.settings)
	}
	if len(store.writes) != 0 {
		t.Fatalf("starting a session wrote to the recipe store: %v", store.writes)
	}
}

// Applying a loaded recipe inside a running session adopts its settings and
// keeps the seed the session was started from. A recipe describes how to pick
// tracks, not what to pick them like — recapturing the seed here would silently
// re-aim a running session at whatever happens to be playing.
func TestApplyingALoadedRecipeKeepsTheRunningSeed(t *testing.T) {
	store := newRecipeStore(storedRecipe(1, "Explore Hard", exploreSettings()))
	m := newDJModel(WithLibraryTracks(djTestTracks()),
		WithDJRecipeReader(store), WithDJRecipeWriter(store),
		WithDJQueueWriter(&stubDJQueueWriter{}))
	m.dj.active = true
	m.dj.seed = djTestRequest().Seed
	m.playback.currentHash = "f2"
	seed := m.dj.seed

	m, _ = m.openAutoDJTunePanel()
	m, _ = djPress(t, m, "tab", "tab")
	m, _ = djPress(t, m, "enter")
	m, _ = djPress(t, m, "ctrl+s")

	if !m.dj.settings.Equal(exploreSettings()) {
		t.Fatalf("settings = %+v, want the loaded recipe", m.dj.settings)
	}
	if m.dj.seed.TrackHash != seed.TrackHash || m.dj.seed.CapturedAt != seed.CapturedAt {
		t.Fatalf("seed = %+v, want the running session's %+v", m.dj.seed, seed)
	}
}

// Storage is the authority on collisions. The panel's own check is skipped here
// by handing the request straight to the root, which is what a stale list would
// do in practice.
func TestStorageRefusesADuplicateNameInWordsTheTabUses(t *testing.T) {
	store := newRecipeStore(storedRecipe(1, "Late Night", exploreSettings()))
	m := recipeModel(t, store)

	res := m.performAutoDJRecipe(autodj.RecipeRequest{
		Op: autodj.RecipeSave, Name: "late night", Settings: m.dj.settings})

	if !errors.Is(res.Err, autodj.ErrRecipeNameTaken) {
		t.Fatalf("refusal = %v, want the tab's own name-taken error", res.Err)
	}
	if strings.Contains(res.Err.Error(), "db:") {
		t.Fatalf("the refusal still carries a storage prefix: %q", res.Err)
	}
	if len(store.rows) != 1 {
		t.Fatalf("the refused save still wrote: %d rows", len(store.rows))
	}
	if res.Recipes == nil {
		t.Fatal("a refusal must still refresh the list, so the collision is visible")
	}
}

// A recipe deleted from under the cursor is reported as gone rather than as a
// raw storage error, and the reload shows it is no longer there.
func TestActingOnAVanishedRecipeReportsItGone(t *testing.T) {
	store := newRecipeStore()
	m := recipeModel(t, store)

	res := m.performAutoDJRecipe(autodj.RecipeRequest{
		Op: autodj.RecipeDelete, ID: 42, Name: "Ghost"})
	if !errors.Is(res.Err, errDJRecipeGone) {
		t.Fatalf("refusal = %v", res.Err)
	}
}

// Renaming keeps the settings. This is the round trip that would silently
// re-save a recipe if the root decided which settings to send.
func TestRenamingThroughTheTabKeepsTheStoredSettings(t *testing.T) {
	store := newRecipeStore(storedRecipe(1, "Late Night", exploreSettings()))
	m := recipeModel(t, store)

	// Move the draft away from the stored snapshot first, so a rename that
	// wrongly sent the draft would be visible.
	m, _ = djPress(t, m, "shift+tab", "shift+tab")
	m, _ = djPress(t, m, "down", "right")
	m, _ = djPress(t, m, "tab", "tab")

	m, _ = djPress(t, m, "n")
	m, _ = djPress(t, m, "ctrl+u")
	m = djTypeName(t, m, "Very Late")
	m, _ = djPress(t, m, "enter")

	renamed, ok := store.find("Very Late")
	if !ok {
		t.Fatalf("the rename did not land: %v", store.writes)
	}
	if !renamed.Settings.Equal(exploreSettings()) {
		t.Fatalf("the rename rewrote the settings to %+v", renamed.Settings)
	}
	if _, still := store.find("Late Night"); still {
		t.Fatal("the old name survives, so the rename duplicated rather than moved")
	}
	if out := recipePage(t, m); !strings.Contains(out, "Renamed “Very Late”") {
		t.Fatalf("the page does not report the rename:\n%s", out)
	}
}

// Updating is the deliberate overwrite, and it writes the draft.
func TestUpdatingThroughTheTabWritesTheDraft(t *testing.T) {
	store := newRecipeStore(storedRecipe(1, "Late Night", radio.DefaultSettings()))
	m := recipeModel(t, store)
	m, _ = djPress(t, m, "shift+tab", "shift+tab")
	m, _ = djPress(t, m, "down", "right")
	draft := djPanel(t, m).Draft()
	m, _ = djPress(t, m, "tab", "tab")

	m, _ = djPress(t, m, "u")

	updated, ok := store.find("Late Night")
	if !ok {
		t.Fatal("the recipe is gone")
	}
	if !updated.Settings.Equal(draft) {
		t.Fatalf("stored %+v, want the draft %+v", updated.Settings, draft)
	}
	// An update keeps the name it overwrote, so the page names the same recipe.
	if out := recipePage(t, m); !strings.Contains(out, "Updated “Late Night”") {
		t.Fatalf("the page does not report the update:\n%s", out)
	}
}

// Deleting removes the row and the reloaded list shows it.
func TestDeletingThroughTheTabRemovesTheRow(t *testing.T) {
	store := newRecipeStore(
		storedRecipe(1, "Late Night", exploreSettings()),
		storedRecipe(2, "Deep Focus", radio.DefaultSettings()))
	m := recipeModel(t, store)

	m, _ = djPress(t, m, "d")

	if len(store.rows) != 1 {
		t.Fatalf("storage holds %d recipes", len(store.rows))
	}
	if _, gone := store.find("Late Night"); gone {
		t.Fatal("the wrong recipe was deleted")
	}
	// The page says what happened and moves the cursor onto the survivor, so a
	// second d cannot land on a row that is already gone.
	out := recipePage(t, m)
	if !strings.Contains(out, "Deleted “Late Night”") {
		t.Fatalf("the page does not confirm the delete:\n%s", out)
	}
	if !strings.Contains(out, "▸ Deep Focus") {
		t.Fatalf("the cursor did not land on the surviving recipe:\n%s", out)
	}
}

// A build with no recipe storage says so rather than appearing to save.
func TestWithoutAStoreTheTabRefusesRatherThanPretends(t *testing.T) {
	m := newDJModel(WithLibraryTracks(djTestTracks()))
	m, _ = m.openAutoDJTunePanel()

	res := m.performAutoDJRecipe(autodj.RecipeRequest{Op: autodj.RecipeSave, Name: "Nope"})
	if !errors.Is(res.Err, errNoDJRecipeStore) {
		t.Fatalf("refusal = %v", res.Err)
	}
}

// An unreadable store opens the tab with the failure on it, not with a list that
// looks empty.
func TestAnUnreadableStoreOpensTheTabWithTheFailure(t *testing.T) {
	store := newRecipeStore()
	store.readErr = errors.New("database is locked")
	m := newDJModel(WithLibraryTracks(djTestTracks()),
		WithDJRecipeReader(store), WithDJRecipeWriter(store))

	if _, err := m.autoDJRecipes(); err == nil {
		t.Fatal("the read failure was swallowed")
	}
	m, _ = m.openAutoDJTunePanel()
	m, _ = djPress(t, m, "tab", "tab")
	out := recipePage(t, m)
	if !strings.Contains(out, "Saved recipes could not be read") {
		t.Fatalf("the page does not report the failure:\n%s", out)
	}
}

// The Recipes page is its own overlay as far as a harness is concerned, because
// it can write where the Preview page cannot.
func TestTheRecipesPageIsItsOwnOverlaySnapshot(t *testing.T) {
	m := recipeModel(t, newRecipeStore())
	if got := m.Snapshot().Overlay; got != "autodj_recipes" {
		t.Fatalf("overlay = %q", got)
	}
}
