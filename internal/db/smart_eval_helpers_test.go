package db_test

import (
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
)

// insertFullTrack inserts a track with all metadata fields. durationMs is in
// milliseconds, sizeBytes in bytes. createdAt is a Unix timestamp.
func insertFullTrack(t *testing.T, d *db.DB, hash, path, title, artist, album, genre string, durationMs, sizeBytes, createdAt int64) {
	t.Helper()
	now := time.Now().Unix()
	_, err := d.Exec(`
		INSERT INTO tracks (hash, path, title, artist, album, genre, duration_ms, size_bytes, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		hash, path, title, artist, album, genre, durationMs, sizeBytes, createdAt, now,
	)
	if err != nil {
		t.Fatalf("insertFullTrack: %v", err)
	}
}

// syncTrackGenres fills track_genres from every track's genre column, as the
// scan bridge does after each batch, for tests that insert tracks directly.
func syncTrackGenres(t *testing.T, d *db.DB) {
	t.Helper()
	rows, err := d.Query(`SELECT hash, genre FROM tracks`)
	if err != nil {
		t.Fatalf("syncTrackGenres: %v", err)
	}
	type pair struct{ hash, genre string }
	var pairs []pair
	for rows.Next() {
		var hash, genre string
		if err := rows.Scan(&hash, &genre); err != nil {
			t.Fatalf("syncTrackGenres scan: %v", err)
		}
		for _, g := range library.SplitGenres(genre) {
			pairs = append(pairs, pair{hash, g})
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("syncTrackGenres rows: %v", err)
	}
	if err := rows.Close(); err != nil {
		t.Fatalf("syncTrackGenres close: %v", err)
	}
	for _, p := range pairs {
		if _, err := d.Exec(`INSERT OR IGNORE INTO track_genres (track_hash, genre) VALUES (?, ?)`, p.hash, p.genre); err != nil {
			t.Fatalf("syncTrackGenres insert: %v", err)
		}
	}
}

func insertFavoriteTrack(t *testing.T, d *db.DB, hash string) {
	t.Helper()
	now := time.Now().Unix()
	_, err := d.Exec(`INSERT INTO favorites (type, value, created_at) VALUES ('track', ?, ?)`, hash, now)
	if err != nil {
		t.Fatalf("insertFavoriteTrack: %v", err)
	}
}

func setPlayCount(t *testing.T, d *db.DB, hash string, count int) {
	t.Helper()
	_, err := d.Exec(`UPDATE tracks SET play_count = ? WHERE hash = ?`, count, hash)
	if err != nil {
		t.Fatalf("setPlayCount: %v", err)
	}
}

func evalQuery(t *testing.T, d *db.DB, q db.SmartPlaylistQuery) []db.PlaylistTrackRow {
	t.Helper()
	rows, err := d.EvalSmartPlaylist(q)
	if err != nil {
		t.Fatalf("EvalSmartPlaylist: %v", err)
	}
	return rows
}

func hashes(rows []db.PlaylistTrackRow) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.Hash
	}
	return out
}

func newWriter(t *testing.T, d *db.DB) *db.Writer {
	t.Helper()
	w := db.NewWriter(t.Context(), d.DB)
	t.Cleanup(w.Wait)
	return w
}

func insertTag(t *testing.T, d *db.DB, w *db.Writer, name string) int64 {
	t.Helper()
	id, err := w.CreateTag(name, "")
	if err != nil {
		t.Fatalf("CreateTag(%q): %v", name, err)
	}
	return id
}

func tagTrack(t *testing.T, w *db.Writer, hash string, tagID int64) {
	t.Helper()
	if err := w.AddTrackTag(hash, tagID); err != nil {
		t.Fatalf("AddTrackTag(%q, %d): %v", hash, tagID, err)
	}
}
