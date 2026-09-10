package db_test

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/radio"
)

func TestCreateRecipe_RoundTrip(t *testing.T) {
	d, w := openTagDB(t)
	rock := makeTag(t, w, "rock")
	live := makeTag(t, w, "live")

	settings := djTestSettings(rock, live)
	id := makeRecipe(t, w, "  Late Night  ", settings)

	got := readRecipe(t, d, id)

	if !got.Usable() {
		t.Fatalf("recipe is not usable: %v", got.Invalid)
	}
	if got.Recipe.Name != "Late Night" {
		t.Errorf("Name = %q, want %q (trimmed)", got.Recipe.Name, "Late Night")
	}
	if want := settings.Normalized(); !reflect.DeepEqual(got.Recipe.Settings, want) {
		t.Errorf("Settings = %+v, want %+v", got.Recipe.Settings, want)
	}
	if got.Version != db.DJSettingsVersion {
		t.Errorf("Version = %d, want %d", got.Version, db.DJSettingsVersion)
	}
	if got.Recipe.CreatedAt.IsZero() || got.Recipe.UpdatedAt.IsZero() {
		t.Errorf("timestamps not persisted: created=%v updated=%v", got.Recipe.CreatedAt, got.Recipe.UpdatedAt)
	}
}

// TestCreateRecipe_StoresSettingsWithoutSeed pins the split the product contract
// requires: recipes save settings, and the seed belongs to the active session.
func TestCreateRecipe_StoresSettingsWithoutSeed(t *testing.T) {
	d, w := openTagDB(t)
	rock := makeTag(t, w, "rock")
	live := makeTag(t, w, "live")

	makeRecipe(t, w, "Late Night", djTestSettings(rock, live))
	writeDJSession(t, w, db.DJSession{
		Active:   true,
		Settings: djTestSettings(rock, live),
		Seed:     djTestSeed(rock, live),
	})

	if n := countRows(t, d, `SELECT COUNT(*) FROM dj_tag_refs WHERE scope = 'recipe' AND role = 'seed'`); n != 0 {
		t.Errorf("recipe-scoped seed tag refs = %d, want 0", n)
	}
	if n := countRows(t, d, `SELECT COUNT(*) FROM dj_tag_refs WHERE scope = 'session' AND role = 'seed'`); n != 2 {
		t.Errorf("session-scoped seed tag refs = %d, want 2", n)
	}
}

func TestReadRecipes_OrderedByName(t *testing.T) {
	d, w := openTagDB(t)

	makeRecipe(t, w, "zebra", radio.DefaultSettings())
	makeRecipe(t, w, "Alpha", radio.DefaultSettings())
	makeRecipe(t, w, "middle", radio.DefaultSettings())

	recipes, err := d.ReadRecipes()
	if err != nil {
		t.Fatalf("ReadRecipes: %v", err)
	}
	got := make([]string, 0, len(recipes))
	for _, r := range recipes {
		got = append(got, r.Recipe.Name)
	}
	want := []string{"Alpha", "middle", "zebra"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("recipe order = %v, want %v", got, want)
	}
}

func TestCreateRecipe_RejectsDuplicateNameIgnoringCase(t *testing.T) {
	d, w := openTagDB(t)
	makeRecipe(t, w, "Late Night", radio.DefaultSettings())

	_, err := w.CreateRecipe(radio.Recipe{Name: "late night", Settings: radio.DefaultSettings()})
	if !errors.Is(err, db.ErrRecipeNameTaken) {
		t.Fatalf("CreateRecipe duplicate error = %v, want ErrRecipeNameTaken", err)
	}
	if n := countRows(t, d, `SELECT COUNT(*) FROM dj_recipes`); n != 1 {
		t.Errorf("recipe count = %d after a rejected duplicate, want 1", n)
	}
}

