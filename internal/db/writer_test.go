package db_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/sglavoie/muse/internal/db"
)

func openTestDB(t *testing.T) *db.DB {
	t.Helper()
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

// TestWriter_Serialization verifies that concurrent callers do not produce
// SQLITE_BUSY errors; all writes must succeed.
func TestWriter_Serialization(t *testing.T) {
	d := openTestDB(t)
	if _, err := d.Exec("CREATE TABLE counter (n INTEGER)"); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := d.Exec("INSERT INTO counter VALUES (0)"); err != nil {
		t.Fatalf("insert: %v", err)
	}

	w := db.NewWriter(t.Context(), d.DB)

	const goroutines = 50
	var wg sync.WaitGroup
	var errCount atomic.Int64

	for range goroutines {
		wg.Go(func() {
			err := w.Exec(func(sdb *sql.DB) error {
				_, err := sdb.Exec("UPDATE counter SET n = n + 1")
				return err
			})
			if err != nil {
				errCount.Add(1)
			}
		})
	}

	wg.Wait()

	if n := errCount.Load(); n != 0 {
		t.Errorf("got %d write errors (want 0); concurrent writes produced errors", n)
	}

	var count int
	if err := d.QueryRow("SELECT n FROM counter").Scan(&count); err != nil {
		t.Fatalf("select: %v", err)
	}
	if count != goroutines {
		t.Errorf("counter = %d, want %d", count, goroutines)
	}
}

// TestWriter_CallerBlocks verifies that Exec blocks until the write completes
// and the result is available before returning.
func TestWriter_CallerBlocks(t *testing.T) {
	d := openTestDB(t)
	if _, err := d.Exec("CREATE TABLE val (v TEXT)"); err != nil {
		t.Fatalf("create: %v", err)
	}

	w := db.NewWriter(t.Context(), d.DB)

	err := w.Exec(func(sdb *sql.DB) error {
		_, err := sdb.Exec("INSERT INTO val VALUES ('hello')")
		return err
	})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}

	// The row must be visible immediately after Exec returns.
	var v string
	if err := d.QueryRow("SELECT v FROM val").Scan(&v); err != nil {
		t.Fatalf("query: %v", err)
	}
	if v != "hello" {
		t.Errorf("val = %q, want %q", v, "hello")
	}
}

// TestWriter_PropagatesError confirms that errors returned by fn are returned
// to the caller unchanged.
func TestWriter_PropagatesError(t *testing.T) {
	d := openTestDB(t)

	w := db.NewWriter(t.Context(), d.DB)

	err := w.Exec(func(sdb *sql.DB) error {
		_, err := sdb.Exec("INSERT INTO nonexistent_table VALUES (1)")
		return err
	})
	if err == nil {
		t.Fatal("expected error from bad INSERT, got nil")
	}
}

// TestWriter_ShutdownClean verifies that cancelling the context causes the
// writer goroutine to exit and Wait returns.
func TestWriter_ShutdownClean(t *testing.T) {
	d := openTestDB(t)

	ctx, cancel := context.WithCancel(t.Context())
	w := db.NewWriter(ctx, d.DB)

	cancel()
	// Wait must return after cancellation; if it hangs the test times out.
	w.Wait()
}

// TestWriter_ExecAfterStop confirms that submitting work after the writer has
// stopped returns an error instead of blocking.
func TestWriter_ExecAfterStop(t *testing.T) {
	d := openTestDB(t)

	ctx, cancel := context.WithCancel(t.Context())
	w := db.NewWriter(ctx, d.DB)
	cancel()
	w.Wait()

	err := w.Exec(func(sdb *sql.DB) error {
		return nil
	})
	if err == nil {
		t.Fatal("expected error when submitting to stopped writer, got nil")
	}
}
