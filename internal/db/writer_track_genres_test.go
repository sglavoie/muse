package db_test

import (
	"sort"
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/library"
)

func TestPopulateTrackGenres_Empty(t *testing.T) {
	d := openMigratedDB(t)
	w := newWriter(t, d)

	// No tracks: should be a no-op with no error.
	if err := w.PopulateTrackGenres(nil); err != nil {
		t.Fatalf("PopulateTrackGenres(nil): %v", err)
	}
	if err := w.PopulateTrackGenres([]library.Track{}); err != nil {
		t.Fatalf("PopulateTrackGenres([]): %v", err)
	}

	var count int
	if err := d.QueryRow(`SELECT COUNT(*) FROM track_genres`).Scan(&count); err != nil {
		t.Fatalf("count track_genres: %v", err)
	}
	if count != 0 {
		t.Errorf("track_genres rows = %d, want 0", count)
	}
}

func TestPopulateTrackGenres_EmptyGenre(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	if _, err := d.Exec(
		`INSERT INTO tracks (hash, path, created_at, updated_at) VALUES (?, ?, ?, ?)`,
		"h1", "/a.flac", now, now,
	); err != nil {
		t.Fatalf("insert track: %v", err)
	}
	w := newWriter(t, d)

	tracks := []library.Track{{Hash: "h1", Path: "/a.flac", Genre: ""}}
	if err := w.PopulateTrackGenres(tracks); err != nil {
		t.Fatalf("PopulateTrackGenres: %v", err)
	}

	var count int
	if err := d.QueryRow(`SELECT COUNT(*) FROM track_genres WHERE track_hash = ?`, "h1").Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Errorf("track_genres rows for empty genre = %d, want 0", count)
	}
}

func TestPopulateTrackGenres_SingleGenre(t *testing.T) {
	d := openMigratedDB(t)
	insertTrack(t, d, "h1", "/a.flac")
	w := newWriter(t, d)

	tracks := []library.Track{{Hash: "h1", Path: "/a.flac", Genre: "Rock"}}
	if err := w.PopulateTrackGenres(tracks); err != nil {
		t.Fatalf("PopulateTrackGenres: %v", err)
	}

	rows, err := d.Query(`SELECT genre FROM track_genres WHERE track_hash = ? ORDER BY genre`, "h1")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()
	var genres []string
	for rows.Next() {
		var g string
		if err := rows.Scan(&g); err != nil {
			t.Fatalf("scan: %v", err)
		}
		genres = append(genres, g)
	}
	if len(genres) != 1 || genres[0] != "Rock" {
		t.Errorf("genres = %v, want [Rock]", genres)
	}
}

func TestPopulateTrackGenres_MultiGenre(t *testing.T) {
	d := openMigratedDB(t)
	insertTrack(t, d, "h1", "/a.flac")
	w := newWriter(t, d)

	tracks := []library.Track{{Hash: "h1", Path: "/a.flac", Genre: "Rock; Jazz; Blues"}}
	if err := w.PopulateTrackGenres(tracks); err != nil {
		t.Fatalf("PopulateTrackGenres: %v", err)
	}

	rows, err := d.Query(`SELECT genre FROM track_genres WHERE track_hash = ? ORDER BY genre`, "h1")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()
	var genres []string
	for rows.Next() {
		var g string
		if err := rows.Scan(&g); err != nil {
			t.Fatalf("scan: %v", err)
		}
		genres = append(genres, g)
	}
	sort.Strings(genres)
	want := []string{"Blues", "Jazz", "Rock"}
	if len(genres) != len(want) {
		t.Fatalf("genres = %v, want %v", genres, want)
	}
	for i := range want {
		if genres[i] != want[i] {
			t.Errorf("genres[%d] = %q, want %q", i, genres[i], want[i])
		}
	}
}

func TestPopulateTrackGenres_Dedup(t *testing.T) {
	d := openMigratedDB(t)
	insertTrack(t, d, "h1", "/a.flac")
	w := newWriter(t, d)

	// Call twice with the same genre — should not duplicate rows (INSERT OR IGNORE).
	tracks := []library.Track{{Hash: "h1", Path: "/a.flac", Genre: "Rock"}}
	if err := w.PopulateTrackGenres(tracks); err != nil {
		t.Fatalf("first PopulateTrackGenres: %v", err)
	}
	if err := w.PopulateTrackGenres(tracks); err != nil {
		t.Fatalf("second PopulateTrackGenres: %v", err)
	}

	var count int
	if err := d.QueryRow(`SELECT COUNT(*) FROM track_genres WHERE track_hash = ?`, "h1").Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("track_genres rows after dedup = %d, want 1", count)
	}
}

func TestPopulateTrackGenres_MultipleTracksDelimiters(t *testing.T) {
	d := openMigratedDB(t)
	insertTrack(t, d, "h1", "/a.flac")
	insertTrack(t, d, "h2", "/b.flac")
	w := newWriter(t, d)

	tracks := []library.Track{
		{Hash: "h1", Genre: "Rock/Metal"},     // "/" delimiter
		{Hash: "h2", Genre: "Pop,Electronic"}, // "," delimiter
	}
	if err := w.PopulateTrackGenres(tracks); err != nil {
		t.Fatalf("PopulateTrackGenres: %v", err)
	}

	var count1, count2 int
	if err := d.QueryRow(`SELECT COUNT(*) FROM track_genres WHERE track_hash = ?`, "h1").Scan(&count1); err != nil {
		t.Fatalf("count h1: %v", err)
	}
	if err := d.QueryRow(`SELECT COUNT(*) FROM track_genres WHERE track_hash = ?`, "h2").Scan(&count2); err != nil {
		t.Fatalf("count h2: %v", err)
	}
	if count1 != 2 {
		t.Errorf("h1 track_genres count = %d, want 2 (Rock, Metal)", count1)
	}
	if count2 != 2 {
		t.Errorf("h2 track_genres count = %d, want 2 (Pop, Electronic)", count2)
	}
}
