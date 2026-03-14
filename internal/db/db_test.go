package db_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/db"
)

func TestOpen_WALMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")

	d, err := db.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer d.Close()

	var mode string
	if err := d.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatalf("query journal_mode: %v", err)
	}
	if mode != "wal" {
		t.Errorf("journal_mode = %q, want %q", mode, "wal")
	}
}

func TestOpen_ForeignKeys(t *testing.T) {
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer d.Close()

	var fk int
	if err := d.QueryRow("PRAGMA foreign_keys").Scan(&fk); err != nil {
		t.Fatalf("query foreign_keys: %v", err)
	}
	if fk != 1 {
		t.Errorf("foreign_keys = %d, want 1", fk)
	}
}

func TestOpen_BusyTimeout(t *testing.T) {
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer d.Close()

	var timeout int
	if err := d.QueryRow("PRAGMA busy_timeout").Scan(&timeout); err != nil {
		t.Fatalf("query busy_timeout: %v", err)
	}
	if timeout != 5000 {
		t.Errorf("busy_timeout = %d, want 5000", timeout)
	}
}

func TestOpen_InvalidPath(t *testing.T) {
	_, err := db.Open("/nonexistent/directory/test.db")
	if err == nil {
		t.Fatal("expected error for invalid path, got nil")
	}
}

func TestReadQueue_Empty(t *testing.T) {
	d := openMigratedDB(t)

	rows, err := d.ReadQueue()
	if err != nil {
		t.Fatalf("ReadQueue: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("got %d rows, want 0", len(rows))
	}
}

func TestReadQueue_PartiallySoftDeleted(t *testing.T) {
	d := openMigratedDB(t)

	insertTrack(t, d, "hashA", "/music/a.flac")
	insertTrack(t, d, "hashB", "/music/b.flac")

	// Insert two active entries.
	insertQueueEntry(t, d, "hashA", 1000)
	qidB := insertQueueEntry(t, d, "hashB", 2000)

	// Soft-delete entry B.
	now := time.Now().Unix()
	if _, err := d.Exec(`UPDATE queue SET deleted_at = ? WHERE id = ?`, now, qidB); err != nil {
		t.Fatalf("soft-delete: %v", err)
	}

	rows, err := d.ReadQueue()
	if err != nil {
		t.Fatalf("ReadQueue: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	if rows[0].Hash != "hashA" {
		t.Errorf("got hash %q, want %q", rows[0].Hash, "hashA")
	}
}

func TestReadSession_NoRow(t *testing.T) {
	d := openMigratedDB(t)

	// Remove the seeded session row to simulate a missing row.
	if _, err := d.Exec(`DELETE FROM session WHERE id = 1`); err != nil {
		t.Fatalf("delete session: %v", err)
	}

	row, err := d.ReadSession()
	if err != nil {
		t.Fatalf("ReadSession: %v", err)
	}
	if row != nil {
		t.Errorf("got non-nil row, want nil when no session row exists")
	}
}

func TestReadSession_Default(t *testing.T) {
	d := openMigratedDB(t)

	row, err := d.ReadSession()
	if err != nil {
		t.Fatalf("ReadSession: %v", err)
	}
	if row == nil {
		t.Fatal("got nil row, want seeded session row")
	}
	if row.RepeatMode != "none" {
		t.Errorf("RepeatMode = %q, want %q", row.RepeatMode, "none")
	}
	if row.Shuffle {
		t.Errorf("Shuffle = true, want false")
	}
	if row.CurrentHash != "" {
		t.Errorf("CurrentHash = %q, want empty", row.CurrentHash)
	}
	if row.PositionMs != 0 {
		t.Errorf("PositionMs = %d, want 0", row.PositionMs)
	}
}

func TestReadSession_WithValues(t *testing.T) {
	d := openMigratedDB(t)
	insertTrack(t, d, "hashX", "/music/x.flac")

	if _, err := d.Exec(`
		UPDATE session SET repeat_mode='all', shuffle=1, current_hash='hashX', position_ms=42000, updated_at=?
		WHERE id=1`, time.Now().Unix()); err != nil {
		t.Fatalf("update session: %v", err)
	}

	row, err := d.ReadSession()
	if err != nil {
		t.Fatalf("ReadSession: %v", err)
	}
	if row == nil {
		t.Fatal("got nil row")
	}
	if row.RepeatMode != "all" {
		t.Errorf("RepeatMode = %q, want %q", row.RepeatMode, "all")
	}
	if !row.Shuffle {
		t.Errorf("Shuffle = false, want true")
	}
	if row.CurrentHash != "hashX" {
		t.Errorf("CurrentHash = %q, want %q", row.CurrentHash, "hashX")
	}
	if row.PositionMs != 42000 {
		t.Errorf("PositionMs = %d, want 42000", row.PositionMs)
	}
}
