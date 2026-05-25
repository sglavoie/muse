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