func TestCreateRecipe_RejectsEmptyName(t *testing.T) {
	_, w := openTagDB(t)

	_, err := w.CreateRecipe(radio.Recipe{Name: "   ", Settings: radio.DefaultSettings()})
	if !errors.Is(err, radio.ErrEmptyRecipeName) {
		t.Fatalf("CreateRecipe blank name error = %v, want ErrEmptyRecipeName", err)
	}
}

func TestUpdateRecipe_RewritesSettings(t *testing.T) {
	d, w := openTagDB(t)
	rock := makeTag(t, w, "rock")
	live := makeTag(t, w, "live")
	id := makeRecipe(t, w, "Late Night", djTestSettings(rock, live))

	updated := radio.Settings{
		DiscoveryPercent: 20,
		Anchor:           radio.AnchorSeed,
		Include:          radio.FilterSet{Genres: []string{"Ambient"}},
	}
	if err := w.UpdateRecipe(radio.Recipe{ID: id, Name: "Early Morning", Settings: updated}); err != nil {
		t.Fatalf("UpdateRecipe: %v", err)
	}

	got := readRecipe(t, d, id)
	if got.Recipe.Name != "Early Morning" {
		t.Errorf("Name = %q, want %q", got.Recipe.Name, "Early Morning")
	}
	if want := updated.Normalized(); !reflect.DeepEqual(got.Recipe.Settings, want) {
		t.Errorf("Settings = %+v, want %+v", got.Recipe.Settings, want)
	}
	if !got.Recipe.Settings.Exclude.IsEmpty() {
		t.Errorf("Exclude = %+v, want empty after the update cleared it", got.Recipe.Settings.Exclude)
	}
}

func TestUpdateRecipe_RejectsUnknownID(t *testing.T) {
	_, w := openTagDB(t)

	err := w.UpdateRecipe(radio.Recipe{ID: 404, Name: "Ghost", Settings: radio.DefaultSettings()})
	if !errors.Is(err, db.ErrRecipeNotFound) {
		t.Fatalf("UpdateRecipe unknown id error = %v, want ErrRecipeNotFound", err)
	}
}

func TestUpdateRecipe_RejectsNameTakenByAnother(t *testing.T) {
	_, w := openTagDB(t)
	makeRecipe(t, w, "Late Night", radio.DefaultSettings())
	id := makeRecipe(t, w, "Early Morning", radio.DefaultSettings())

	err := w.UpdateRecipe(radio.Recipe{ID: id, Name: "LATE NIGHT", Settings: radio.DefaultSettings()})
	if !errors.Is(err, db.ErrRecipeNameTaken) {
		t.Fatalf("UpdateRecipe collision error = %v, want ErrRecipeNameTaken", err)
	}
}

// TestUpdateRecipe_KeepsOwnName proves the self-exclusion in the uniqueness
// check: re-saving a recipe under its own name must not collide with itself.
func TestUpdateRecipe_KeepsOwnName(t *testing.T) {
	d, w := openTagDB(t)
	id := makeRecipe(t, w, "Late Night", radio.DefaultSettings())

	settings := radio.DefaultSettings().WithDiscoveryPercent(80)
	if err := w.UpdateRecipe(radio.Recipe{ID: id, Name: "Late Night", Settings: settings}); err != nil {
		t.Fatalf("UpdateRecipe: %v", err)
	}
	if got := readRecipe(t, d, id).Recipe.Settings.DiscoveryPercent; got != 80 {
		t.Errorf("DiscoveryPercent = %d, want 80", got)
	}
}

