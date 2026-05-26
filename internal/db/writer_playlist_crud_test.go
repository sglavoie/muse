package db_test

import (
	"database/sql"
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

// TestRenamePlaylist_SoftDeletedNoOp verifies that RenamePlaylist does not
// mutate soft-deleted playlists.
func TestRenamePlaylist_SoftDeletedNoOp(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)

	id := seedPlaylist(t, d, "Original Name")
	if err := w.DeletePlaylistSoft(id); err != nil {
		t.Fatalf("DeletePlaylistSoft: %v", err)
	}

	if err := w.RenamePlaylist(id, "Renamed While Deleted"); err != nil {
		t.Fatalf("RenamePlaylist on soft-deleted playlist: %v", err)
	}

	var name string
	if err := d.QueryRow(`SELECT name FROM playlists WHERE id=?`, id).Scan(&name); err != nil {
		t.Fatalf("query: %v", err)
	}
	if name != "Original Name" {
		t.Errorf("name = %q; want %q", name, "Original Name")
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
