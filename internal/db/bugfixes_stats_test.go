package db_test

import (
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/db"
)

// TestReadListeningStats_TopGenres_SplitsMultiGenreTracks verifies that
// ReadListeningStats.TopGenres counts each individual genre on multi-genre
// tracks (via the track_genres junction) rather than the raw comma-separated
// tracks.genre string. Before the fix a track tagged "rock,pop" was grouped
// into a single composite "rock,pop" bucket, hiding both "rock" and "pop"
// from the user-facing top-N output.
func TestReadListeningStats_TopGenres_SplitsMultiGenreTracks(t *testing.T) {
	d := openMigratedDB(t)

	now := time.Now().Unix()
	if _, err := d.Exec(
		`INSERT INTO tracks (hash, path, genre, created_at, updated_at) VALUES ('a', '/a.flac', 'rock,pop', ?, ?)`,
		now, now,
	); err != nil {
		t.Fatalf("insert track a: %v", err)
	}
	if _, err := d.Exec(
		`INSERT INTO tracks (hash, path, genre, created_at, updated_at) VALUES ('b', '/b.flac', 'rock', ?, ?)`,
		now, now,
	); err != nil {
		t.Fatalf("insert track b: %v", err)
	}
	for _, row := range []struct{ hash, genre string }{
		{"a", "rock"},
		{"a", "pop"},
		{"b", "rock"},
	} {
		if _, err := d.Exec(
			`INSERT INTO track_genres (track_hash, genre) VALUES (?, ?)`,
			row.hash, row.genre,
		); err != nil {
			t.Fatalf("insert track_genres: %v", err)
		}
	}
	for _, hash := range []string{"a", "b"} {
		if _, err := d.Exec(
			`INSERT INTO play_history (track_hash, played_at, listened_secs) VALUES (?, ?, ?)`,
			hash, now, 30,
		); err != nil {
			t.Fatalf("insert play_history: %v", err)
		}
	}

	stats, err := d.ReadListeningStats(0, db.DefaultMixCountConfig())
	if err != nil {
		t.Fatalf("ReadListeningStats: %v", err)
	}

	seen := map[string]int{}
	for _, e := range stats.TopGenres {
		seen[e.Name] = e.PlayCount
	}
	if seen["rock"] != 2 {
		t.Errorf("top genres rock = %d, want 2 (multi-genre tracks must be counted per individual genre)", seen["rock"])
	}
	if seen["pop"] != 1 {
		t.Errorf("top genres pop = %d, want 1", seen["pop"])
	}
	if _, ok := seen["rock,pop"]; ok {
		t.Errorf("top genres contains composite key %q; expected per-genre keys only", "rock,pop")
	}
}
