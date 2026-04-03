package db_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/db"
)

func openTestDB(t *testing.T) *db.DB {
	t.Helper()
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func openMigratedTestDB(t *testing.T) *db.DB {
	t.Helper()
	d := openTestDB(t)
	if err := db.Migrate(d.DB); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return d
}

func seedTrack(t *testing.T, d *db.DB, hash, path string) {
	t.Helper()
	now := time.Now().Unix()
	if _, err := d.Exec(
		`INSERT INTO tracks (hash, path, created_at, updated_at) VALUES (?, ?, ?, ?)`,
		hash, path, now, now,
	); err != nil {
		t.Fatalf("seedTrack: %v", err)
	}
}

func seedPlaylist(t *testing.T, d *db.DB, name string) int64 {
	t.Helper()
	now := time.Now().Unix()
	res, err := d.Exec(
		`INSERT INTO playlists (name, created_at, updated_at) VALUES (?, ?, ?)`,
		name, now, now,
	)
	if err != nil {
		t.Fatalf("seedPlaylist %q: %v", name, err)
	}
	id, _ := res.LastInsertId()
	return id
}

func seedPlaylistTrack(t *testing.T, d *db.DB, playlistID int64, hash string, position int64) int64 {
	t.Helper()
	res, err := d.Exec(
		`INSERT INTO playlist_tracks (playlist_id, track_hash, position, added_at) VALUES (?, ?, ?, ?)`,
		playlistID, hash, position, time.Now().Unix(),
	)
	if err != nil {
		t.Fatalf("seedPlaylistTrack: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

func seedQueueEntry(t *testing.T, d *db.DB, hash string, pos int64) int64 {
	t.Helper()
	now := time.Now().Unix()
	res, err := d.Exec(
		`INSERT INTO queue (track_hash, position, added_at) VALUES (?, ?, ?)`,
		hash, pos, now,
	)
	if err != nil {
		t.Fatalf("seedQueueEntry hash=%s pos=%d: %v", hash, pos, err)
	}
	id, _ := res.LastInsertId()
	return id
}
