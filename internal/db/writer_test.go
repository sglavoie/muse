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
	"github.com/sglavoie/muse/internal/library"
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
// writer goroutine to exit and Wait returns. This test specifically covers
// clean shutdown with no pending writes buffered in cmds; see
// TestWriter_DrainOnShutdown for the pending-writes drain scenario.
func TestWriter_ShutdownClean(t *testing.T) {
	d := openTestDB(t)

	ctx, cancel := context.WithCancel(t.Context())
	w := db.NewWriter(ctx, d.DB)

	cancel()
	// Wait must return after cancellation; if it hangs the test times out.
	w.Wait()
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

	// The first command occupies the writer goroutine (via gate) so that all
	// subsequent commands buffer in the 64-entry cmds channel.
	writerBusy := make(chan struct{})
	gate := make(chan struct{})

	var wg sync.WaitGroup
	wg.Go(func() {
		_ = w.Exec(func(sdb *sql.DB) error {
			close(writerBusy) // writer goroutine is now inside this fn
			<-gate            // block until the test releases it
			_, err := sdb.Exec("INSERT INTO items VALUES (0)")
			return err
		})
	})

	<-writerBusy // writer goroutine is blocked; cmds channel is ready to buffer

	// Submit n-1 goroutines. Because the writer goroutine is busy, their
	// sends to the buffered cmds channel complete immediately. submitted
	// tracks when each goroutine is scheduled and about to call Exec.
	var submitted sync.WaitGroup
	for i := 1; i < n; i++ {
		wg.Add(1)
		submitted.Add(1)
		val := i
		go func() {
			defer wg.Done()
			submitted.Done() // goroutine is running; cmds send is next
			_ = w.Exec(func(sdb *sql.DB) error {
				_, err := sdb.Exec("INSERT INTO items VALUES (?)", val)
				return err
			})
		}()
	}

	// On GOMAXPROCS=1 goroutines block at the resp select after sending to
	// cmds, so the scheduler returns here only after all sends complete.
	// On GOMAXPROCS>1 the window between submitted.Done() and the cmds send
	// is negligible compared with the time needed to unblock this goroutine.
	submitted.Wait()

	// Unblock the writer and cancel. After processing cmd 0 the writer sees
	// ctx.Done() and calls drain(), which flushes the n-1 buffered commands.
	close(gate)
	cancel()

	wg.Wait()
	w.Wait()

	var count int
	if err := d.QueryRow("SELECT COUNT(*) FROM items").Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != n {
		t.Errorf("DB has %d rows; want %d – drain() lost pending writes on shutdown", count, n)
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

// TestDequeueTrackSoftDoesNotAffectPlaylists verifies that DequeueTrackSoft
// only soft-deletes the queue row and leaves any playlist_tracks rows for the
// same track hash completely untouched.
func TestDequeueTrackSoftDoesNotAffectPlaylists(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)

	seedTrack(t, d, "hs_pt1", "/fpt1")
	now := time.Now().Unix()

	// Create a playlist to satisfy the playlist_tracks FK constraint.
	res, err := d.Exec(
		`INSERT INTO playlists (name, created_at, updated_at) VALUES ('test', ?, ?)`,
		now, now,
	)
	if err != nil {
		t.Fatalf("insert playlist: %v", err)
	}
	playlistID, _ := res.LastInsertId()

	// Insert a queue row for the track.
	res, err = d.Exec(
		`INSERT INTO queue (track_hash, position, added_at) VALUES ('hs_pt1', 1000, ?)`, now,
	)
	if err != nil {
		t.Fatalf("insert queue: %v", err)
	}
	queueID, _ := res.LastInsertId()

	// Insert a playlist_tracks row for the same track hash.
	if _, err := d.Exec(
		`INSERT INTO playlist_tracks (playlist_id, track_hash, position, added_at) VALUES (?, 'hs_pt1', 1000, ?)`,
		playlistID, now,
	); err != nil {
		t.Fatalf("insert playlist_tracks: %v", err)
	}

	// Soft-delete the queue row.
	if err := w.DequeueTrackSoft(queueID); err != nil {
		t.Fatalf("DequeueTrackSoft: %v", err)
	}

	// playlist_tracks row must still exist with deleted_at IS NULL.
	var count int
	var deletedAt sql.NullInt64
	if err := d.QueryRow(
		`SELECT COUNT(*), MAX(deleted_at) FROM playlist_tracks WHERE track_hash='hs_pt1'`,
	).Scan(&count, &deletedAt); err != nil {
		t.Fatalf("query playlist_tracks: %v", err)
	}
	if count != 1 {
		t.Errorf("playlist_tracks count = %d; want 1 (row must not be removed)", count)
	}
	if deletedAt.Valid {
		t.Errorf("playlist_tracks.deleted_at = %d; want NULL (row must not be soft-deleted)", deletedAt.Int64)
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

// TestWriteShuffle_Enable verifies that WriteShuffle(true) assigns non-zero
// shuffle_position values to all active rows inside a single transaction and
// writes shuffle=1 to the session row. Soft-deleted rows are not reordered.
func TestWriteShuffle_Enable(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)

	seedTrack(t, d, "hs10", "/f10")
	now := time.Now().Unix()

	var activeIDs []int64
	for _, pos := range []int{1000, 2000, 3000} {
		res, err := d.Exec(
			`INSERT INTO queue (track_hash, position, added_at) VALUES ('hs10', ?, ?)`, pos, now,
		)
		if err != nil {
			t.Fatalf("insert active row pos %d: %v", pos, err)
		}
		id, _ := res.LastInsertId()
		activeIDs = append(activeIDs, id)
	}

	// One soft-deleted row — must not be touched.
	res, err := d.Exec(
		`INSERT INTO queue (track_hash, position, added_at, deleted_at) VALUES ('hs10', 4000, ?, ?)`,
		now, now,
	)
	if err != nil {
		t.Fatalf("insert soft-deleted row: %v", err)
	}
	deletedID, _ := res.LastInsertId()

	if err := w.WriteShuffle(true); err != nil {
		t.Fatalf("WriteShuffle(true): %v", err)
	}

	// All active rows must have a positive shuffle_position.
	for _, id := range activeIDs {
		var sp int64
		if err := d.QueryRow(`SELECT shuffle_position FROM queue WHERE id=?`, id).Scan(&sp); err != nil {
			t.Fatalf("query shuffle_position for id %d: %v", id, err)
		}
		if sp <= 0 {
			t.Errorf("id %d: shuffle_position = %d; want > 0", id, sp)
		}
	}

	// Soft-deleted row must keep shuffle_position = 0 (default).
	var sp int64
	if err := d.QueryRow(`SELECT shuffle_position FROM queue WHERE id=?`, deletedID).Scan(&sp); err != nil {
		t.Fatalf("query deleted row: %v", err)
	}
	if sp != 0 {
		t.Errorf("soft-deleted row shuffle_position = %d; want 0", sp)
	}

	// Session must have shuffle=1.
	var shuffle int
	if err := d.QueryRow(`SELECT shuffle FROM session WHERE id=1`).Scan(&shuffle); err != nil {
		t.Fatalf("query session: %v", err)
	}
	if shuffle != 1 {
		t.Errorf("session.shuffle = %d; want 1", shuffle)
	}
}

// TestWriteShuffle_Disable verifies that WriteShuffle(false) writes shuffle=0
// to the session row without modifying shuffle_position values.
func TestWriteShuffle_Disable(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)

	seedTrack(t, d, "hs11", "/f11")
	now := time.Now().Unix()

	// Insert a row with a non-default shuffle_position.
	res, err := d.Exec(
		`INSERT INTO queue (track_hash, position, shuffle_position, added_at) VALUES ('hs11', 1000, 5000, ?)`, now,
	)
	if err != nil {
		t.Fatalf("insert queue row: %v", err)
	}
	id, _ := res.LastInsertId()

	// Set session shuffle=1 first.
	if _, err := d.Exec(`UPDATE session SET shuffle=1, updated_at=? WHERE id=1`, now); err != nil {
		t.Fatalf("set session shuffle: %v", err)
	}

	if err := w.WriteShuffle(false); err != nil {
		t.Fatalf("WriteShuffle(false): %v", err)
	}

	// Session must have shuffle=0.
	var shuffle int
	if err := d.QueryRow(`SELECT shuffle FROM session WHERE id=1`).Scan(&shuffle); err != nil {
		t.Fatalf("query session: %v", err)
	}
	if shuffle != 0 {
		t.Errorf("session.shuffle = %d; want 0", shuffle)
	}

	// shuffle_position must be unchanged.
	var sp int64
	if err := d.QueryRow(`SELECT shuffle_position FROM queue WHERE id=?`, id).Scan(&sp); err != nil {
		t.Fatalf("query shuffle_position: %v", err)
	}
	if sp != 5000 {
		t.Errorf("shuffle_position = %d; want 5000 (unchanged)", sp)
	}
}

// TestWriteShuffle_TwoEnablesProduceDifferentOrderings verifies that calling
// WriteShuffle(true) twice produces different shuffle_position assignments.
// This test is probabilistic: a false positive is astronomically unlikely with
// 5+ rows (1/5! ≈ 0.8% chance of identical ordering).
func TestWriteShuffle_TwoEnablesProduceDifferentOrderings(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)

	seedTrack(t, d, "hs12", "/f12")
	now := time.Now().Unix()

	var ids []int64
	for _, pos := range []int{1000, 2000, 3000, 4000, 5000} {
		res, err := d.Exec(
			`INSERT INTO queue (track_hash, position, added_at) VALUES ('hs12', ?, ?)`, pos, now,
		)
		if err != nil {
			t.Fatalf("insert row pos %d: %v", pos, err)
		}
		id, _ := res.LastInsertId()
		ids = append(ids, id)
	}

	readPositions := func() []int64 {
		var positions []int64
		for _, id := range ids {
			var sp int64
			if err := d.QueryRow(`SELECT shuffle_position FROM queue WHERE id=?`, id).Scan(&sp); err != nil {
				t.Fatalf("query shuffle_position for id %d: %v", id, err)
			}
			positions = append(positions, sp)
		}
		return positions
	}

	if err := w.WriteShuffle(true); err != nil {
		t.Fatalf("first WriteShuffle(true): %v", err)
	}
	first := readPositions()

	if err := w.WriteShuffle(true); err != nil {
		t.Fatalf("second WriteShuffle(true): %v", err)
	}
	second := readPositions()

	if slices.Equal(first, second) {
		t.Errorf("two consecutive enables produced identical shuffle_position ordering %v; expected different", first)
	}
}

