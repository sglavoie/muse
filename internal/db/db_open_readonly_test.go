package db_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
)

// newReadOnlyFixture creates a migrated database on disk, closes it, and
// returns its path so it can be reopened read-only.
func newReadOnlyFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")

	writable, err := db.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := db.Migrate(writable.DB); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	w := db.NewWriter(t.Context(), writable.DB)
	if err := w.UpsertBatch([]library.Track{{Hash: "h1", Path: "/music/a.flac", Title: "A", Artist: "Artist"}}); err != nil {
		t.Fatalf("seed UpsertBatch: %v", err)
	}
	if err := writable.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	return path
}

func TestOpenReadOnlyReads(t *testing.T) {
	path := newReadOnlyFixture(t)

	d, err := db.OpenReadOnly(path)
	if err != nil {
		t.Fatalf("OpenReadOnly: %v", err)
	}
	defer d.Close()

	var title string
	if err := d.QueryRow(`SELECT title FROM tracks WHERE hash = 'h1'`).Scan(&title); err != nil {
		t.Fatalf("query: %v", err)
	}
	if title != "A" {
		t.Fatalf("title = %q, want %q", title, "A")
	}
}

func TestOpenReadOnlyRejectsWrites(t *testing.T) {
	path := newReadOnlyFixture(t)

	d, err := db.OpenReadOnly(path)
	if err != nil {
		t.Fatalf("OpenReadOnly: %v", err)
	}
	defer d.Close()

	if _, err := d.Exec(`UPDATE tracks SET title = 'B' WHERE hash = 'h1'`); err == nil {
		t.Fatal("UPDATE succeeded on a read-only connection, want error")
	}
}

func TestOpenReadOnlyMissingFileCreatesNothing(t *testing.T) {
	dir := t.TempDir()
	parent := filepath.Join(dir, "missing-parent")
	path := filepath.Join(parent, "muse.db")

	if _, err := db.OpenReadOnly(path); err == nil {
		t.Fatal("OpenReadOnly on a nonexistent path succeeded, want error")
	}

	if _, err := os.Stat(parent); !os.IsNotExist(err) {
		t.Fatalf("parent directory stat err = %v, want IsNotExist", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("database file stat err = %v, want IsNotExist", err)
	}
}

func TestOpenReadOnlyPreservesJournalMode(t *testing.T) {
	path := newReadOnlyFixture(t)

	d, err := db.OpenReadOnly(path)
	if err != nil {
		t.Fatalf("OpenReadOnly: %v", err)
	}
	defer d.Close()

	var mode string
	if err := d.QueryRow(`PRAGMA journal_mode`).Scan(&mode); err != nil {
		t.Fatalf("PRAGMA journal_mode: %v", err)
	}
	if !strings.EqualFold(mode, "wal") {
		t.Fatalf("journal_mode = %q, want wal", mode)
	}
}
