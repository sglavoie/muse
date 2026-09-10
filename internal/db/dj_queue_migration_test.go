package db_test

import (
	"errors"
	"testing"

	"github.com/sglavoie/muse/internal/db"
)

// queueProvenanceMigrationVersion is the version that adds queue provenance.
const queueProvenanceMigrationVersion = 24

// TestMigrate_ProtectsPreMigrationQueueRows is the upgrade contract: a queue
// row written before provenance existed comes back as a manual entry, so the
// first refresh after upgrading cannot discard the user's queue.
func TestMigrate_ProtectsPreMigrationQueueRows(t *testing.T) {
	d := openMigratedTestDB(t)

	// Roll back to the schema that has no provenance columns and write a row
	// exactly as the previous build would have, including the "Radio" label
	// the pre-DJ radio path used.
	migrateDownTo(t, d, queueProvenanceMigrationVersion-1)
	seedTrack(t, d, "legacy1", "/music/legacy1.mp3")
	seedTrack(t, d, "legacy2", "/music/legacy2.mp3")
	if _, err := d.Exec(`
		INSERT INTO queue (track_hash, position, added_at, source_playlist)
		VALUES ('legacy1', 1000, 0, ''), ('legacy2', 2000, 0, 'Radio')
	`); err != nil {
		t.Fatalf("insert pre-migration queue rows: %v", err)
	}

	if err := db.Migrate(d.DB); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	rows := readQueue(t, d)
	if len(rows) != 2 {
		t.Fatalf("queue rows = %d after the upgrade, want 2", len(rows))
	}
	legacyIDs := make([]int64, 0, len(rows))
	for _, row := range rows {
		if row.DJOwned {
			t.Errorf("pre-migration row %q (source %q) reports DJ ownership",
				row.Hash, row.SourcePlaylist)
		}
		if len(row.DJReasons) != 0 {
			t.Errorf("pre-migration row %q carries reasons %v, want none", row.Hash, row.DJReasons)
		}
		legacyIDs = append(legacyIDs, row.ID)
	}

	w := restartWriter(t, d)
	if _, err := w.PrepareDJReplacement(djEntries(t, d, "dj1"), legacyIDs); !errors.Is(err, db.ErrQueueRowProtected) {
		t.Fatalf("PrepareDJReplacement over pre-migration rows: err = %v, want ErrQueueRowProtected", err)
	}
	assertHashes(t, "visible queue", queueHashes(t, d), []string{"legacy1", "legacy2"})
}

// TestMigrateDown_RemovesQueueProvenanceAndPreservesEntries rolls the
// provenance migration back and forward. Queue entries themselves must survive
// both directions, and the second Up has to leave a writable schema.
func TestMigrateDown_RemovesQueueProvenanceAndPreservesEntries(t *testing.T) {
	d, w := openTagDB(t)

	prep := prepareDJBatch(t, w, djEntries(t, d, "dj1"))
	acceptDJBatch(t, w, prep, db.DJCommit{})
	manualID, err := w.AppendTrack(djTrack(t, d, "manual1"), "Chill Mix")
	if err != nil {
		t.Fatalf("AppendTrack: %v", err)
	}

	migrateDownTo(t, d, queueProvenanceMigrationVersion-1)

	for _, column := range []string{"dj_owned", "dj_reasons", "pending_kind"} {
		if queueColumnExists(t, d, column) {
			t.Errorf("queue column %q survived the rollback", column)
		}
	}
	if n := countRows(t, d, `SELECT COUNT(*) FROM queue WHERE deleted_at IS NULL`); n != 2 {
		t.Errorf("active queue rows = %d after the rollback, want 2", n)
	}

	if err := db.Migrate(d.DB); err != nil {
		t.Fatalf("Migrate after rollback: %v", err)
	}

	// Provenance is gone, as it must be: the columns were dropped. Every
	// surviving row therefore reads back protected, which is the safe side.
	rows := readQueue(t, d)
	if len(rows) != 2 {
		t.Fatalf("queue rows = %d after the second Up, want 2", len(rows))
	}
	for _, row := range rows {
		if row.DJOwned {
			t.Errorf("row %q reports DJ ownership after the columns were rebuilt", row.Hash)
		}
	}
	if rows[1].ID != manualID {
		t.Errorf("manual row id = %d after the round trip, want %d", rows[1].ID, manualID)
	}

	// Writes still work against the rebuilt schema.
	rebuilt := prepareDJBatch(t, w, djEntries(t, d, "dj2"))
	acceptDJBatch(t, w, rebuilt, db.DJCommit{})
	assertHashes(t, "visible queue", queueHashes(t, d), []string{"dj1", "manual1", "dj2"})
}

// queueColumnExists reports whether the queue table has the named column.
func queueColumnExists(t *testing.T, d *db.DB, column string) bool {
	t.Helper()
	return countRows(t, d,
		`SELECT COUNT(*) FROM pragma_table_info('queue') WHERE name = ?`, column) > 0
}