// TestWriteShuffle_ShufflePositionDiffersFromPosition verifies that after
// WriteShuffle(true), the shuffle_position ordering is distinct from the
// original position ordering, confirming that shuffle_position is freshly
// generated and not simply copied from the position column.
// The test uses 6 rows to make a false positive astronomically unlikely (1/720).
func TestWriteShuffle_ShufflePositionDiffersFromPosition(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)

	seedTrack(t, d, "hs13", "/f13")
	now := time.Now().Unix()

	positions := []int{1000, 2000, 3000, 4000, 5000, 6000}
	var ids []int64
	for _, pos := range positions {
		res, err := d.Exec(
			`INSERT INTO queue (track_hash, position, added_at) VALUES ('hs13', ?, ?)`, pos, now,
		)
		if err != nil {
			t.Fatalf("insert row pos %d: %v", pos, err)
		}
		id, _ := res.LastInsertId()
		ids = append(ids, id)
	}

	if err := w.WriteShuffle(true); err != nil {
		t.Fatalf("WriteShuffle(true): %v", err)
	}

	// Collect (position, shuffle_position) pairs for all rows in insertion order.
	allMatch := true
	for _, id := range ids {
		var pos, sp int64
		if err := d.QueryRow(`SELECT position, shuffle_position FROM queue WHERE id=?`, id).Scan(&pos, &sp); err != nil {
			t.Fatalf("query row %d: %v", id, err)
		}
		if pos != sp {
			allMatch = false
		}
	}
	if allMatch {
		t.Error("shuffle_position values are identical to position values; expected a different ordering")
	}
}

