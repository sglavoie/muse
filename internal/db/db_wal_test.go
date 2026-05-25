package db_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/db"
)

func TestWALMode_DurabilityAfterAbruptClose(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "wal_durability.db")

	d, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := db.Migrate(d.DB); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	now := time.Now().Unix()
	type rowData struct{ hash, path string }
	wantRows := []rowData{
		{"wal-hash-1", "/music/wal-1.flac"},
		{"wal-hash-2", "/music/wal-2.flac"},
		{"wal-hash-3", "/music/wal-3.flac"},
	}

	tx, err := d.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	for _, r := range wantRows {
		if _, err := tx.Exec(
			`INSERT INTO tracks (hash, path, created_at, updated_at) VALUES (?, ?, ?, ?)`,
			r.hash, r.path, now, now,
		); err != nil {
			_ = tx.Rollback()
			t.Fatalf("insert %s: %v", r.hash, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	d.Close()

	d2, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer d2.Close()

	var count int
	if err := d2.QueryRow(`SELECT COUNT(*) FROM tracks WHERE hash LIKE 'wal-hash-%'`).Scan(&count); err != nil {
		t.Fatalf("count query: %v", err)
	}
	if count != len(wantRows) {
		t.Errorf("row count after reopen = %d, want %d", count, len(wantRows))
	}

	var gotPath string
	if err := d2.QueryRow(`SELECT path FROM tracks WHERE hash = ?`, "wal-hash-1").Scan(&gotPath); err != nil {
		t.Fatalf("field query: %v", err)
	}
	if gotPath != "/music/wal-1.flac" {
		t.Errorf("path = %q, want %q", gotPath, "/music/wal-1.flac")
	}
}
