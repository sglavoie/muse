package db_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

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

// openMigratedTestDB opens a test DB with all migrations applied.
func openMigratedTestDB(t *testing.T) *db.DB {
	t.Helper()
	d := openTestDB(t)
	if err := db.Migrate(d.DB); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return d
}

// seedTrack inserts a minimal tracks row so that queue FK constraints are met.
func seedTrack(t *testing.T, d *db.DB, hash, path string) {
	t.Helper()
	now := time.Now().Unix()
	if _, err := d.Exec(
		`INSERT INTO tracks (hash, path, created_at, updated_at) VALUES (?, ?, ?, ?)`,
		hash, path, now, now,
	); err != nil {
		t.Fatalf("seedTrack: %v", err)
	}
}

// TestDequeueTrackSoft verifies that DequeueTrackSoft sets deleted_at.
func TestDequeueTrackSoft(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)

	seedTrack(t, d, "hs1", "/f1")
	now := time.Now().Unix()
	res, err := d.Exec(`INSERT INTO queue (track_hash, position, added_at) VALUES ('hs1', 1000, ?)`, now)
	if err != nil {
		t.Fatalf("insert queue: %v", err)
	}
	id, _ := res.LastInsertId()

	if err := w.DequeueTrackSoft(id); err != nil {
		t.Fatalf("DequeueTrackSoft: %v", err)
	}

	var deletedAt sql.NullInt64
	if err := d.QueryRow(`SELECT deleted_at FROM queue WHERE id=?`, id).Scan(&deletedAt); err != nil {
		t.Fatalf("query: %v", err)
	}
	if !deletedAt.Valid {
		t.Fatal("deleted_at is NULL after DequeueTrackSoft; want non-NULL timestamp")
	}
}

// TestUndoDequeue verifies that UndoDequeue clears deleted_at to NULL.
func TestUndoDequeue(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)

	seedTrack(t, d, "hs2", "/f2")
	now := time.Now().Unix()
	res, err := d.Exec(
		`INSERT INTO queue (track_hash, position, added_at, deleted_at) VALUES ('hs2', 1000, ?, ?)`,
		now, now,
	)
	if err != nil {
		t.Fatalf("insert queue: %v", err)
	}
	id, _ := res.LastInsertId()

	if err := w.UndoDequeue(id); err != nil {
		t.Fatalf("UndoDequeue: %v", err)
	}

	var deletedAt sql.NullInt64
	if err := d.QueryRow(`SELECT deleted_at FROM queue WHERE id=?`, id).Scan(&deletedAt); err != nil {
		t.Fatalf("query: %v", err)
	}
	if deletedAt.Valid {
		t.Fatalf("deleted_at = %d after UndoDequeue; want NULL", deletedAt.Int64)
	}
}

// TestClearQueueSoft verifies that ClearQueueSoft sets deleted_at on all active
// rows and returns their IDs, leaving already soft-deleted rows unchanged.
func TestClearQueueSoft(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)

	seedTrack(t, d, "hs5", "/f5")
	now := time.Now().Unix()

	var activeIDs []int64
	for _, pos := range []int{1000, 2000} {
		res, err := d.Exec(
			`INSERT INTO queue (track_hash, position, added_at) VALUES ('hs5', ?, ?)`, pos, now,
		)
		if err != nil {
			t.Fatalf("insert queue pos %d: %v", pos, err)
		}
		id, _ := res.LastInsertId()
		activeIDs = append(activeIDs, id)
	}

	// One already soft-deleted row — must not be included in the returned IDs.
	res, err := d.Exec(
		`INSERT INTO queue (track_hash, position, added_at, deleted_at) VALUES ('hs5', 3000, ?, ?)`,
		now, now,
	)
	if err != nil {
		t.Fatalf("insert soft-deleted row: %v", err)
	}
	alreadyDeletedID, _ := res.LastInsertId()

	ids, err := w.ClearQueueSoft()
	if err != nil {
		t.Fatalf("ClearQueueSoft: %v", err)
	}

	slices.Sort(ids)
	slices.Sort(activeIDs)
	if !slices.Equal(ids, activeIDs) {
		t.Errorf("returned IDs = %v; want %v", ids, activeIDs)
	}

	for _, id := range ids {
		var deletedAt sql.NullInt64
		if err := d.QueryRow(`SELECT deleted_at FROM queue WHERE id=?`, id).Scan(&deletedAt); err != nil {
			t.Fatalf("query id %d: %v", id, err)
		}
		if !deletedAt.Valid {
			t.Errorf("id %d: deleted_at is NULL after ClearQueueSoft; want non-NULL", id)
		}
	}

	// The already-deleted row must be unchanged.
	var deletedAt sql.NullInt64
	if err := d.QueryRow(`SELECT deleted_at FROM queue WHERE id=?`, alreadyDeletedID).Scan(&deletedAt); err != nil {
		t.Fatalf("query already-deleted row: %v", err)
	}
	if !deletedAt.Valid {
		t.Error("previously soft-deleted row lost its deleted_at after ClearQueueSoft")
	}
}