// TestUpsertSession verifies that UpsertSession writes all session fields and
// that a second call overwrites the previous values.
func TestUpsertSession(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)

	seedTrack(t, d, "hashA", "/trackA")

	if err := w.UpsertSession("hashA", 12345, true, "all"); err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}

	var (
		hash       sql.NullString
		positionMs int64
		shuffle    int
		repeatMode string
	)
	if err := d.QueryRow(
		`SELECT current_hash, position_ms, shuffle, repeat_mode FROM session WHERE id=1`,
	).Scan(&hash, &positionMs, &shuffle, &repeatMode); err != nil {
		t.Fatalf("query session: %v", err)
	}
	if !hash.Valid || hash.String != "hashA" {
		t.Errorf("current_hash = %v; want hashA", hash)
	}
	if positionMs != 12345 {
		t.Errorf("position_ms = %d; want 12345", positionMs)
	}
	if shuffle != 1 {
		t.Errorf("shuffle = %d; want 1", shuffle)
	}
	if repeatMode != "all" {
		t.Errorf("repeat_mode = %q; want all", repeatMode)
	}

	// Second call overwrites with new values.
	if err := w.UpsertSession("", 0, false, "none"); err != nil {
		t.Fatalf("UpsertSession (second): %v", err)
	}
	if err := d.QueryRow(
		`SELECT current_hash, position_ms, shuffle, repeat_mode FROM session WHERE id=1`,
	).Scan(&hash, &positionMs, &shuffle, &repeatMode); err != nil {
		t.Fatalf("query session after second call: %v", err)
	}
	if hash.Valid {
		t.Errorf("current_hash = %v; want NULL", hash)
	}
	if positionMs != 0 {
		t.Errorf("position_ms = %d; want 0", positionMs)
	}
	if shuffle != 0 {
		t.Errorf("shuffle = %d; want 0", shuffle)
	}
	if repeatMode != "none" {
		t.Errorf("repeat_mode = %q; want none", repeatMode)
	}
}

// TestUpsertSession_PropagatesError verifies that a DB error (e.g. invalid
// repeat_mode value violating the CHECK constraint) is returned to the caller.
func TestUpsertSession_PropagatesError(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)

	err := w.UpsertSession("", 0, false, "invalid_mode")
	if err == nil {
		t.Fatal("expected error for invalid repeat_mode CHECK violation, got nil")
	}
}

// TestUpdateSessionPosition verifies that UpdateSessionPosition updates only
// current_hash and position_ms, leaving shuffle and repeat_mode unchanged.
func TestUpdateSessionPosition(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)

	seedTrack(t, d, "hashB", "/trackB")

	// Set up a known state first.
	if err := w.UpsertSession("hashB", 1000, true, "one"); err != nil {
		t.Fatalf("UpsertSession setup: %v", err)
	}

	if err := w.UpdateSessionPosition("hashB", 9999); err != nil {
		t.Fatalf("UpdateSessionPosition: %v", err)
	}

	var (
		hash       sql.NullString
		positionMs int64
		shuffle    int
		repeatMode string
	)
	if err := d.QueryRow(
		`SELECT current_hash, position_ms, shuffle, repeat_mode FROM session WHERE id=1`,
	).Scan(&hash, &positionMs, &shuffle, &repeatMode); err != nil {
		t.Fatalf("query session: %v", err)
	}
	if !hash.Valid || hash.String != "hashB" {
		t.Errorf("current_hash = %v; want hashB", hash)
	}
	if positionMs != 9999 {
		t.Errorf("position_ms = %d; want 9999", positionMs)
	}
	// shuffle and repeat_mode must be untouched.
	if shuffle != 1 {
		t.Errorf("shuffle = %d; want 1 (unchanged)", shuffle)
	}
	if repeatMode != "one" {
		t.Errorf("repeat_mode = %q; want one (unchanged)", repeatMode)
	}
}

// TestUpdateSessionPosition_NullHash verifies that an empty hash stores NULL.
func TestUpdateSessionPosition_NullHash(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)

	if err := w.UpdateSessionPosition("", 500); err != nil {
		t.Fatalf("UpdateSessionPosition: %v", err)
	}

	var hash sql.NullString
	if err := d.QueryRow(`SELECT current_hash FROM session WHERE id=1`).Scan(&hash); err != nil {
		t.Fatalf("query session: %v", err)
	}
	if hash.Valid {
		t.Errorf("current_hash = %v; want NULL for empty hash arg", hash)
	}
}

