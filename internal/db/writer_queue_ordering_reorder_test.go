package db_test

import (
	"strings"
	"testing"

	"github.com/sglavoie/muse/internal/db"
)

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

// TestReorderQueueTrack_InvalidDirection verifies invalid direction values
// return an error and do not mutate ordering.
func TestReorderQueueTrack_InvalidDirection(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)

	seedTrack(t, d, "qri-dir-1", "/qid1")
	seedTrack(t, d, "qri-dir-2", "/qid2")
	id1 := seedQueueEntry(t, d, "qri-dir-1", 1000)
	id2 := seedQueueEntry(t, d, "qri-dir-2", 2000)

	err := w.ReorderQueueTrack(id1, 0)
	if err == nil {
		t.Fatal("expected error for invalid direction, got nil")
	}
	if !strings.Contains(err.Error(), "direction must be -1 or +1") {
		t.Fatalf("unexpected error: %v", err)
	}

	var p1, p2 int64
	if err := d.QueryRow(`SELECT position FROM queue WHERE id=?`, id1).Scan(&p1); err != nil {
		t.Fatalf("query id1: %v", err)
	}
	if err := d.QueryRow(`SELECT position FROM queue WHERE id=?`, id2).Scan(&p2); err != nil {
		t.Fatalf("query id2: %v", err)
	}
	if p1 != 1000 || p2 != 2000 {
		t.Errorf("positions changed after invalid direction: id1=%d id2=%d; want 1000/2000", p1, p2)
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
