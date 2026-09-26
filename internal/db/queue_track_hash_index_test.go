package db_test

import (
	"strings"
	"testing"

	"github.com/sglavoie/muse/internal/db"
)

// queueTrackHashIndexVersion is the migration that indexes queue.track_hash.
const queueTrackHashIndexVersion = 25

func queueTrackHashIndexExists(t *testing.T, d *db.DB) bool {
	t.Helper()
	return countRows(t, d,
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = 'queue_track_hash_idx'`) > 0
}

// TestMigrate_QueueTrackHashIndex guards the per-track queue updates a library
// scan issues: without the index each `WHERE track_hash = ?` scans the whole
// queue, once per scanned track.
func TestMigrate_QueueTrackHashIndex(t *testing.T) {
	d := openMigratedTestDB(t)

	if !queueTrackHashIndexExists(t, d) {
		t.Fatal("queue_track_hash_idx not created")
	}

	rows, err := d.Query(`EXPLAIN QUERY PLAN UPDATE queue SET missing=0 WHERE track_hash=? AND missing=1`, "abc")
	if err != nil {
		t.Fatalf("EXPLAIN QUERY PLAN: %v", err)
	}
	defer rows.Close()
	var plan []string
	for rows.Next() {
		var id, parent, notUsed int
		var detail string
		if err := rows.Scan(&id, &parent, &notUsed, &detail); err != nil {
			t.Fatalf("scan plan row: %v", err)
		}
		plan = append(plan, detail)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("plan rows: %v", err)
	}
	if joined := strings.Join(plan, "\n"); !strings.Contains(joined, "queue_track_hash_idx") {
		t.Errorf("queue update by track_hash does not use the index; plan:\n%s", joined)
	}
}

// TestMigrateDown_DropsQueueTrackHashIndex exercises the Down section and a
// second Up over it.
func TestMigrateDown_DropsQueueTrackHashIndex(t *testing.T) {
	d := openMigratedTestDB(t)

	migrateDownTo(t, d, queueTrackHashIndexVersion-1)
	if queueTrackHashIndexExists(t, d) {
		t.Fatal("queue_track_hash_idx survived the rollback")
	}

	if err := db.Migrate(d.DB); err != nil {
		t.Fatalf("Migrate after rollback: %v", err)
	}
	if !queueTrackHashIndexExists(t, d) {
		t.Fatal("queue_track_hash_idx not recreated by the second Up")
	}
}