// seedPlaylist inserts a minimal playlists row and returns its ID.
func seedPlaylist(t *testing.T, d *db.DB, name string) int64 {
	t.Helper()
	now := time.Now().Unix()
	res, err := d.Exec(
		`INSERT INTO playlists (name, created_at, updated_at) VALUES (?, ?, ?)`,
		name, now, now,
	)
	if err != nil {
		t.Fatalf("seedPlaylist %q: %v", name, err)
	}
	id, _ := res.LastInsertId()
	return id
}

// seedPlaylistTrack inserts a playlist_tracks row and returns its ID.
func seedPlaylistTrack(t *testing.T, d *db.DB, playlistID int64, hash string, position int64) int64 {
	t.Helper()
	res, err := d.Exec(
		`INSERT INTO playlist_tracks (playlist_id, track_hash, position, added_at) VALUES (?, ?, ?, ?)`,
		playlistID, hash, position, time.Now().Unix(),
	)
	if err != nil {
		t.Fatalf("seedPlaylistTrack: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

// TestCreatePlaylist verifies that CreatePlaylist inserts a row and returns a
// valid ID, and that the playlist name is persisted correctly.
func TestCreatePlaylist(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)

	id, err := w.CreatePlaylist("My Playlist")
	if err != nil {
		t.Fatalf("CreatePlaylist: %v", err)
	}
	if id <= 0 {
		t.Fatalf("CreatePlaylist returned id=%d; want > 0", id)
	}

	var name string
	var deletedAt sql.NullInt64
	if err := d.QueryRow(`SELECT name, deleted_at FROM playlists WHERE id=?`, id).Scan(&name, &deletedAt); err != nil {
		t.Fatalf("query playlist: %v", err)
	}
	if name != "My Playlist" {
		t.Errorf("name = %q; want %q", name, "My Playlist")
	}
	if deletedAt.Valid {
		t.Errorf("deleted_at = %d; want NULL (playlist should be active)", deletedAt.Int64)
	}
}

// TestRenamePlaylist verifies that RenamePlaylist updates the playlist name.
func TestRenamePlaylist(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)

	id := seedPlaylist(t, d, "Old Name")

	if err := w.RenamePlaylist(id, "New Name"); err != nil {
		t.Fatalf("RenamePlaylist: %v", err)
	}

	var name string
	if err := d.QueryRow(`SELECT name FROM playlists WHERE id=?`, id).Scan(&name); err != nil {
		t.Fatalf("query: %v", err)
	}
	if name != "New Name" {
		t.Errorf("name = %q; want %q", name, "New Name")
	}
}

// TestDeletePlaylistSoft verifies that DeletePlaylistSoft sets deleted_at.
func TestDeletePlaylistSoft(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)

	id := seedPlaylist(t, d, "To Delete")

	if err := w.DeletePlaylistSoft(id); err != nil {
		t.Fatalf("DeletePlaylistSoft: %v", err)
	}

	var deletedAt sql.NullInt64
	if err := d.QueryRow(`SELECT deleted_at FROM playlists WHERE id=?`, id).Scan(&deletedAt); err != nil {
		t.Fatalf("query: %v", err)
	}
	if !deletedAt.Valid {
		t.Fatal("deleted_at is NULL after DeletePlaylistSoft; want non-NULL")
	}
}

// TestUndoDeletePlaylist verifies that UndoDeletePlaylist clears deleted_at.
func TestUndoDeletePlaylist(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)

	id := seedPlaylist(t, d, "To Restore")
	now := time.Now().Unix()
	if _, err := d.Exec(`UPDATE playlists SET deleted_at=? WHERE id=?`, now, id); err != nil {
		t.Fatalf("set deleted_at: %v", err)
	}

	if err := w.UndoDeletePlaylist(id); err != nil {
		t.Fatalf("UndoDeletePlaylist: %v", err)
	}

	var deletedAt sql.NullInt64
	if err := d.QueryRow(`SELECT deleted_at FROM playlists WHERE id=?`, id).Scan(&deletedAt); err != nil {
		t.Fatalf("query: %v", err)
	}
	if deletedAt.Valid {
		t.Errorf("deleted_at = %d after UndoDeletePlaylist; want NULL", deletedAt.Int64)
	}
}

// TestAddTracksToPlaylist verifies that tracks are appended with gapped
// positions (1000, 2000, …) and that a second call continues the sequence.
func TestAddTracksToPlaylist(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)

	seedTrack(t, d, "ta1", "/a1")
	seedTrack(t, d, "ta2", "/a2")
	seedTrack(t, d, "ta3", "/a3")
	plID := seedPlaylist(t, d, "Playlist A")

	if err := w.AddTracksToPlaylist(plID, []string{"ta1", "ta2"}); err != nil {
		t.Fatalf("AddTracksToPlaylist (first call): %v", err)
	}

	rows, err := d.Query(
		`SELECT position FROM playlist_tracks WHERE playlist_id=? ORDER BY position`, plID,
	)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()
	var positions []int64
	for rows.Next() {
		var p int64
		if err := rows.Scan(&p); err != nil {
			t.Fatalf("scan: %v", err)
		}
		positions = append(positions, p)
	}
	if want := []int64{1000, 2000}; !slices.Equal(positions, want) {
		t.Errorf("positions after first add = %v; want %v", positions, want)
	}

	// Second call must continue after the existing max.
	if err := w.AddTracksToPlaylist(plID, []string{"ta3"}); err != nil {
		t.Fatalf("AddTracksToPlaylist (second call): %v", err)
	}

	rows2, err := d.Query(
		`SELECT position FROM playlist_tracks WHERE playlist_id=? ORDER BY position`, plID,
	)
	if err != nil {
		t.Fatalf("query after second add: %v", err)
	}
	defer rows2.Close()
	positions = positions[:0]
	for rows2.Next() {
		var p int64
		if err := rows2.Scan(&p); err != nil {
			t.Fatalf("scan: %v", err)
		}
		positions = append(positions, p)
	}
	if want := []int64{1000, 2000, 3000}; !slices.Equal(positions, want) {
		t.Errorf("positions after second add = %v; want %v", positions, want)
	}
}

