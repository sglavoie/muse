package db_test

import (
	"context"
	"database/sql"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/db"
)

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
// writer goroutine to exit and Wait returns. This test specifically covers
// clean shutdown with no pending writes buffered in cmds; see
// TestWriter_DrainOnShutdown for the pending-writes drain scenario.
func TestWriter_ShutdownClean(t *testing.T) {
	d := openTestDB(t)

	ctx, cancel := context.WithCancel(t.Context())
	w := db.NewWriter(ctx, d.DB)

	cancel()
	w.Wait()
}

// TestWriter_ExecAfterCancelBeforeWaitRejected verifies that once the writer
// context is cancelled, new writes are rejected even if the writer goroutine is
// still inside an in-flight write and has not closed done yet. Without this
// guard, post-cancel writes could be accepted into the buffer after shutdown
// had started and race with drain().
func TestWriter_ExecAfterCancelBeforeWaitRejected(t *testing.T) {
	d := openTestDB(t)
	if _, err := d.Exec("CREATE TABLE post_cancel (n INTEGER)"); err != nil {
		t.Fatalf("create table: %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	w := db.NewWriter(ctx, d.DB)

	writerBusy := make(chan struct{})
	gate := make(chan struct{})
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- w.Exec(func(sdb *sql.DB) error {
			close(writerBusy)
			<-gate
			return nil
		})
	}()

	<-writerBusy
	cancel()

	err := w.Exec(func(sdb *sql.DB) error {
		_, err := sdb.Exec("INSERT INTO post_cancel VALUES (1)")
		return err
	})
	if err == nil {
		t.Fatal("Exec after cancellation returned nil; want stopped error")
	}

	close(gate)
	if err := <-firstDone; err != nil {
		t.Fatalf("in-flight Exec: %v", err)
	}
	w.Wait()

	var count int
	if err := d.QueryRow("SELECT COUNT(*) FROM post_cancel").Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Errorf("post-cancel write count = %d, want 0", count)
	}
}

// TestWriter_DrainOnShutdown verifies that pending writes buffered in the cmds
// channel are committed before the writer goroutine exits. The test blocks the
// writer goroutine inside the first command so that subsequent commands
// accumulate in the buffer, then cancels the context and confirms drain()
// flushes every buffered write.
func TestWriter_DrainOnShutdown(t *testing.T) {
	d := openTestDB(t)
	if _, err := d.Exec("CREATE TABLE items (n INTEGER)"); err != nil {
		t.Fatalf("create table: %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	w := db.NewWriter(ctx, d.DB)

	const n = 10

	writerBusy := make(chan struct{})
	gate := make(chan struct{})

	var wg sync.WaitGroup
	wg.Go(func() {
		_ = w.Exec(func(sdb *sql.DB) error {
			close(writerBusy)
			<-gate
			_, err := sdb.Exec("INSERT INTO items VALUES (0)")
			return err
		})
	})

	<-writerBusy

	var submitted sync.WaitGroup
	for i := 1; i < n; i++ {
		wg.Add(1)
		submitted.Add(1)
		val := i
		go func() {
			defer wg.Done()
			submitted.Done()
			_ = w.Exec(func(sdb *sql.DB) error {
				_, err := sdb.Exec("INSERT INTO items VALUES (?)", val)
				return err
			})
		}()
	}

	submitted.Wait()
	// submitted only proves the goroutines started; give them a scheduler turn
	// to reach Exec and fill the writer's buffered command channel while the
	// first command still holds the writer goroutine.
	time.Sleep(10 * time.Millisecond)

	close(gate)
	cancel()

	wg.Wait()
	w.Wait()

	var count int
	if err := d.QueryRow("SELECT COUNT(*) FROM items").Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != n {
		t.Errorf("DB has %d rows; want %d - drain() lost pending writes on shutdown", count, n)
	}
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

// TestWriter_DrainedExecDoesNotReturnSpuriousError verifies that an Exec call
// whose command is processed during drain() returns the actual result (not a
// "stopped while waiting" error) even though the done channel closes
// immediately after drain completes.
func TestWriter_DrainedExecDoesNotReturnSpuriousError(t *testing.T) {
	d := openMigratedTestDB(t)

	ctx, cancel := context.WithCancel(t.Context())
	w := db.NewWriter(ctx, d.DB)

	writerBusy := make(chan struct{})
	gate := make(chan struct{})

	var wg sync.WaitGroup
	wg.Go(func() {
		_ = w.Exec(func(sdb *sql.DB) error {
			close(writerBusy)
			<-gate
			return nil
		})
	})

	<-writerBusy

	errCh := make(chan error, 1)
	wg.Go(func() {
		errCh <- w.Exec(func(sdb *sql.DB) error {
			_, err := sdb.Exec(`INSERT INTO tracks (hash, path, created_at, updated_at) VALUES ('drain-test', '/drain', 0, 0)`)
			return err
		})
	})

	time.Sleep(200 * time.Millisecond)

	close(gate)
	cancel()

	wg.Wait()
	w.Wait()

	err := <-errCh
	if err != nil {
		t.Errorf("drained Exec returned error %q; want nil (write succeeded but done closed simultaneously)", err)
	}

	var count int
	if err := d.QueryRow(`SELECT COUNT(*) FROM tracks WHERE hash='drain-test'`).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("drain-test track count = %d; want 1", count)
	}
}
