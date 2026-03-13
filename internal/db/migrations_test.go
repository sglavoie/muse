package db_test

import (
	"path/filepath"
	"testing"

	"github.com/sglavoie/muse/internal/db"
)

func TestMigrate_AllTablesExist(t *testing.T) {
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer d.Close()

	if err := db.Migrate(d.DB); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	tables := []string{"tracks", "playlists", "playlist_tracks", "queue", "session"}
	for _, table := range tables {
		var name string
		err := d.QueryRow(
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?", table,
		).Scan(&name)
		if err != nil {
			t.Errorf("table %q not found: %v", table, err)
		}
	}
}

func TestMigrate_GooseVersionTable(t *testing.T) {
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer d.Close()

	if err := db.Migrate(d.DB); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	var name string
	err = d.QueryRow(
		"SELECT name FROM sqlite_master WHERE type='table' AND name='goose_db_version'",
	).Scan(&name)
	if err != nil {
		t.Errorf("goose_db_version table not found: %v", err)
	}
}

func TestMigrate_SessionSeedRow(t *testing.T) {
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer d.Close()

	if err := db.Migrate(d.DB); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	var count int
	if err := d.QueryRow("SELECT COUNT(*) FROM session WHERE id=1").Scan(&count); err != nil {
		t.Fatalf("query session: %v", err)
	}
	if count != 1 {
		t.Errorf("session seed row count = %d, want 1", count)
	}
}

func TestMigrate_Idempotent(t *testing.T) {
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer d.Close()

	for i := range 3 {
		if err := db.Migrate(d.DB); err != nil {
			t.Fatalf("Migrate call %d: %v", i+1, err)
		}
	}
}
