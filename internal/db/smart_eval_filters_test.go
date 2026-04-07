package db_test

import (
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/db"
)

func TestEvalSmartPlaylist_DurationLessThan(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertFullTrack(t, d, "short", "/s.flac", "Short", "A", "B", "rock", 180000, 1000, now)
	insertFullTrack(t, d, "long", "/l.flac", "Long", "A", "B", "rock", 300000, 1000, now)

	q := db.SmartPlaylistQuery{
		Root: db.SmartRule{Field: db.FieldDuration, Operator: db.OpLessThan, Value: "240"},
	}
	rows := evalQuery(t, d, q)
	if len(rows) != 1 || rows[0].Hash != "short" {
		t.Errorf("got %d rows, want 1 (short)", len(rows))
	}
}

func TestEvalSmartPlaylist_DurationGreaterThan(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertFullTrack(t, d, "short", "/s.flac", "Short", "A", "B", "rock", 180000, 1000, now)
	insertFullTrack(t, d, "long", "/l.flac", "Long", "A", "B", "rock", 600000, 1000, now)

	q := db.SmartPlaylistQuery{
		Root: db.SmartRule{Field: db.FieldDuration, Operator: db.OpGreaterThan, Value: "300"},
	}
	rows := evalQuery(t, d, q)
	if len(rows) != 1 || rows[0].Hash != "long" {
		t.Errorf("got %d rows, want 1 (long)", len(rows))
	}
}

func TestEvalSmartPlaylist_PlayCount(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertFullTrack(t, d, "h1", "/a.flac", "A", "X", "Y", "pop", 200000, 500, now)
	insertFullTrack(t, d, "h2", "/b.flac", "B", "X", "Y", "pop", 200000, 500, now)
	setPlayCount(t, d, "h1", 5)
	setPlayCount(t, d, "h2", 1)

	q := db.SmartPlaylistQuery{
		Root: db.SmartRule{Field: db.FieldPlayCount, Operator: db.OpGreaterThan, Value: "3"},
	}
	rows := evalQuery(t, d, q)
	if len(rows) != 1 || rows[0].Hash != "h1" {
		t.Errorf("got %d rows, want 1 (h1 with play_count=5)", len(rows))
	}
}

func TestEvalSmartPlaylist_InLast(t *testing.T) {
	d := openMigratedDB(t)
	recentTime := time.Now().Unix() - 86400
	oldTime := time.Now().Unix() - 30*86400

	insertFullTrack(t, d, "recent", "/r.flac", "Recent", "A", "B", "rock", 200000, 1000, recentTime)
	insertFullTrack(t, d, "old", "/o.flac", "Old", "A", "B", "rock", 200000, 1000, oldTime)

	q := db.SmartPlaylistQuery{
		Root: db.SmartRule{Field: db.FieldDateAdded, Operator: db.OpInLast, Value: "7"},
	}
	rows := evalQuery(t, d, q)
	if len(rows) != 1 || rows[0].Hash != "recent" {
		t.Errorf("got %d rows (hashes: %v), want 1 (recent)", len(rows), hashes(rows))
	}
}

func TestEvalSmartPlaylist_IsFavoriteTrue(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertFullTrack(t, d, "fav", "/f.flac", "Fav", "A", "B", "rock", 200000, 1000, now)
	insertFullTrack(t, d, "notfav", "/n.flac", "NotFav", "A", "B", "rock", 200000, 1000, now)
	insertFavoriteTrack(t, d, "fav")

	q := db.SmartPlaylistQuery{
		Root: db.SmartRule{Field: db.FieldFavorite, Operator: db.OpEquals, Value: "true"},
	}
	rows := evalQuery(t, d, q)
	if len(rows) != 1 || rows[0].Hash != "fav" {
		t.Errorf("got %d rows, want 1 (fav)", len(rows))
	}
}

func TestEvalSmartPlaylist_IsFavoriteFalse(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertFullTrack(t, d, "fav", "/f.flac", "Fav", "A", "B", "rock", 200000, 1000, now)
	insertFullTrack(t, d, "notfav", "/n.flac", "NotFav", "A", "B", "rock", 200000, 1000, now)
	insertFavoriteTrack(t, d, "fav")

	q := db.SmartPlaylistQuery{
		Root: db.SmartRule{Field: db.FieldFavorite, Operator: db.OpEquals, Value: "false"},
	}
	rows := evalQuery(t, d, q)
	if len(rows) != 1 || rows[0].Hash != "notfav" {
		t.Errorf("got %d rows, want 1 (notfav)", len(rows))
	}
}

func TestEvalSmartPlaylist_ExcludesMissingTracks(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertFullTrack(t, d, "present", "/p.flac", "Present", "A", "B", "rock", 200000, 1000, now)
	insertFullTrack(t, d, "missing", "/m.flac", "Missing", "A", "B", "rock", 200000, 1000, now)
	if _, err := d.Exec(`UPDATE tracks SET missing = 1 WHERE hash = 'missing'`); err != nil {
		t.Fatalf("mark missing: %v", err)
	}

	q := db.SmartPlaylistQuery{
		Root: db.SmartRule{Field: db.FieldArtist, Operator: db.OpEquals, Value: "A"},
	}
	rows := evalQuery(t, d, q)
	if len(rows) != 1 || rows[0].Hash != "present" {
		t.Errorf("got %d rows (hashes: %v), want 1 (present)", len(rows), hashes(rows))
	}
}
