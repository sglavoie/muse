package db_test

import (
	"strings"
	"testing"

	"github.com/sglavoie/muse/internal/db"
)

// dropRedundantIndexesVersion is the migration that drops the two indexes
// already covered by a wider one.
const dropRedundantIndexesVersion = 26

var redundantIndexes = []string{"idx_recommendation_cache_mix", "playlist_tracks_playlist_idx"}

func indexExists(t *testing.T, d *db.DB, name string) bool {
	t.Helper()
	return countRows(t, d,
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = ?`, name) > 0
}

func queryPlan(t *testing.T, d *db.DB, query string, args ...any) string {
	t.Helper()
	rows, err := d.Query("EXPLAIN QUERY PLAN "+query, args...)
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
	return strings.Join(plan, "\n")
}

// TestMigrate_DropsRedundantIndexes checks the two left-prefix indexes are gone
// and that the lookups they served still use an index rather than a scan.
func TestMigrate_DropsRedundantIndexes(t *testing.T) {
	d := openMigratedTestDB(t)

	for _, name := range redundantIndexes {
		if indexExists(t, d, name) {
			t.Errorf("%s still exists after migration %d", name, dropRedundantIndexesVersion)
		}
	}

	for _, tc := range []struct {
		query string
		index string
	}{
		{`SELECT track_hash FROM recommendation_cache WHERE mix_id = ?`, "sqlite_autoindex_recommendation_cache_1"},
		{`DELETE FROM recommendation_cache WHERE mix_id = ?`, "sqlite_autoindex_recommendation_cache_1"},
		{`SELECT track_hash FROM playlist_tracks WHERE playlist_id = ? AND deleted_at IS NULL ORDER BY position`, "playlist_tracks_pos_idx"},
	} {
		if plan := queryPlan(t, d, tc.query, 1); !strings.Contains(plan, tc.index) {
			t.Errorf("%q does not use %s; plan:\n%s", tc.query, tc.index, plan)
		}
	}
}

// TestMigrateDown_RestoresRedundantIndexes exercises the Down section and a
// second Up over it.
func TestMigrateDown_RestoresRedundantIndexes(t *testing.T) {
	d := openMigratedTestDB(t)

	migrateDownTo(t, d, dropRedundantIndexesVersion-1)
	for _, name := range redundantIndexes {
		if !indexExists(t, d, name) {
			t.Errorf("%s not restored by the rollback", name)
		}
	}

	if err := db.Migrate(d.DB); err != nil {
		t.Fatalf("Migrate after rollback: %v", err)
	}
	for _, name := range redundantIndexes {
		if indexExists(t, d, name) {
			t.Errorf("%s survived the second Up", name)
		}
	}
}
