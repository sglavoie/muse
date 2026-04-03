package db_test

import (
	"database/sql"
	"slices"
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/db"
)

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
