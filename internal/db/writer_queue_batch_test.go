package db_test

import (
	"slices"
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
)

func TestAppendTracks_BatchInsert(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)

	tracks := []library.Track{
		{Hash: "h1", Path: "/a.mp3"},
		{Hash: "h2", Path: "/b.mp3"},
		{Hash: "h3", Path: "/c.mp3"},
	}
	for _, tr := range tracks {
		seedTrack(t, d, tr.Hash, tr.Path)
	}

	ids, err := w.AppendTracks(tracks, "mylist")
	if err != nil {
		t.Fatalf("AppendTracks: %v", err)
	}
	if len(ids) != 3 {
		t.Fatalf("AppendTracks returned %d IDs, want 3", len(ids))
	}
	for i := 1; i < len(ids); i++ {
		if ids[i] <= ids[i-1] {
			t.Errorf("IDs not ascending: %v", ids)
			break
		}
	}

	rows, err := d.Query(`SELECT position, source_playlist FROM queue WHERE deleted_at IS NULL ORDER BY position`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()
	var positions []int64
	for rows.Next() {
		var pos int64
		var src string
		if err := rows.Scan(&pos, &src); err != nil {
			t.Fatalf("scan: %v", err)
		}
		positions = append(positions, pos)
		if src != "mylist" {
			t.Errorf("source_playlist = %q, want 'mylist'", src)
		}
	}
	if want := []int64{1000, 2000, 3000}; !slices.Equal(positions, want) {
		t.Errorf("positions = %v; want %v", positions, want)
	}
}

func TestAppendTracks_EmptySlice(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)

	ids, err := w.AppendTracks(nil, "")
	if err != nil {
		t.Fatalf("AppendTracks(nil): %v", err)
	}
	if ids == nil {
		t.Errorf("AppendTracks(nil): expected empty slice, got nil")
	}
	if len(ids) != 0 {
		t.Errorf("AppendTracks(nil): expected 0 ids, got %d", len(ids))
	}
}

// TestAppendTracks_NilIdsOnError verifies that AppendTracks returns nil IDs
// when the transaction fails mid-way (e.g. FK constraint violation). Before the
// fix, partial IDs from the rolled-back transaction were returned to the caller.
func TestAppendTracks_NilIdsOnError(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)

	// Seed only the first track. The second track has no matching tracks row,
	// so the FK constraint on queue.track_hash will reject the insert.
	seedTrack(t, d, "ok1", "/ok1.mp3")

	tracks := []library.Track{
		{Hash: "ok1", Path: "/ok1.mp3"},
		{Hash: "missing_fk", Path: "/missing.mp3"}, // no tracks row
	}
	ids, err := w.AppendTracks(tracks, "")
	if err == nil {
		t.Fatal("expected error from FK constraint violation, got nil")
	}
	if ids != nil {
		t.Errorf("expected nil ids on error, got %v", ids)
	}
}

func TestDequeueTracksSoft_BatchSoftDelete(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)

	for _, h := range []string{"d1", "d2", "d3"} {
		seedTrack(t, d, h, "/"+h+".mp3")
	}
	tracks := []library.Track{
		{Hash: "d1", Path: "/d1.mp3"},
		{Hash: "d2", Path: "/d2.mp3"},
		{Hash: "d3", Path: "/d3.mp3"},
	}
	ids, err := w.AppendTracks(tracks, "")
	if err != nil {
		t.Fatalf("AppendTracks: %v", err)
	}

	if err := w.DequeueTracksSoft(ids[:2]); err != nil {
		t.Fatalf("DequeueTracksSoft: %v", err)
	}

	var activeCount int
	if err := d.QueryRow(`SELECT COUNT(*) FROM queue WHERE deleted_at IS NULL`).Scan(&activeCount); err != nil {
		t.Fatalf("select: %v", err)
	}
	if activeCount != 1 {
		t.Errorf("active queue entries = %d, want 1", activeCount)
	}
}

func TestDequeueTracksSoft_EmptySlice(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)
	_ = d

	if err := w.DequeueTracksSoft(nil); err != nil {
		t.Fatalf("DequeueTracksSoft(nil): %v", err)
	}
}

// TestReorderQueueTrack_SoftDeletedEntryIsNoOp verifies that attempting to
// reorder a soft-deleted queue entry returns an error.
func TestReorderQueueTrack_SoftDeletedEntryIsNoOp(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)

	seedTrack(t, d, "qrsd1", "/qsd1")
	seedTrack(t, d, "qrsd2", "/qsd2")
	id1 := seedQueueEntry(t, d, "qrsd1", 1000)
	id2 := seedQueueEntry(t, d, "qrsd2", 2000)

	now := time.Now().Unix()
	if _, err := d.Exec(`UPDATE queue SET deleted_at=? WHERE id=?`, now, id1); err != nil {
		t.Fatalf("soft-delete: %v", err)
	}

	err := w.ReorderQueueTrack(id1, +1)
	if err == nil {
		t.Fatal("expected error when reordering soft-deleted queue entry, got nil")
	}

	var p2 int64
	if err := d.QueryRow(`SELECT position FROM queue WHERE id=?`, id2).Scan(&p2); err != nil {
		t.Fatalf("query id2: %v", err)
	}
	if p2 != 2000 {
		t.Errorf("id2.position=%d; want 2000 (should be unchanged)", p2)
	}
}
