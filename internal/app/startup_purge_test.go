package app_test

import (
	"context"
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/app"
	"github.com/sglavoie/muse/internal/db"
)

// countWhere returns the result of a single-value COUNT query.
func countWhere(t *testing.T, database *db.DB, query string, args ...any) int {
	t.Helper()
	var n int
	if err := database.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("count %q: %v", query, err)
	}
	return n
}

func mustExec(t *testing.T, database *db.DB, query string, args ...any) int64 {
	t.Helper()
	res, err := database.Exec(query, args...)
	if err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
	id, _ := res.LastInsertId()
	return id
}

// TestRunPurgesSoftDeletedRowsFromPreviousRun seeds tombstones a previous
// process left behind, whose in-memory undo windows are gone, and checks that
// startup hard-deletes them while leaving every live row alone. The visible
// symptom it guards is a deleted tag name staying reserved forever.
func TestRunPurgesSoftDeletedRowsFromPreviousRun(t *testing.T) {
	dirs := setupDirs(t)
	installLogger(t)

	database := openTestDB(t, dirs)
	past := time.Now().Add(-time.Hour).Unix()
	insertTrack(t, database, "h1", "/music/a.mp3", "Alice", "One")
	insertTrack(t, database, "h2", "/music/b.mp3", "Bob", "Two")

	// Tags: one tombstone with an assignment, one live tag.
	deadTag := mustExec(t, database, `INSERT INTO tags (name, created_at, deleted_at) VALUES ('Rock', ?, ?)`, past, past)
	mustExec(t, database, `INSERT INTO track_tags (track_hash, tag_id, created_at) VALUES ('h1', ?, ?)`, deadTag, past)
	liveTag := mustExec(t, database, `INSERT INTO tags (name, created_at) VALUES ('Jazz', ?)`, past)

	// Playlists: a deleted playlist, and a live one holding a deleted entry.
	deadList := mustExec(t, database, `INSERT INTO playlists (name, created_at, updated_at, deleted_at) VALUES ('Old', ?, ?, ?)`, past, past, past)
	mustExec(t, database, `INSERT INTO playlist_tracks (playlist_id, track_hash, position, added_at) VALUES (?, 'h1', 1000, ?)`, deadList, past)
	liveList := mustExec(t, database, `INSERT INTO playlists (name, created_at, updated_at) VALUES ('Keep', ?, ?)`, past, past)
	mustExec(t, database, `INSERT INTO playlist_tracks (playlist_id, track_hash, position, added_at) VALUES (?, 'h1', 1000, ?)`, liveList, past)
	mustExec(t, database, `INSERT INTO playlist_tracks (playlist_id, track_hash, position, added_at, deleted_at) VALUES (?, 'h2', 2000, ?, ?)`, liveList, past, past)

	// Queue: a live row, a tombstone, and a preparation the previous process
	// abandoned, which NewWriter turns into a tombstone stamped "now".
	insertQueueEntry(t, database, "h1", 1000)
	mustExec(t, database, `INSERT INTO queue (track_hash, position, added_at, deleted_at) VALUES ('h2', 2000, ?, ?)`, past, past)
	mustExec(t, database, `INSERT INTO queue (track_hash, position, added_at, deleted_at, pending_kind) VALUES ('h2', 3000, ?, ?, 'append')`, past, int64(1<<63-1))

	// Recipes: a deleted recipe with filter rows, and a live one with its own.
	deadRecipe := mustExec(t, database, `INSERT INTO dj_recipes (name, discovery_percent, anchor_mode, created_at, updated_at, deleted_at) VALUES ('Gone', 40, 'anchor', ?, ?, ?)`, past, past, past)
	liveRecipe := mustExec(t, database, `INSERT INTO dj_recipes (name, discovery_percent, anchor_mode, created_at, updated_at) VALUES ('Kept', 40, 'anchor', ?, ?)`, past, past)
	for _, owner := range []int64{deadRecipe, liveRecipe} {
		mustExec(t, database, `INSERT INTO dj_tag_refs (scope, owner_id, role, tag_id) VALUES ('recipe', ?, 'include', ?)`, owner, liveTag)
		mustExec(t, database, `INSERT INTO dj_filter_values (scope, owner_id, role, kind, value) VALUES ('recipe', ?, 'include', 'genre', 'jazz')`, owner)
	}
	database.Close()

	result, err := app.Run(context.Background(), dirs, noopDeps())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	t.Cleanup(func() { closeRunResult(t, result) })
	d := result.DB

	checks := []struct {
		name  string
		query string
		want  int
	}{
		{"tags", `SELECT COUNT(*) FROM tags`, 1},
		{"track_tags of the purged tag", `SELECT COUNT(*) FROM track_tags`, 0},
		{"playlists", `SELECT COUNT(*) FROM playlists`, 1},
		{"playlist_tracks", `SELECT COUNT(*) FROM playlist_tracks`, 1},
		{"queue", `SELECT COUNT(*) FROM queue`, 1},
		{"dj_recipes", `SELECT COUNT(*) FROM dj_recipes`, 1},
		{"dj_tag_refs", `SELECT COUNT(*) FROM dj_tag_refs WHERE scope = 'recipe'`, 1},
		{"dj_filter_values", `SELECT COUNT(*) FROM dj_filter_values WHERE scope = 'recipe'`, 1},
		{"soft-deleted rows left", `SELECT
			(SELECT COUNT(*) FROM tags WHERE deleted_at IS NOT NULL) +
			(SELECT COUNT(*) FROM playlists WHERE deleted_at IS NOT NULL) +
			(SELECT COUNT(*) FROM playlist_tracks WHERE deleted_at IS NOT NULL) +
			(SELECT COUNT(*) FROM queue WHERE deleted_at IS NOT NULL) +
			(SELECT COUNT(*) FROM dj_recipes WHERE deleted_at IS NOT NULL)`, 0},
	}
	for _, c := range checks {
		if got := countWhere(t, d, c.query); got != c.want {
			t.Errorf("%s: count = %d, want %d", c.name, got, c.want)
		}
	}
	if got := countWhere(t, d, `SELECT COUNT(*) FROM dj_tag_refs WHERE owner_id = ?`, liveRecipe); got != 1 {
		t.Errorf("live recipe tag refs = %d, want 1", got)
	}
	if got := len(result.Restored.QueueEntries); got != 1 {
		t.Errorf("restored queue entries = %d, want 1", got)
	}

	// The purged tag no longer holds its name hostage.
	if _, err := result.Writer.CreateTag("rock", ""); err != nil {
		t.Errorf("CreateTag reusing a purged tag's name: %v", err)
	}
}
