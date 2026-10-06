package db_test

import (
	"database/sql"
	"testing"

	"github.com/sglavoie/muse/internal/db"
)

func TestPlaylistBulkRestoreRollsBackAndCanRetry(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)
	seedTrack(t, d, "h", "/music/h.flac")
	pl := seedPlaylist(t, d, "Restore")
	ids := []int64{seedPlaylistTrack(t, d, pl, "h", 1000), seedPlaylistTrack(t, d, pl, "h", 2000)}
	for _, id := range ids {
		if err := w.RemovePlaylistTrackSoft(id); err != nil {
			t.Fatal(err)
		}
	}
	// Fail on the second row, after the first UPDATE has succeeded.
	if _, err := d.Exec(`CREATE TRIGGER fail_restore BEFORE UPDATE OF deleted_at ON playlist_tracks
		WHEN NEW.position=2000 AND NEW.deleted_at IS NULL
		BEGIN SELECT RAISE(ABORT, 'restore refused'); END`); err != nil {
		t.Fatal(err)
	}
	if err := w.UndoRemovePlaylistTracks(ids); err == nil {
		t.Fatal("expected restore failure")
	}
	rows, err := d.ReadPlaylistTracks(pl)
	if err != nil || len(rows) != 0 {
		t.Fatalf("partial restore: %v, %v", rows, err)
	}
	if _, err := d.Exec(`DROP TRIGGER fail_restore`); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := w.UndoRemovePlaylistTracks(ids); err != nil {
			t.Fatal(err)
		}
	}
	rows, err = d.ReadPlaylistTracks(pl)
	if err != nil || len(rows) != 2 || rows[0].ID != ids[0] || rows[1].ID != ids[1] {
		t.Fatalf("retry lost row identity/order: %v, %v", rows, err)
	}
}

func TestTagRestoreRollsBackTagAndAssignments(t *testing.T) {
	d, w := openTagDB(t)
	seedTrack(t, d, "one", "/music/one.flac")
	seedTrack(t, d, "two", "/music/two.flac")
	id, err := w.CreateTag("Restore", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := w.DeleteTagSoft(id); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(`CREATE TRIGGER fail_tag_restore BEFORE INSERT ON track_tags
		WHEN NEW.track_hash='two'
		BEGIN SELECT RAISE(ABORT, 'assignment refused'); END`); err != nil {
		t.Fatal(err)
	}
	if err := w.RestoreTrackTagBulk([]string{"one", "two"}, id); err == nil {
		t.Fatal("expected restore failure")
	}
	var deleted sql.NullInt64
	if err := d.QueryRow(`SELECT deleted_at FROM tags WHERE id=?`, id).Scan(&deleted); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := d.QueryRow(`SELECT COUNT(*) FROM track_tags WHERE tag_id=?`, id).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if !deleted.Valid || count != 0 {
		t.Fatalf("partial restore: deleted=%v assignments=%d", deleted, count)
	}
	if _, err := d.Exec(`DROP TRIGGER fail_tag_restore`); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := w.RestoreTrackTagBulk([]string{"one", "two"}, id); err != nil {
			t.Fatal(err)
		}
	}
	for _, hash := range []string{"one", "two"} {
		tags, err := d.ReadTagsForTrack(hash)
		if err != nil || len(tags) != 1 || tags[0].ID != id {
			t.Fatalf("retry: %v, %v", tags, err)
		}
	}
}
