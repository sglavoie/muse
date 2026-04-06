package db_test

import (
	"slices"
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/db"
)

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
// rewrites all active positions as 1000, 2000, 3000... when an adjacent gap is
// below 1. Soft-deleted rows are excluded from renumbering.
func TestRenumberQueuePositions_Triggered(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)

	seedTrack(t, d, "hs4", "/f4")
	now := time.Now().Unix()

	if _, err := d.Exec(`DROP INDEX IF EXISTS queue_pos_idx`); err != nil {
		t.Fatalf("drop index: %v", err)
	}

	for _, pos := range []int{1000, 1000, 2000} {
		if _, err := d.Exec(
			`INSERT INTO queue (track_hash, position, added_at) VALUES ('hs4', ?, ?)`, pos, now,
		); err != nil {
			t.Fatalf("insert queue pos %d: %v", pos, err)
		}
	}
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
	if want := []int64{9000}; !slices.Equal(deletedPos, want) {
		t.Errorf("soft-deleted positions = %v; want %v", deletedPos, want)
	}
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

	if err := w.ReorderQueueTrack(id1, -1); err != nil {
		t.Fatalf("ReorderQueueTrack boundary up: %v", err)
	}
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

// TestRenumberQueuePositions_SoftDeletedConflict verifies that
// RenumberQueuePositions succeeds when soft-deleted rows occupy positions in
// the range that the renumber would assign to active rows.
func TestRenumberQueuePositions_SoftDeletedConflict(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)

	seedTrack(t, d, "conflict1", "/c1")
	now := time.Now().Unix()

	if _, err := d.Exec(`DROP INDEX IF EXISTS queue_pos_idx`); err != nil {
		t.Fatalf("drop index: %v", err)
	}

	for _, pos := range []int{500, 500} {
		if _, err := d.Exec(
			`INSERT INTO queue (track_hash, position, added_at) VALUES ('conflict1', ?, ?)`,
			pos, now,
		); err != nil {
			t.Fatalf("insert active row: %v", err)
		}
	}

	if _, err := d.Exec(
		`INSERT INTO queue (track_hash, position, added_at, deleted_at) VALUES ('conflict1', 1000, ?, ?)`,
		now, now,
	); err != nil {
		t.Fatalf("insert conflicting soft-deleted row: %v", err)
	}

	if _, err := d.Exec(
		`INSERT INTO queue (track_hash, position, added_at, deleted_at) VALUES ('conflict1', 2000, ?, ?)`,
		now, now,
	); err != nil {
		t.Fatalf("insert conflicting soft-deleted row 2: %v", err)
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
	for rows.Next() {
		var pos int64
		var active int
		if err := rows.Scan(&pos, &active); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if active == 1 {
			activePos = append(activePos, pos)
		}
	}

	if want := []int64{1000, 2000}; !slices.Equal(activePos, want) {
		t.Errorf("active positions = %v; want %v", activePos, want)
	}
}

// TestRenumberQueuePositions_SoftDeletedConflictAboveRange verifies that
// RenumberQueuePositions succeeds even when soft-deleted rows exist both in
// the renumber range AND above it, which would cause a unique constraint
// violation if relocated rows were placed at maxNewPos+originalPos instead
// of above the global maximum.
func TestRenumberQueuePositions_SoftDeletedConflictAboveRange(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)

	seedTrack(t, d, "conflict2", "/c2")
	now := time.Now().Unix()

	if _, err := d.Exec(`DROP INDEX IF EXISTS queue_pos_idx`); err != nil {
		t.Fatalf("drop index: %v", err)
	}

	// Two active rows with identical positions -> triggers renumber.
	for _, pos := range []int{500, 500} {
		if _, err := d.Exec(
			`INSERT INTO queue (track_hash, position, added_at) VALUES ('conflict2', ?, ?)`,
			pos, now,
		); err != nil {
			t.Fatalf("insert active row: %v", err)
		}
	}

	// Soft-deleted rows at positions 1000, 2000 (in the renumber range)
	// AND at 3000, 4000 (above maxNewPos=2000, which would collide with
	// the old safePos=maxNewPos+r.position calculation).
	for _, pos := range []int{1000, 2000, 3000, 4000} {
		if _, err := d.Exec(
			`INSERT INTO queue (track_hash, position, added_at, deleted_at) VALUES ('conflict2', ?, ?, ?)`,
			pos, now, now,
		); err != nil {
			t.Fatalf("insert soft-deleted row pos=%d: %v", pos, err)
		}
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
	for rows.Next() {
		var pos int64
		var active int
		if err := rows.Scan(&pos, &active); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if active == 1 {
			activePos = append(activePos, pos)
		}
	}

	if want := []int64{1000, 2000}; !slices.Equal(activePos, want) {
		t.Errorf("active positions = %v; want %v", activePos, want)
	}
}