// TestClearQueueSoft_Empty verifies that ClearQueueSoft on an empty queue
// returns an empty slice without error.
func TestClearQueueSoft_Empty(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)

	ids, err := w.ClearQueueSoft()
	if err != nil {
		t.Fatalf("ClearQueueSoft: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("got %v; want empty slice", ids)
	}
}

// TestUndoClearQueue verifies that UndoClearQueue clears deleted_at on all
// provided IDs inside a single transaction.
func TestUndoClearQueue(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)

	seedTrack(t, d, "hs6", "/f6")
	now := time.Now().Unix()

	var ids []int64
	for _, pos := range []int{1000, 2000, 3000} {
		res, err := d.Exec(
			`INSERT INTO queue (track_hash, position, added_at, deleted_at) VALUES ('hs6', ?, ?, ?)`,
			pos, now, now,
		)
		if err != nil {
			t.Fatalf("insert soft-deleted row pos %d: %v", pos, err)
		}
		id, _ := res.LastInsertId()
		ids = append(ids, id)
	}

	if err := w.UndoClearQueue(ids); err != nil {
		t.Fatalf("UndoClearQueue: %v", err)
	}

	for _, id := range ids {
		var deletedAt sql.NullInt64
		if err := d.QueryRow(`SELECT deleted_at FROM queue WHERE id=?`, id).Scan(&deletedAt); err != nil {
			t.Fatalf("query id %d: %v", id, err)
		}
		if deletedAt.Valid {
			t.Errorf("id %d: deleted_at = %d after UndoClearQueue; want NULL", id, deletedAt.Int64)
		}
	}
}

// TestUndoClearQueue_Empty verifies that UndoClearQueue with no IDs is a no-op.
func TestUndoClearQueue_Empty(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)

	if err := w.UndoClearQueue(nil); err != nil {
		t.Fatalf("UndoClearQueue(nil): %v", err)
	}
}

// TestPurgeDeletedQueueEntries verifies that rows with deleted_at before the
// cutoff are physically removed while rows at/after the cutoff and active rows
// are retained.
func TestPurgeDeletedQueueEntries(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)

	seedTrack(t, d, "hs7", "/f7")
	base := time.Now().Unix()

	// Row deleted before cutoff — should be purged.
	res, err := d.Exec(
		`INSERT INTO queue (track_hash, position, added_at, deleted_at) VALUES ('hs7', 1000, ?, ?)`,
		base, base-10,
	)
	if err != nil {
		t.Fatalf("insert expired row: %v", err)
	}
	expiredID, _ := res.LastInsertId()

	// Row deleted after cutoff — should be retained.
	res, err = d.Exec(
		`INSERT INTO queue (track_hash, position, added_at, deleted_at) VALUES ('hs7', 2000, ?, ?)`,
		base, base+10,
	)
	if err != nil {
		t.Fatalf("insert recent row: %v", err)
	}
	recentID, _ := res.LastInsertId()

	// Active row — should be retained.
	res, err = d.Exec(
		`INSERT INTO queue (track_hash, position, added_at) VALUES ('hs7', 3000, ?)`, base,
	)
	if err != nil {
		t.Fatalf("insert active row: %v", err)
	}
	activeID, _ := res.LastInsertId()

	if err := w.PurgeDeletedQueueEntries(base); err != nil {
		t.Fatalf("PurgeDeletedQueueEntries: %v", err)
	}

	// Expired row must be gone.
	var count int
	if err := d.QueryRow(`SELECT COUNT(*) FROM queue WHERE id=?`, expiredID).Scan(&count); err != nil {
		t.Fatalf("query expired row: %v", err)
	}
	if count != 0 {
		t.Errorf("expired row still present after purge; want removed")
	}

	// Recent and active rows must remain.
	for _, id := range []int64{recentID, activeID} {
		if err := d.QueryRow(`SELECT COUNT(*) FROM queue WHERE id=?`, id).Scan(&count); err != nil {
			t.Fatalf("query row %d: %v", id, err)
		}
		if count != 1 {
			t.Errorf("row %d: count = %d after purge; want 1", id, count)
		}
	}
}

