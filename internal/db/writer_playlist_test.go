package db_test

import (
	"database/sql"
	"slices"
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/db"
)

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
// positions (1000, 2000, ...) and that a second call continues the sequence.
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
// reorder a soft-deleted playlist_tracks entry returns sql.ErrNoRows.
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

	err := w.ReorderPlaylistTrack(plID, id1, +1)
	if err == nil {
		t.Fatal("expected error when reordering soft-deleted entry, got nil")
	}

	var p2 int64
	if err := d.QueryRow(`SELECT position FROM playlist_tracks WHERE id=?`, id2).Scan(&p2); err != nil {
		t.Fatalf("query id2: %v", err)
	}
	if p2 != 2000 {
		t.Errorf("id2.position=%d; want 2000 (should be unchanged)", p2)
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

	expiredID := seedPlaylistTrack(t, d, plID, "tp1", 1000)
	if _, err := d.Exec(`UPDATE playlist_tracks SET deleted_at=? WHERE id=?`, base-10, expiredID); err != nil {
		t.Fatalf("set expired deleted_at: %v", err)
	}

	recentID := seedPlaylistTrack(t, d, plID, "tp1", 2000)
	if _, err := d.Exec(`UPDATE playlist_tracks SET deleted_at=? WHERE id=?`, base+10, recentID); err != nil {
		t.Fatalf("set recent deleted_at: %v", err)
	}

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

// TestPurgeDeletedPlaylists verifies that only playlists deleted before the
// supplied cutoff are permanently removed.
func TestPurgeDeletedPlaylists(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)

	id1, _ := w.CreatePlaylist("keep")
	id2, _ := w.CreatePlaylist("purge-old")
	id3, _ := w.CreatePlaylist("purge-recent")
	_ = id1

	if err := w.DeletePlaylistSoft(id2); err != nil {
		t.Fatalf("DeletePlaylistSoft: %v", err)
	}
	if err := w.DeletePlaylistSoft(id3); err != nil {
		t.Fatalf("DeletePlaylistSoft: %v", err)
	}

	if _, err := d.Exec(`UPDATE playlists SET deleted_at=100 WHERE id=?`, id2); err != nil {
		t.Fatalf("backdate: %v", err)
	}

	if err := w.PurgeDeletedPlaylists(200); err != nil {
		t.Fatalf("PurgeDeletedPlaylists: %v", err)
	}

	var count int
	if err := d.QueryRow(`SELECT COUNT(*) FROM playlists`).Scan(&count); err != nil {
		t.Fatalf("select: %v", err)
	}
	if count != 2 {
		t.Errorf("playlist count after purge = %d, want 2", count)
	}

	var exists int
	if err := d.QueryRow(`SELECT COUNT(*) FROM playlists WHERE id=?`, id2).Scan(&exists); err != nil {
		t.Fatalf("select id2: %v", err)
	}
	if exists != 0 {
		t.Error("id2 should have been purged")
	}
}