// TestAddTracksToPlaylist_Empty verifies that AddTracksToPlaylist is a no-op
// when called with an empty slice.
func TestAddTracksToPlaylist_Empty(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)

	plID := seedPlaylist(t, d, "Empty Playlist")

	if err := w.AddTracksToPlaylist(plID, nil); err != nil {
		t.Fatalf("AddTracksToPlaylist(nil): %v", err)
	}

	var count int
	if err := d.QueryRow(`SELECT COUNT(*) FROM playlist_tracks WHERE playlist_id=?`, plID).Scan(&count); err != nil {
		t.Fatalf("query: %v", err)
	}
	if count != 0 {
		t.Errorf("count = %d after empty add; want 0", count)
	}
}

// TestRemovePlaylistTrackSoft verifies that RemovePlaylistTrackSoft sets
// deleted_at on the playlist_tracks row.
func TestRemovePlaylistTrackSoft(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)

	seedTrack(t, d, "tr1", "/r1")
	plID := seedPlaylist(t, d, "Playlist R")
	ptID := seedPlaylistTrack(t, d, plID, "tr1", 1000)

	if err := w.RemovePlaylistTrackSoft(ptID); err != nil {
		t.Fatalf("RemovePlaylistTrackSoft: %v", err)
	}

	var deletedAt sql.NullInt64
	if err := d.QueryRow(`SELECT deleted_at FROM playlist_tracks WHERE id=?`, ptID).Scan(&deletedAt); err != nil {
		t.Fatalf("query: %v", err)
	}
	if !deletedAt.Valid {
		t.Fatal("deleted_at is NULL after RemovePlaylistTrackSoft; want non-NULL")
	}
}

// TestUndoRemovePlaylistTrack verifies that UndoRemovePlaylistTrack clears
// deleted_at.
func TestUndoRemovePlaylistTrack(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)

	seedTrack(t, d, "tu1", "/u1")
	plID := seedPlaylist(t, d, "Playlist U")
	ptID := seedPlaylistTrack(t, d, plID, "tu1", 1000)

	now := time.Now().Unix()
	if _, err := d.Exec(`UPDATE playlist_tracks SET deleted_at=? WHERE id=?`, now, ptID); err != nil {
		t.Fatalf("set deleted_at: %v", err)
	}

	if err := w.UndoRemovePlaylistTrack(ptID); err != nil {
		t.Fatalf("UndoRemovePlaylistTrack: %v", err)
	}

	var deletedAt sql.NullInt64
	if err := d.QueryRow(`SELECT deleted_at FROM playlist_tracks WHERE id=?`, ptID).Scan(&deletedAt); err != nil {
		t.Fatalf("query: %v", err)
	}
	if deletedAt.Valid {
		t.Errorf("deleted_at = %d after UndoRemovePlaylistTrack; want NULL", deletedAt.Int64)
	}
}

// TestReorderPlaylistTrack_Down verifies that direction=+1 swaps the track
// with its successor, producing the expected position values.
func TestReorderPlaylistTrack_Down(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)

	seedTrack(t, d, "trd1", "/d1")
	seedTrack(t, d, "trd2", "/d2")
	seedTrack(t, d, "trd3", "/d3")
	plID := seedPlaylist(t, d, "Playlist D")
	id1 := seedPlaylistTrack(t, d, plID, "trd1", 1000)
	id2 := seedPlaylistTrack(t, d, plID, "trd2", 2000)
	_ = seedPlaylistTrack(t, d, plID, "trd3", 3000)

	// Move track at position 1000 down — it should swap with the track at 2000.
	if err := w.ReorderPlaylistTrack(plID, id1, +1); err != nil {
		t.Fatalf("ReorderPlaylistTrack down: %v", err)
	}

	var p1, p2 int64
	if err := d.QueryRow(`SELECT position FROM playlist_tracks WHERE id=?`, id1).Scan(&p1); err != nil {
		t.Fatalf("query id1: %v", err)
	}
	if err := d.QueryRow(`SELECT position FROM playlist_tracks WHERE id=?`, id2).Scan(&p2); err != nil {
		t.Fatalf("query id2: %v", err)
	}
	if p1 != 2000 || p2 != 1000 {
		t.Errorf("after move-down: id1.position=%d id2.position=%d; want 2000/1000", p1, p2)
	}
}

