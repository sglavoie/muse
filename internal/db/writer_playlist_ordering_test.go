package db_test

import (
	"strings"
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/db"
)

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

	if err := w.ReorderPlaylistTrack(plID, id1, -1); err != nil {
		t.Fatalf("ReorderPlaylistTrack boundary up: %v", err)
	}
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

// TestReorderPlaylistTrack_InvalidDirection verifies invalid direction values
// return an error and do not mutate ordering.
func TestReorderPlaylistTrack_InvalidDirection(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)

	seedTrack(t, d, "tri-dir-1", "/tid1")
	seedTrack(t, d, "tri-dir-2", "/tid2")
	plID := seedPlaylist(t, d, "Playlist Invalid Direction")
	id1 := seedPlaylistTrack(t, d, plID, "tri-dir-1", 1000)
	id2 := seedPlaylistTrack(t, d, plID, "tri-dir-2", 2000)

	err := w.ReorderPlaylistTrack(plID, id1, 0)
	if err == nil {
		t.Fatal("expected error for invalid direction, got nil")
	}
	if !strings.Contains(err.Error(), "direction must be -1 or +1") {
		t.Fatalf("unexpected error: %v", err)
	}

	var p1, p2 int64
	if err := d.QueryRow(`SELECT position FROM playlist_tracks WHERE id=?`, id1).Scan(&p1); err != nil {
		t.Fatalf("query id1: %v", err)
	}
	if err := d.QueryRow(`SELECT position FROM playlist_tracks WHERE id=?`, id2).Scan(&p2); err != nil {
		t.Fatalf("query id2: %v", err)
	}
	if p1 != 1000 || p2 != 2000 {
		t.Errorf("positions changed after invalid direction: id1=%d id2=%d; want 1000/2000", p1, p2)
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
	id2 := seedPlaylistTrack(t, d, plID, "trs2", 2000)
	id3 := seedPlaylistTrack(t, d, plID, "trs3", 3000)

	now := time.Now().Unix()
	if _, err := d.Exec(`UPDATE playlist_tracks SET deleted_at=? WHERE id=?`, now, id2); err != nil {
		t.Fatalf("soft-delete middle: %v", err)
	}

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

// TestReorderPlaylistTrack_SoftDeletedEntryIsNoOp verifies that attempting to
// reorder a soft-deleted playlist_tracks entry is a silent no-op. The UI may
// hold a stale ID after a concurrent soft-delete or after an undo window
// purge; surfacing sql.ErrNoRows would only generate noise. Positions of
// other rows must be unchanged.
func TestReorderPlaylistTrack_SoftDeletedEntryIsNoOp(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)

	seedTrack(t, d, "trsd1", "/sd1")
	seedTrack(t, d, "trsd2", "/sd2")
	plID := seedPlaylist(t, d, "Playlist SD")
	id1 := seedPlaylistTrack(t, d, plID, "trsd1", 1000)
	id2 := seedPlaylistTrack(t, d, plID, "trsd2", 2000)

	now := time.Now().Unix()
	if _, err := d.Exec(`UPDATE playlist_tracks SET deleted_at=? WHERE id=?`, now, id1); err != nil {
		t.Fatalf("soft-delete: %v", err)
	}

	if err := w.ReorderPlaylistTrack(plID, id1, +1); err != nil {
		t.Errorf("ReorderPlaylistTrack(soft-deleted) = %v, want nil (stale IDs must be no-ops)", err)
	}

	var p2 int64
	if err := d.QueryRow(`SELECT position FROM playlist_tracks WHERE id=?`, id2).Scan(&p2); err != nil {
		t.Fatalf("query id2: %v", err)
	}
	if p2 != 2000 {
		t.Errorf("id2.position=%d; want 2000 (should be unchanged)", p2)
	}
}

func TestSetPlaylistOrder_IgnoresIDsFromOtherPlaylists(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)

	seedTrack(t, d, "set-scope-a1", "/set-scope-a1")
	seedTrack(t, d, "set-scope-a2", "/set-scope-a2")
	seedTrack(t, d, "set-scope-b1", "/set-scope-b1")
	plA := seedPlaylist(t, d, "Playlist Scope A")
	plB := seedPlaylist(t, d, "Playlist Scope B")
	a1 := seedPlaylistTrack(t, d, plA, "set-scope-a1", 1000)
	a2 := seedPlaylistTrack(t, d, plA, "set-scope-a2", 2000)
	b1 := seedPlaylistTrack(t, d, plB, "set-scope-b1", 9000)

	if err := w.SetPlaylistOrder(plA, []int64{a2, a1, b1}); err != nil {
		t.Fatalf("SetPlaylistOrder: %v", err)
	}

	gotA, err := d.ReadPlaylistTracks(plA)
	if err != nil {
		t.Fatalf("ReadPlaylistTracks A: %v", err)
	}
	if len(gotA) != 2 {
		t.Fatalf("playlist A row count = %d, want 2", len(gotA))
	}
	if gotA[0].ID != a2 || gotA[0].Position != 1000 || gotA[1].ID != a1 || gotA[1].Position != 2000 {
		t.Errorf("playlist A order = [{id:%d pos:%d} {id:%d pos:%d}], want a2@1000 then a1@2000",
			gotA[0].ID, gotA[0].Position, gotA[1].ID, gotA[1].Position)
	}

	var bPos int64
	if err := d.QueryRow(`SELECT position FROM playlist_tracks WHERE id=?`, b1).Scan(&bPos); err != nil {
		t.Fatalf("query playlist B row: %v", err)
	}
	if bPos != 9000 {
		t.Errorf("foreign playlist row position = %d, want 9000 (SetPlaylistOrder must not update other playlists)", bPos)
	}
}
