package db_test

import (
	"strconv"
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/db"
)

func TestEvalSmartPlaylist_LastPlayedInLast(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()

	insertFullTrack(t, d, "h1", "/a.flac", "Song1", "Artist", "Album", "rock", 180000, 5000000, now)
	insertFullTrack(t, d, "h2", "/b.flac", "Song2", "Artist", "Album", "rock", 180000, 5000000, now)

	setTrackLastPlayedAt(t, d, "h1", now-3600)
	setTrackLastPlayedAt(t, d, "h2", now-8*86400)

	rows := evalQuery(t, d, db.SmartPlaylistQuery{
		Root: db.SmartRule{Field: db.FieldLastPlayed, Operator: db.OpInLast, Value: "7"},
	})
	if len(rows) != 1 || rows[0].Hash != "h1" {
		t.Errorf("got %d rows (hashes: %v), want 1 row with hash h1", len(rows), hashes(rows))
	}
}

// TestEvalSmartPlaylist_LastPlayedInLastExcludesUnplayed verifies that the
// "last_played in_last N days" filter does NOT match tracks that have never
// been played (last_played_at = 0).
func TestEvalSmartPlaylist_LastPlayedInLastExcludesUnplayed(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()

	insertFullTrack(t, d, "h1", "/a.flac", "Song1", "Artist", "Album", "rock", 180000, 5000000, now)
	insertFullTrack(t, d, "h2", "/b.flac", "Song2", "Artist", "Album", "rock", 180000, 5000000, now)
	setTrackLastPlayedAt(t, d, "h2", now-2*86400)

	rows := evalQuery(t, d, db.SmartPlaylistQuery{
		Root: db.SmartRule{Field: db.FieldLastPlayed, Operator: db.OpInLast, Value: "7"},
	})
	if len(rows) != 1 || rows[0].Hash != "h2" {
		t.Errorf("got %d rows (hashes: %v), want only h2 (unplayed h1 must be excluded)", len(rows), hashes(rows))
	}
}

func TestEvalSmartPlaylist_LastPlayedGreaterThan(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()

	insertFullTrack(t, d, "h1", "/a.flac", "Song1", "Artist", "Album", "rock", 180000, 5000000, now)
	insertFullTrack(t, d, "h2", "/b.flac", "Song2", "Artist", "Album", "rock", 180000, 5000000, now)

	threshold := now - 3600
	setTrackLastPlayedAt(t, d, "h1", now)

	rows := evalQuery(t, d, db.SmartPlaylistQuery{
		Root: db.SmartRule{
			Field:    db.FieldLastPlayed,
			Operator: db.OpGreaterThan,
			Value:    strconv.FormatInt(threshold, 10),
		},
	})
	if len(rows) != 1 || rows[0].Hash != "h1" {
		t.Errorf("got %d rows (hashes: %v), want 1 row with hash h1", len(rows), hashes(rows))
	}
}