// TestReorderPlaylistTrack_Up verifies that direction=-1 swaps the track with
// its predecessor.
func TestReorderPlaylistTrack_Up(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)

	seedTrack(t, d, "tru1", "/u1")
	seedTrack(t, d, "tru2", "/u2")
	plID := seedPlaylist(t, d, "Playlist U2")
	id1 := seedPlaylistTrack(t, d, plID, "tru1", 1000)
	id2 := seedPlaylistTrack(t, d, plID, "tru2", 2000)

	// Move track at position 2000 up.
	if err := w.ReorderPlaylistTrack(plID, id2, -1); err != nil {
		t.Fatalf("ReorderPlaylistTrack up: %v", err)
	}

	var p1, p2 int64
	if err := d.QueryRow(`SELECT position FROM playlist_tracks WHERE id=?`, id1).Scan(&p1); err != nil {
		t.Fatalf("query id1: %v", err)
	}
	if err := d.QueryRow(`SELECT position FROM playlist_tracks WHERE id=?`, id2).Scan(&p2); err != nil {
		t.Fatalf("query id2: %v", err)
	}
	if p1 != 2000 || p2 != 1000 {
		t.Errorf("after move-up: id1.position=%d id2.position=%d; want 2000/1000", p1, p2)
	}
}

// TestReorderPlaylistTrack_Boundary verifies that moving the first track up or
// the last track down is a no-op (positions unchanged).
func TestReorderPlaylistTrack_Boundary(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)

	seedTrack(t, d, "trb1", "/b1")
	seedTrack(t, d, "trb2", "/b2")
	plID := seedPlaylist(t, d, "Playlist B")
	id1 := seedPlaylistTrack(t, d, plID, "trb1", 1000)
	id2 := seedPlaylistTrack(t, d, plID, "trb2", 2000)

	// Move first track up — no-op.
	if err := w.ReorderPlaylistTrack(plID, id1, -1); err != nil {
		t.Fatalf("ReorderPlaylistTrack boundary up: %v", err)
	}
	// Move last track down — no-op.
	if err := w.ReorderPlaylistTrack(plID, id2, +1); err != nil {
		t.Fatalf("ReorderPlaylistTrack boundary down: %v", err)
	}

	var p1, p2 int64
	if err := d.QueryRow(`SELECT position FROM playlist_tracks WHERE id=?`, id1).Scan(&p1); err != nil {
		t.Fatalf("query id1: %v", err)
	}
	if err := d.QueryRow(`SELECT position FROM playlist_tracks WHERE id=?`, id2).Scan(&p2); err != nil {
		t.Fatalf("query id2: %v", err)
	}
	if p1 != 1000 || p2 != 2000 {
		t.Errorf("positions changed after boundary ops: id1=%d id2=%d; want 1000/2000", p1, p2)
	}
}

// TestReorderPlaylistTrack_SkipsSoftDeleted verifies that soft-deleted rows
// are skipped when finding the adjacent track for a swap.
func TestReorderPlaylistTrack_SkipsSoftDeleted(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)

	seedTrack(t, d, "trs1", "/s1")
	seedTrack(t, d, "trs2", "/s2")
	seedTrack(t, d, "trs3", "/s3")
	plID := seedPlaylist(t, d, "Playlist S")
	id1 := seedPlaylistTrack(t, d, plID, "trs1", 1000)
	id2 := seedPlaylistTrack(t, d, plID, "trs2", 2000) // will be soft-deleted
	id3 := seedPlaylistTrack(t, d, plID, "trs3", 3000)

	// Soft-delete the middle track.
	now := time.Now().Unix()
	if _, err := d.Exec(`UPDATE playlist_tracks SET deleted_at=? WHERE id=?`, now, id2); err != nil {
		t.Fatalf("soft-delete middle: %v", err)
	}

	// Move first track down — should swap with id3 (skipping soft-deleted id2).
	if err := w.ReorderPlaylistTrack(plID, id1, +1); err != nil {
		t.Fatalf("ReorderPlaylistTrack: %v", err)
	}

	var p1, p3 int64
	if err := d.QueryRow(`SELECT position FROM playlist_tracks WHERE id=?`, id1).Scan(&p1); err != nil {
		t.Fatalf("query id1: %v", err)
	}
	if err := d.QueryRow(`SELECT position FROM playlist_tracks WHERE id=?`, id3).Scan(&p3); err != nil {
		t.Fatalf("query id3: %v", err)
	}
	if p1 != 3000 || p3 != 1000 {
		t.Errorf("id1.position=%d id3.position=%d; want 3000/1000 (skipped soft-deleted)", p1, p3)
	}
}