func TestDeleteRecipeSoft_HidesAndUndoRestores(t *testing.T) {
	d, w := openTagDB(t)
	rock := makeTag(t, w, "rock")
	live := makeTag(t, w, "live")
	id := makeRecipe(t, w, "Late Night", djTestSettings(rock, live))

	if err := w.DeleteRecipeSoft(id); err != nil {
		t.Fatalf("DeleteRecipeSoft: %v", err)
	}
	if _, err := d.ReadRecipe(id); !errors.Is(err, db.ErrRecipeNotFound) {
		t.Fatalf("ReadRecipe after delete = %v, want ErrRecipeNotFound", err)
	}
	recipes, err := d.ReadRecipes()
	if err != nil {
		t.Fatalf("ReadRecipes: %v", err)
	}
	if len(recipes) != 0 {
		t.Errorf("ReadRecipes returned %d rows after a soft delete, want 0", len(recipes))
	}

	if err := w.UndoDeleteRecipe(id); err != nil {
		t.Fatalf("UndoDeleteRecipe: %v", err)
	}
	got := readRecipe(t, d, id)
	if want := djTestSettings(rock, live).Normalized(); !reflect.DeepEqual(got.Recipe.Settings, want) {
		t.Errorf("Settings after undo = %+v, want %+v", got.Recipe.Settings, want)
	}
}

// TestCreateRecipe_ReusesSoftDeletedName shows the uniqueness index only binds
// active recipes: a deleted recipe must not hold its name hostage.
func TestCreateRecipe_ReusesSoftDeletedName(t *testing.T) {
	_, w := openTagDB(t)
	id := makeRecipe(t, w, "Late Night", radio.DefaultSettings())
	if err := w.DeleteRecipeSoft(id); err != nil {
		t.Fatalf("DeleteRecipeSoft: %v", err)
	}

	if _, err := w.CreateRecipe(radio.Recipe{Name: "Late Night", Settings: radio.DefaultSettings()}); err != nil {
		t.Fatalf("CreateRecipe reusing a freed name: %v", err)
	}
}

func TestUndoDeleteRecipe_RejectsReclaimedName(t *testing.T) {
	_, w := openTagDB(t)
	id := makeRecipe(t, w, "Late Night", radio.DefaultSettings())
	if err := w.DeleteRecipeSoft(id); err != nil {
		t.Fatalf("DeleteRecipeSoft: %v", err)
	}
	makeRecipe(t, w, "late night", radio.DefaultSettings())

	if err := w.UndoDeleteRecipe(id); !errors.Is(err, db.ErrRecipeNameTaken) {
		t.Fatalf("UndoDeleteRecipe error = %v, want ErrRecipeNameTaken", err)
	}
}

func TestPurgeDeletedRecipes_RemovesFilterRows(t *testing.T) {
	d, w := openTagDB(t)
	rock := makeTag(t, w, "rock")
	live := makeTag(t, w, "live")
	purged := makeRecipe(t, w, "Purge Me", djTestSettings(rock, live))
	kept := makeRecipe(t, w, "Keep Me", djTestSettings(rock, live))

	if err := w.DeleteRecipeSoft(purged); err != nil {
		t.Fatalf("DeleteRecipeSoft: %v", err)
	}
	if err := w.PurgeDeletedRecipes(time.Now().Add(time.Hour).Unix()); err != nil {
		t.Fatalf("PurgeDeletedRecipes: %v", err)
	}

	if n := countRows(t, d, `SELECT COUNT(*) FROM dj_recipes WHERE id = ?`, purged); n != 0 {
		t.Errorf("purged recipe rows = %d, want 0", n)
	}
	if n := countRows(t, d, `SELECT COUNT(*) FROM dj_tag_refs WHERE scope = 'recipe' AND owner_id = ?`, purged); n != 0 {
		t.Errorf("purged recipe tag refs = %d, want 0", n)
	}
	if n := countRows(t, d, `SELECT COUNT(*) FROM dj_filter_values WHERE scope = 'recipe' AND owner_id = ?`, purged); n != 0 {
		t.Errorf("purged recipe filter values = %d, want 0", n)
	}
	if n := countRows(t, d, `SELECT COUNT(*) FROM dj_tag_refs WHERE scope = 'recipe' AND owner_id = ?`, kept); n == 0 {
		t.Error("purge removed the surviving recipe's tag refs")
	}
	if _, err := d.ReadRecipe(kept); err != nil {
		t.Errorf("ReadRecipe(kept): %v", err)
	}
}