// TestRenumberQueuePositions_NoOp verifies that RenumberQueuePositions makes
// no changes when all adjacent gaps are >= 1.
func TestRenumberQueuePositions_NoOp(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)

	seedTrack(t, d, "hs3", "/f3")
	now := time.Now().Unix()
	for _, pos := range []int{1000, 2000, 3000} {
		if _, err := d.Exec(
			`INSERT INTO queue (track_hash, position, added_at) VALUES ('hs3', ?, ?)`, pos, now,
		); err != nil {
			t.Fatalf("insert queue pos %d: %v", pos, err)
		}
	}

	if err := w.RenumberQueuePositions(); err != nil {
		t.Fatalf("RenumberQueuePositions: %v", err)
	}

	rows, err := d.Query(`SELECT position FROM queue ORDER BY position`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()
	var got []int64
	for rows.Next() {
		var p int64
		if err := rows.Scan(&p); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got = append(got, p)
	}
	if want := []int64{1000, 2000, 3000}; !slices.Equal(got, want) {
		t.Errorf("positions = %v; want %v (no-op expected)", got, want)
	}
}

// TestRenumberQueuePositions_Triggered verifies that RenumberQueuePositions
// rewrites all active positions as 1000, 2000, 3000… when an adjacent gap is
// below 1. Soft-deleted rows are excluded from renumbering.
func TestRenumberQueuePositions_Triggered(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)

	seedTrack(t, d, "hs4", "/f4")
	now := time.Now().Unix()

	// Drop the unique position index so we can insert two rows at the same
	// position (gap = 0), simulating exhausted gap space.
	if _, err := d.Exec(`DROP INDEX IF EXISTS queue_pos_idx`); err != nil {
		t.Fatalf("drop index: %v", err)
	}

	// Three active rows: two at position 1000 (gap = 0) and one at 2000.
	for _, pos := range []int{1000, 1000, 2000} {
		if _, err := d.Exec(
			`INSERT INTO queue (track_hash, position, added_at) VALUES ('hs4', ?, ?)`, pos, now,
		); err != nil {
			t.Fatalf("insert queue pos %d: %v", pos, err)
		}
	}
	// One soft-deleted row that must not be renumbered.
	if _, err := d.Exec(
		`INSERT INTO queue (track_hash, position, added_at, deleted_at) VALUES ('hs4', 9000, ?, ?)`,
		now, now,
	); err != nil {
		t.Fatalf("insert soft-deleted row: %v", err)
	}

	if err := w.RenumberQueuePositions(); err != nil {
		t.Fatalf("RenumberQueuePositions: %v", err)
	}

	rows, err := d.Query(`SELECT position, deleted_at IS NULL FROM queue ORDER BY position`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()

	var activePos []int64
	var deletedPos []int64
	for rows.Next() {
		var pos int64
		var active int
		if err := rows.Scan(&pos, &active); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if active == 1 {
			activePos = append(activePos, pos)
		} else {
			deletedPos = append(deletedPos, pos)
		}
	}

	if want := []int64{1000, 2000, 3000}; !slices.Equal(activePos, want) {
		t.Errorf("active positions = %v; want %v", activePos, want)
	}
	// Soft-deleted row must retain its original position.
	if want := []int64{9000}; !slices.Equal(deletedPos, want) {
		t.Errorf("soft-deleted positions = %v; want %v", deletedPos, want)
	}
}