// TestPurgeDeletedPlaylistTracks verifies that rows with deleted_at before the
// cutoff are removed, while rows at/after the cutoff and active rows remain.
func TestPurgeDeletedPlaylistTracks(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)

	seedTrack(t, d, "tp1", "/p1")
	plID := seedPlaylist(t, d, "Playlist P")
	base := time.Now().Unix()

	// Row deleted before cutoff — should be purged.
	expiredID := seedPlaylistTrack(t, d, plID, "tp1", 1000)
	if _, err := d.Exec(`UPDATE playlist_tracks SET deleted_at=? WHERE id=?`, base-10, expiredID); err != nil {
		t.Fatalf("set expired deleted_at: %v", err)
	}

	// Row deleted after cutoff — should be retained.
	recentID := seedPlaylistTrack(t, d, plID, "tp1", 2000)
	if _, err := d.Exec(`UPDATE playlist_tracks SET deleted_at=? WHERE id=?`, base+10, recentID); err != nil {
		t.Fatalf("set recent deleted_at: %v", err)
	}

	// Active row — should be retained.
	activeID := seedPlaylistTrack(t, d, plID, "tp1", 3000)

	if err := w.PurgeDeletedPlaylistTracks(base); err != nil {
		t.Fatalf("PurgeDeletedPlaylistTracks: %v", err)
	}

	var count int
	if err := d.QueryRow(`SELECT COUNT(*) FROM playlist_tracks WHERE id=?`, expiredID).Scan(&count); err != nil {
		t.Fatalf("query expired: %v", err)
	}
	if count != 0 {
		t.Errorf("expired row still present after purge; want removed")
	}
	for _, id := range []int64{recentID, activeID} {
		if err := d.QueryRow(`SELECT COUNT(*) FROM playlist_tracks WHERE id=?`, id).Scan(&count); err != nil {
			t.Fatalf("query row %d: %v", id, err)
		}
		if count != 1 {
			t.Errorf("row %d: count=%d after purge; want 1", id, count)
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

// seedQueueEntry inserts an active queue row at the given position and returns its id.
func seedQueueEntry(t *testing.T, d *db.DB, hash string, pos int64) int64 {
	t.Helper()
	now := time.Now().Unix()
	res, err := d.Exec(
		`INSERT INTO queue (track_hash, position, added_at) VALUES (?, ?, ?)`,
		hash, pos, now,
	)
	if err != nil {
		t.Fatalf("seedQueueEntry hash=%s pos=%d: %v", hash, pos, err)
	}
	id, _ := res.LastInsertId()
	return id
}

// TestReorderQueueTrack_Down verifies that direction=+1 swaps the track with
// its successor, producing the expected position values.
func TestReorderQueueTrack_Down(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)

	seedTrack(t, d, "qrd1", "/qd1")
	seedTrack(t, d, "qrd2", "/qd2")
	seedTrack(t, d, "qrd3", "/qd3")
	id1 := seedQueueEntry(t, d, "qrd1", 1000)
	id2 := seedQueueEntry(t, d, "qrd2", 2000)
	_ = seedQueueEntry(t, d, "qrd3", 3000)

	// Move track at position 1000 down — it should swap with the track at 2000.
	if err := w.ReorderQueueTrack(id1, +1); err != nil {
		t.Fatalf("ReorderQueueTrack down: %v", err)
	}

	var p1, p2 int64
	if err := d.QueryRow(`SELECT position FROM queue WHERE id=?`, id1).Scan(&p1); err != nil {
		t.Fatalf("query id1: %v", err)
	}
	if err := d.QueryRow(`SELECT position FROM queue WHERE id=?`, id2).Scan(&p2); err != nil {
		t.Fatalf("query id2: %v", err)
	}
	if p1 != 2000 || p2 != 1000 {
		t.Errorf("after move-down: id1.position=%d id2.position=%d; want 2000/1000", p1, p2)
	}
}

// TestReorderQueueTrack_Up verifies that direction=-1 swaps the track with its
// predecessor.
func TestReorderQueueTrack_Up(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)

	seedTrack(t, d, "qru1", "/qu1")
	seedTrack(t, d, "qru2", "/qu2")
	id1 := seedQueueEntry(t, d, "qru1", 1000)
	id2 := seedQueueEntry(t, d, "qru2", 2000)

	// Move track at position 2000 up.
	if err := w.ReorderQueueTrack(id2, -1); err != nil {
		t.Fatalf("ReorderQueueTrack up: %v", err)
	}

	var p1, p2 int64
	if err := d.QueryRow(`SELECT position FROM queue WHERE id=?`, id1).Scan(&p1); err != nil {
		t.Fatalf("query id1: %v", err)
	}
	if err := d.QueryRow(`SELECT position FROM queue WHERE id=?`, id2).Scan(&p2); err != nil {
		t.Fatalf("query id2: %v", err)
	}
	if p1 != 2000 || p2 != 1000 {
		t.Errorf("after move-up: id1.position=%d id2.position=%d; want 2000/1000", p1, p2)
	}
}

// TestReorderQueueTrack_Boundary verifies that moving the first track up or the
// last track down is a no-op (positions unchanged).
func TestReorderQueueTrack_Boundary(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)

	seedTrack(t, d, "qrb1", "/qb1")
	seedTrack(t, d, "qrb2", "/qb2")
	id1 := seedQueueEntry(t, d, "qrb1", 1000)
	id2 := seedQueueEntry(t, d, "qrb2", 2000)

	// Move first track up — no-op.
	if err := w.ReorderQueueTrack(id1, -1); err != nil {
		t.Fatalf("ReorderQueueTrack boundary up: %v", err)
	}
	// Move last track down — no-op.
	if err := w.ReorderQueueTrack(id2, +1); err != nil {
		t.Fatalf("ReorderQueueTrack boundary down: %v", err)
	}

	var p1, p2 int64
	if err := d.QueryRow(`SELECT position FROM queue WHERE id=?`, id1).Scan(&p1); err != nil {
		t.Fatalf("query id1: %v", err)
	}
	if err := d.QueryRow(`SELECT position FROM queue WHERE id=?`, id2).Scan(&p2); err != nil {
		t.Fatalf("query id2: %v", err)
	}
	if p1 != 1000 || p2 != 2000 {
		t.Errorf("positions changed after boundary ops: id1=%d id2=%d; want 1000/2000", p1, p2)
	}
}

