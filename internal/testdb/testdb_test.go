package testdb_test

import (
	"testing"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/testdb"
)

func TestFixturesHaveIndependentDataAndSchema(t *testing.T) {
	first := testdb.Open(t)
	if _, err := first.Exec(`INSERT INTO tracks (hash, path, created_at, updated_at)
		VALUES ('first', '/first.flac', 1, 1)`); err != nil {
		t.Fatal(err)
	}
	if err := db.MigrateDown(first.DB); err != nil {
		t.Fatal(err)
	}
	firstVersion, err := db.MigrationVersion(first.DB)
	if err != nil {
		t.Fatal(err)
	}

	second := testdb.Open(t)
	var count int
	if err := second.QueryRow("SELECT count(*) FROM tracks").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("fresh fixture contains %d tracks from another test", count)
	}
	secondVersion, err := db.MigrationVersion(second.DB)
	if err != nil {
		t.Fatal(err)
	}
	if secondVersion <= firstVersion {
		t.Fatalf("rollback leaked into another fixture: versions %d, %d", firstVersion, secondVersion)
	}
	var mode string
	if err := second.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatal(err)
	}
	if mode != "wal" {
		t.Fatalf("journal mode = %q, want wal", mode)
	}
	if _, err := second.Exec(`INSERT INTO queue (track_hash, position, added_at)
		VALUES ('missing', 1, 1)`); err == nil {
		t.Fatal("fixture did not enforce foreign keys")
	}
}

func TestFixtureLifetimeIsIndependentOfFirstTest(t *testing.T) {
	t.Run("first owner", func(t *testing.T) {
		first := testdb.Open(t)
		if err := first.Ping(); err != nil {
			t.Fatal(err)
		}
	})
	// The first subtest's database and directory have both been cleaned up.
	t.Run("next owner", func(t *testing.T) {
		next := testdb.Open(t)
		var count int
		if err := next.QueryRow("SELECT count(*) FROM session WHERE id = 1").Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("session seed rows = %d, want 1", count)
		}
	})
}