// TestApplyRecipe_SnapshotSurvivesRecipeEdits is the core snapshot guarantee:
// once a recipe's settings are applied to the active session, editing or
// deleting that recipe must not change what the DJ is playing.
func TestApplyRecipe_SnapshotSurvivesRecipeEdits(t *testing.T) {
	d, w := openTagDB(t)
	rock := makeTag(t, w, "rock")
	live := makeTag(t, w, "live")
	id := makeRecipe(t, w, "Late Night", djTestSettings(rock, live))

	applied := readRecipe(t, d, id).Recipe.Settings
	writeDJSession(t, w, db.DJSession{Active: true, Settings: applied, Seed: djTestSeed(rock)})

	// Edit the recipe out from under the running session, then delete it.
	wideOpen := radio.Settings{DiscoveryPercent: 100, Anchor: radio.AnchorSeed}
	if err := w.UpdateRecipe(radio.Recipe{ID: id, Name: "Late Night", Settings: wideOpen}); err != nil {
		t.Fatalf("UpdateRecipe: %v", err)
	}
	if err := w.DeleteRecipeSoft(id); err != nil {
		t.Fatalf("DeleteRecipeSoft: %v", err)
	}
	if err := w.PurgeDeletedRecipes(time.Now().Add(time.Hour).Unix()); err != nil {
		t.Fatalf("PurgeDeletedRecipes: %v", err)
	}

	got := readDJSession(t, d)
	if !reflect.DeepEqual(got.Settings, applied) {
		t.Errorf("session settings = %+v after the recipe changed, want the applied snapshot %+v",
			got.Settings, applied)
	}
	if !got.Active {
		t.Error("session stopped being active because its recipe was deleted")
	}
}

// TestReadRecipes_InvalidRowsSurfaced proves a corrupt row is listed with its
// problem attached, never silently repaired into permissive settings, and never
// hides the rest of the recipe library.
func TestReadRecipes_InvalidRowsSurfaced(t *testing.T) {
	cases := []struct {
		name    string
		mutate  string
		wantErr error
	}{
		{"off-step discovery target", `UPDATE dj_recipes SET discovery_percent = 45 WHERE name = 'Broken'`, db.ErrDJStateInvalid},
		{"unknown anchor mode", `UPDATE dj_recipes SET anchor_mode = 'sideways' WHERE name = 'Broken'`, db.ErrDJStateInvalid},
		{"version from a newer build", `UPDATE dj_recipes SET version = 99 WHERE name = 'Broken'`, db.ErrDJStateUnsupported},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, w := openTagDB(t)
			rock := makeTag(t, w, "rock")
			live := makeTag(t, w, "live")
			makeRecipe(t, w, "Broken", djTestSettings(rock, live))
			makeRecipe(t, w, "Healthy", radio.DefaultSettings())

			if _, err := d.Exec(tc.mutate); err != nil {
				t.Fatalf("mutate: %v", err)
			}

			recipes, err := d.ReadRecipes()
			if err != nil {
				t.Fatalf("ReadRecipes: %v", err)
			}
			if len(recipes) != 2 {
				t.Fatalf("ReadRecipes returned %d rows, want 2 (a broken row must not hide the rest)", len(recipes))
			}

			byName := map[string]db.DJRecipe{}
			for _, r := range recipes {
				byName[r.Recipe.Name] = r
			}
			broken, healthy := byName["Broken"], byName["Healthy"]

			if broken.Usable() {
				t.Fatal("broken recipe reports as usable")
			}
			if !errors.Is(broken.Invalid, tc.wantErr) {
				t.Errorf("Invalid = %v, want one wrapping %v", broken.Invalid, tc.wantErr)
			}
			// Zero settings fail radio validation, so an unchecked caller gets
			// a refusal rather than an unrestricted DJ.
			if err := broken.Recipe.Settings.Validate(); err == nil {
				t.Error("broken recipe carries settings that validate; they must be left zero")
			}
			if !healthy.Usable() {
				t.Errorf("healthy recipe reports invalid: %v", healthy.Invalid)
			}
		})
	}
}