// TestReorderQueueTrack_IntegerPositions verifies that position values remain
// integers (not floating point) after a swap.
func TestReorderQueueTrack_IntegerPositions(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)

	seedTrack(t, d, "qri1", "/qi1")
	seedTrack(t, d, "qri2", "/qi2")
	id1 := seedQueueEntry(t, d, "qri1", 1000)
	id2 := seedQueueEntry(t, d, "qri2", 2000)

	if err := w.ReorderQueueTrack(id1, +1); err != nil {
		t.Fatalf("ReorderQueueTrack: %v", err)
	}

	// Scan into int64 — if the column held a float the scan would succeed but
	// the round-trip value would be detectable via the SQL typeof() function.
	rows, err := d.Query(`SELECT id, position, typeof(position) FROM queue WHERE id IN (?,?) ORDER BY id`, id1, id2)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, pos int64
		var typ string
		if err := rows.Scan(&id, &pos, &typ); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if typ != "integer" {
			t.Errorf("queue id=%d: position typeof=%q; want integer", id, typ)
		}
	}
}

// TestUpsertBatch verifies that UpsertBatch persists tracks to the tracks table
// and that AppendTrack does not return a FK constraint error after the batch is
// committed. This covers the critical integration path where a missing tracks
// row would cause queue inserts, queue reads, and playlist track reads to fail.
func TestUpsertBatch(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)

	tracks := []library.Track{
		{Hash: "hash-a1", Path: "/music/a.mp3", Title: "Track A", Artist: "Artist A", Album: "Album A", Duration: 180.5},
		{Hash: "hash-b2", Path: "/music/b.flac", Title: "Track B", Artist: "Artist B", Album: "Album B", Duration: 240.0},
	}

	if err := w.UpsertBatch(tracks); err != nil {
		t.Fatalf("UpsertBatch: %v", err)
	}

	// Both tracks must now exist in the tracks table.
	var count int
	if err := d.QueryRow(`SELECT COUNT(*) FROM tracks WHERE hash IN ('hash-a1', 'hash-b2')`).Scan(&count); err != nil {
		t.Fatalf("count tracks: %v", err)
	}
	if count != 2 {
		t.Errorf("tracks count = %d; want 2", count)
	}

	// Verify stored fields are correct.
	var path, title, artist, album string
	var durationMs int64
	if err := d.QueryRow(
		`SELECT path, title, artist, album, duration_ms FROM tracks WHERE hash='hash-a1'`,
	).Scan(&path, &title, &artist, &album, &durationMs); err != nil {
		t.Fatalf("select track: %v", err)
	}
	if path != "/music/a.mp3" {
		t.Errorf("path = %q; want /music/a.mp3", path)
	}
	if title != "Track A" {
		t.Errorf("title = %q; want Track A", title)
	}
	if durationMs != 180500 {
		t.Errorf("duration_ms = %d; want 180500", durationMs)
	}

	// AppendTrack must not fail with a FK constraint error now that the track
	// exists in the tracks table. Before UpsertBatch was wired, this would
	// return "FOREIGN KEY constraint failed".
	if _, err := w.AppendTrack(tracks[0]); err != nil {
		t.Errorf("AppendTrack after UpsertBatch: %v", err)
	}
}

// TestUpsertBatch_Idempotent verifies that calling UpsertBatch twice with the
// same hash updates the row in place without cascading a delete on any FK
// dependants (queue rows, playlist_tracks rows).
func TestUpsertBatch_Idempotent(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)

	original := library.Track{Hash: "hash-idem", Path: "/old.mp3", Title: "Old Title", Artist: "Old Artist", Album: "Old Album", Duration: 60.0}
	if err := w.UpsertBatch([]library.Track{original}); err != nil {
		t.Fatalf("first UpsertBatch: %v", err)
	}

	// Enqueue the track so we can verify the FK row survives the second upsert.
	queueID, err := w.AppendTrack(original)
	if err != nil {
		t.Fatalf("AppendTrack: %v", err)
	}

	// Second upsert with updated metadata.
	updated := library.Track{Hash: "hash-idem", Path: "/new.mp3", Title: "New Title", Artist: "New Artist", Album: "New Album", Duration: 90.0}
	if err := w.UpsertBatch([]library.Track{updated}); err != nil {
		t.Fatalf("second UpsertBatch: %v", err)
	}

	// Path and title must reflect the updated values.
	var path, title string
	if err := d.QueryRow(`SELECT path, title FROM tracks WHERE hash='hash-idem'`).Scan(&path, &title); err != nil {
		t.Fatalf("select: %v", err)
	}
	if path != "/new.mp3" {
		t.Errorf("path = %q; want /new.mp3", path)
	}
	if title != "New Title" {
		t.Errorf("title = %q; want New Title", title)
	}

	// The queue row must still exist (INSERT OR REPLACE would have deleted it).
	var queueCount int
	if err := d.QueryRow(`SELECT COUNT(*) FROM queue WHERE id=?`, queueID).Scan(&queueCount); err != nil {
		t.Fatalf("count queue: %v", err)
	}
	if queueCount != 1 {
		t.Errorf("queue row count = %d after second upsert; want 1 (no cascade delete)", queueCount)
	}
}

// TestUpsertBatch_Empty verifies that UpsertBatch returns nil without error
// when given an empty slice.
func TestUpsertBatch_Empty(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)

	if err := w.UpsertBatch(nil); err != nil {
		t.Errorf("UpsertBatch(nil): %v", err)
	}
	if err := w.UpsertBatch([]library.Track{}); err != nil {
		t.Errorf("UpsertBatch([]): %v", err)
	}

	var count int
	if err := d.QueryRow(`SELECT COUNT(*) FROM tracks`).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Errorf("tracks count = %d; want 0", count)
	}
}
