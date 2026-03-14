package db_test

import (
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/db"
)

// openMigratedDB opens a fresh DB and applies all migrations.
func openMigratedDB(t *testing.T) *db.DB {
	t.Helper()
	d := openTestDB(t)
	if err := db.Migrate(d.DB); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return d
}

// insertTrack inserts a minimal tracks row and returns the hash.
func insertTrack(t *testing.T, d *db.DB, hash, path string) {
	t.Helper()
	now := time.Now().Unix()
	_, err := d.Exec(
		`INSERT INTO tracks (hash, path, created_at, updated_at) VALUES (?, ?, ?, ?)`,
		hash, path, now, now,
	)
	if err != nil {
		t.Fatalf("insertTrack: %v", err)
	}
}

// insertQueueEntry inserts a queue row referencing hash and returns its id.
func insertQueueEntry(t *testing.T, d *db.DB, hash string, position int) int64 {
	t.Helper()
	now := time.Now().Unix()
	res, err := d.Exec(
		`INSERT INTO queue (track_hash, position, added_at) VALUES (?, ?, ?)`,
		hash, position, now,
	)
	if err != nil {
		t.Fatalf("insertQueueEntry: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

// insertPlaylist inserts a minimal playlists row and returns its id.
func insertPlaylist(t *testing.T, d *db.DB, name string) int64 {
	t.Helper()
	now := time.Now().Unix()
	res, err := d.Exec(
		`INSERT INTO playlists (name, created_at, updated_at) VALUES (?, ?, ?)`,
		name, now, now,
	)
	if err != nil {
		t.Fatalf("insertPlaylist: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

// insertPlaylistTrack inserts a playlist_tracks row for testing.
func insertPlaylistTrack(t *testing.T, d *db.DB, playlistID int64, hash string, position int) {
	t.Helper()
	now := time.Now().Unix()
	_, err := d.Exec(
		`INSERT INTO playlist_tracks (playlist_id, track_hash, position, added_at) VALUES (?, ?, ?, ?)`,
		playlistID, hash, position, now,
	)
	if err != nil {
		t.Fatalf("insertPlaylistTrack: %v", err)
	}
}

func TestWriter_MarkTrackMissing(t *testing.T) {
	d := openMigratedDB(t)
	insertTrack(t, d, "hash1", "/music/a.flac")

	w := db.NewWriter(t.Context(), d.DB)

	if err := w.MarkTrackMissing("hash1"); err != nil {
		t.Fatalf("MarkTrackMissing: %v", err)
	}

	var missing int
	if err := d.QueryRow(`SELECT missing FROM tracks WHERE hash='hash1'`).Scan(&missing); err != nil {
		t.Fatalf("query: %v", err)
	}
	if missing != 1 {
		t.Errorf("tracks.missing = %d, want 1", missing)
	}
}

func TestWriter_ClearTrackMissing(t *testing.T) {
	d := openMigratedDB(t)
	insertTrack(t, d, "hash2", "/music/b.flac")

	w := db.NewWriter(t.Context(), d.DB)

	// First mark, then clear.
	if err := w.MarkTrackMissing("hash2"); err != nil {
		t.Fatalf("MarkTrackMissing: %v", err)
	}
	if err := w.ClearTrackMissing("hash2"); err != nil {
		t.Fatalf("ClearTrackMissing: %v", err)
	}

	var missing int
	if err := d.QueryRow(`SELECT missing FROM tracks WHERE hash='hash2'`).Scan(&missing); err != nil {
		t.Fatalf("query: %v", err)
	}
	if missing != 0 {
		t.Errorf("tracks.missing = %d after clear, want 0", missing)
	}
}

func TestWriter_MarkQueueEntryMissing(t *testing.T) {
	d := openMigratedDB(t)
	insertTrack(t, d, "hash3", "/music/c.flac")
	qid := insertQueueEntry(t, d, "hash3", 1000)

	w := db.NewWriter(t.Context(), d.DB)

	if err := w.MarkQueueEntryMissing(qid); err != nil {
		t.Fatalf("MarkQueueEntryMissing: %v", err)
	}

	var missing int
	if err := d.QueryRow(`SELECT missing FROM queue WHERE id=?`, qid).Scan(&missing); err != nil {
		t.Fatalf("query: %v", err)
	}
	if missing != 1 {
		t.Errorf("queue.missing = %d, want 1", missing)
	}
}

func TestWriter_ClearQueueEntriesMissing(t *testing.T) {
	d := openMigratedDB(t)
	insertTrack(t, d, "hash4", "/music/d.flac")
	qid1 := insertQueueEntry(t, d, "hash4", 1000)
	qid2 := insertQueueEntry(t, d, "hash4", 2000)

	w := db.NewWriter(t.Context(), d.DB)

	// Mark both entries then clear via hash.
	if err := w.MarkQueueEntryMissing(qid1); err != nil {
		t.Fatalf("MarkQueueEntryMissing qid1: %v", err)
	}
	if err := w.MarkQueueEntryMissing(qid2); err != nil {
		t.Fatalf("MarkQueueEntryMissing qid2: %v", err)
	}
	if err := w.ClearQueueEntriesMissing("hash4"); err != nil {
		t.Fatalf("ClearQueueEntriesMissing: %v", err)
	}

	for _, qid := range []int64{qid1, qid2} {
		var missing int
		if err := d.QueryRow(`SELECT missing FROM queue WHERE id=?`, qid).Scan(&missing); err != nil {
			t.Fatalf("query qid=%d: %v", qid, err)
		}
		if missing != 0 {
			t.Errorf("queue.missing = %d for id=%d after clear, want 0", missing, qid)
		}
	}
}
