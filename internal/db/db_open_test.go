package db_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/sglavoie/muse/internal/db"
)

func TestOpen_WALMode(t *testing.T) {
	d := openTestDB(t)

	var mode string
	if err := d.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatalf("query journal_mode: %v", err)
	}
	if mode != "wal" {
		t.Errorf("journal_mode = %q, want %q", mode, "wal")
	}
}

func TestOpen_ForeignKeys(t *testing.T) {
	d := openTestDB(t)

	var fk int
	if err := d.QueryRow("PRAGMA foreign_keys").Scan(&fk); err != nil {
		t.Fatalf("query foreign_keys: %v", err)
	}
	if fk != 1 {
		t.Errorf("foreign_keys = %d, want 1", fk)
	}
}

func TestOpen_BusyTimeout(t *testing.T) {
	d := openTestDB(t)

	var timeout int
	if err := d.QueryRow("PRAGMA busy_timeout").Scan(&timeout); err != nil {
		t.Fatalf("query busy_timeout: %v", err)
	}
	if timeout != 5000 {
		t.Errorf("busy_timeout = %d, want 5000", timeout)
	}
}

func TestOpen_PragmasApplyToPooledConnections(t *testing.T) {
	d := openTestDB(t)
	d.SetMaxOpenConns(2)

	ctx := context.Background()
	conn, err := d.Conn(ctx)
	if err != nil {
		t.Fatalf("Conn: %v", err)
	}
	defer conn.Close()

	var pinnedFK int
	if err := conn.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&pinnedFK); err != nil {
		t.Fatalf("query pinned foreign_keys: %v", err)
	}
	if pinnedFK != 1 {
		t.Fatalf("pinned foreign_keys = %d, want 1", pinnedFK)
	}

	var pooledFK int
	if err := d.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&pooledFK); err != nil {
		t.Fatalf("query pooled foreign_keys: %v", err)
	}
	if pooledFK != 1 {
		t.Errorf("pooled foreign_keys = %d, want 1", pooledFK)
	}

	var pooledTimeout int
	if err := d.QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&pooledTimeout); err != nil {
		t.Fatalf("query pooled busy_timeout: %v", err)
	}
	if pooledTimeout != 5000 {
		t.Errorf("pooled busy_timeout = %d, want 5000", pooledTimeout)
	}
}

// TestOpen_InvalidPath opens a database whose parent directory is a regular
// file, which fails for every user including root, and never writes outside
// the test's temp dir (a missing absolute path would be created as root).
func TestOpen_InvalidPath(t *testing.T) {
	notADir := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(notADir, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := db.Open(filepath.Join(notADir, "directory", "test.db"))
	if err == nil {
		t.Fatal("expected error for invalid path, got nil")
	}
}
