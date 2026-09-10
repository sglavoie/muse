package db_test

import (
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/radio"
)

// djCapturedAt is a fixed capture instant so seed round-trips compare exactly.
// Storage keeps whole seconds, so the literal has no sub-second component.
var djCapturedAt = time.Unix(1_700_000_000, 0)

// makeTag creates an active tag and returns its id.
func makeTag(t *testing.T, w *db.Writer, name string) int64 {
	t.Helper()
	id, err := w.CreateTag(name, "")
	if err != nil {
		t.Fatalf("CreateTag(%q): %v", name, err)
	}
	return id
}

// djTestSettings returns a fully populated settings value: every filter field on
// both sides is exercised so a round-trip that loses one is visible.
func djTestSettings(includeTag, excludeTag int64) radio.Settings {
	return radio.Settings{
		DiscoveryPercent: 60,
		Anchor:           radio.AnchorFollow,
		Include: radio.FilterSet{
			TagIDs:  []int64{includeTag},
			Genres:  []string{"Shoegaze", "Dream Pop"},
			Artists: []string{"Slowdive"},
		},
		Exclude: radio.FilterSet{
			TagIDs:  []int64{excludeTag},
			Genres:  []string{"Comedy"},
			Artists: []string{"Weird Al Yankovic"},
		},
	}
}

// djTestSeed returns a captured seed referencing the given tag ids.
func djTestSeed(tagIDs ...int64) radio.SeedSnapshot {
	return radio.SeedSnapshot{
		TrackHash:  "seedhash",
		Title:      "Alison",
		Artist:     "Slowdive",
		Album:      "Souvlaki",
		Genres:     []string{"Shoegaze", "Dream Pop"},
		TagIDs:     tagIDs,
		Duration:   234.5,
		CapturedAt: djCapturedAt,
	}
}

// writeDJSession writes a session and fails the test on error.
func writeDJSession(t *testing.T, w *db.Writer, session db.DJSession) {
	t.Helper()
	if err := w.WriteDJSession(session); err != nil {
		t.Fatalf("WriteDJSession: %v", err)
	}
}

// readDJSession reads the active session and fails the test on error.
func readDJSession(t *testing.T, d *db.DB) db.DJSession {
	t.Helper()
	session, err := d.ReadDJSession()
	if err != nil {
		t.Fatalf("ReadDJSession: %v", err)
	}
	return session
}

// makeRecipe saves a recipe and returns its id.
func makeRecipe(t *testing.T, w *db.Writer, name string, settings radio.Settings) int64 {
	t.Helper()
	id, err := w.CreateRecipe(radio.Recipe{Name: name, Settings: settings})
	if err != nil {
		t.Fatalf("CreateRecipe(%q): %v", name, err)
	}
	return id
}

// readRecipe reads one active recipe and fails the test on error.
func readRecipe(t *testing.T, d *db.DB, id int64) db.DJRecipe {
	t.Helper()
	recipe, err := d.ReadRecipe(id)
	if err != nil {
		t.Fatalf("ReadRecipe(%d): %v", id, err)
	}
	return recipe
}

// tagCatalog reads the active tag catalog and fails the test on error.
func tagCatalog(t *testing.T, d *db.DB) radio.TagIDSet {
	t.Helper()
	catalog, err := d.ReadTagCatalog()
	if err != nil {
		t.Fatalf("ReadTagCatalog: %v", err)
	}
	return catalog
}

// countRows returns the result of a single-value COUNT query.
func countRows(t *testing.T, d *db.DB, query string, args ...any) int {
	t.Helper()
	var count int
	if err := d.QueryRow(query, args...).Scan(&count); err != nil {
		t.Fatalf("count query %q: %v", query, err)
	}
	return count
}
