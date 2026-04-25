package db_test

import (
	"database/sql"
	"slices"
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/db"
)

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
