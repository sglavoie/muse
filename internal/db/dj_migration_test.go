package db_test

import (
	"testing"

	"github.com/sglavoie/muse/internal/db"
)

// djMigrationVersion is the version that introduces the DJ tables.
const djMigrationVersion = 23

func djTables() []string {
	return []string{"dj_recipes", "dj_session", "dj_tag_refs", "dj_filter_values"}
}

func tableExists(t *testing.T, d *db.DB, table string) bool {
	t.Helper()
	return countRows(t, d,
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table) > 0
}

func TestMigrate_CreatesDJTables(t *testing.T) {
	d := openMigratedTestDB(t)

	for _, table := range djTables() {
		if !tableExists(t, d, table) {
			t.Errorf("table %q not created", table)
		}
	}
	if n := countRows(t, d, `SELECT COUNT(*) FROM dj_session WHERE id = 1`); n != 1 {
		t.Errorf("dj_session singleton rows = %d, want 1", n)
	}

	version, err := db.MigrationVersion(d.DB)
	if err != nil {
		t.Fatalf("MigrationVersion: %v", err)
	}
	if version < djMigrationVersion {
		t.Errorf("migration version = %d, want at least %d", version, djMigrationVersion)
	}
}

// TestMigrate_DJTablesSurviveRepeatedUp guards the additive upgrade path:
// re-running migrations over a populated database must not disturb DJ state.
func TestMigrate_DJTablesSurviveRepeatedUp(t *testing.T) {
	d, w := openTagDB(t)
	rock := makeTag(t, w, "rock")
	live := makeTag(t, w, "live")

	settings := djTestSettings(rock, live)
	recipeID := makeRecipe(t, w, "Late Night", settings)
	writeDJSession(t, w, db.DJSession{Active: true, Settings: settings, Seed: djTestSeed(rock)})

	if err := db.Migrate(d.DB); err != nil {
		t.Fatalf("Migrate again: %v", err)
	}

	if !readDJSession(t, d).Active {
		t.Error("active DJ session lost by re-running migrations")
	}
	if got := readRecipe(t, d, recipeID).Recipe.Name; got != "Late Night" {
		t.Errorf("recipe name = %q after re-running migrations, want %q", got, "Late Night")
	}
}

// TestMigrateDown_RemovesDJTablesAndPreservesTransport rolls the DJ migration
// back. The pre-existing session row and tag data must be untouched, and a
// second Up must rebuild the DJ tables cleanly.
func TestMigrateDown_RemovesDJTablesAndPreservesTransport(t *testing.T) {
	d, w := openTagDB(t)
	seedTrack(t, d, "abc", "/music/a.mp3")
	if err := w.UpsertSession("abc", 42_000, true, "all"); err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}
	rock := makeTag(t, w, "rock")
	live := makeTag(t, w, "live")
	if err := w.AddTrackTag("abc", rock); err != nil {
		t.Fatalf("AddTrackTag: %v", err)
	}

	settings := djTestSettings(rock, live)
	makeRecipe(t, w, "Late Night", settings)
	writeDJSession(t, w, db.DJSession{Active: true, Settings: settings, Seed: djTestSeed(rock)})

	if err := db.MigrateDown(d.DB); err != nil {
		t.Fatalf("MigrateDown: %v", err)
	}

	version, err := db.MigrationVersion(d.DB)
	if err != nil {
		t.Fatalf("MigrationVersion: %v", err)
	}
	if version != djMigrationVersion-1 {
		t.Errorf("migration version = %d after rollback, want %d", version, djMigrationVersion-1)
	}
	for _, table := range djTables() {
		if tableExists(t, d, table) {
			t.Errorf("table %q survived the rollback", table)
		}
	}

	// Everything the DJ migration did not create must still be there.
	session, err := d.ReadSession()
	if err != nil {
		t.Fatalf("ReadSession after rollback: %v", err)
	}
	if session == nil || session.CurrentHash != "abc" || session.PositionMs != 42_000 || session.RepeatMode != "all" {
		t.Errorf("transport session = %+v after rollback, want the pre-rollback values", session)
	}
	tags, err := d.ReadTags()
	if err != nil {
		t.Fatalf("ReadTags after rollback: %v", err)
	}
	if len(tags) != 2 {
		t.Errorf("tags = %d after rollback, want 2", len(tags))
	}
	trackTags, err := d.ReadTagsForTrack("abc")
	if err != nil {
		t.Fatalf("ReadTagsForTrack after rollback: %v", err)
	}
	if len(trackTags) != 1 {
		t.Errorf("track tags = %d after rollback, want 1", len(trackTags))
	}

	// Re-applying the migration must produce empty, usable DJ tables.
	if err := db.Migrate(d.DB); err != nil {
		t.Fatalf("Migrate after rollback: %v", err)
	}
	for _, table := range djTables() {
		if !tableExists(t, d, table) {
			t.Errorf("table %q not recreated by the second Up", table)
		}
	}
	restored := readDJSession(t, d)
	if restored.Active {
		t.Error("DJ session reports active after the tables were rebuilt from scratch")
	}
	if err := restored.Settings.Validate(); err != nil {
		t.Errorf("rebuilt DJ session settings do not validate: %v", err)
	}
	recipes, err := d.ReadRecipes()
	if err != nil {
		t.Fatalf("ReadRecipes after rollback and Up: %v", err)
	}
	if len(recipes) != 0 {
		t.Errorf("recipes = %d after rebuilding the tables, want 0", len(recipes))
	}

	// Writes still work against the rebuilt schema.
	writeDJSession(t, w, db.DJSession{Active: true, Settings: settings, Seed: djTestSeed(rock)})
	if !readDJSession(t, d).Active {
		t.Error("write against the rebuilt DJ tables did not take effect")
	}
}
