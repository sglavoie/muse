package db_test

import (
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/db"
)

func TestEvalSmartPlaylist_ArtistContains(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertFullTrack(t, d, "h1", "/a.flac", "Creep", "Radiohead", "Pablo Honey", "rock", 240000, 1000, now)
	insertFullTrack(t, d, "h2", "/b.flac", "Song2", "Other Artist", "Album", "pop", 180000, 800, now)

	q := db.SmartPlaylistQuery{
		Root: db.SmartRule{Field: db.FieldArtist, Operator: db.OpContains, Value: "radio"},
	}
	rows := evalQuery(t, d, q)
	if len(rows) != 1 || rows[0].Hash != "h1" {
		t.Errorf("got %d rows (hashes: %v), want 1 row with hash h1", len(rows), hashes(rows))
	}
}

func TestEvalSmartPlaylist_TitleEquals(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertFullTrack(t, d, "h1", "/a.flac", "Creep", "Radiohead", "Pablo Honey", "rock", 240000, 1000, now)
	insertFullTrack(t, d, "h2", "/b.flac", "Karma Police", "Radiohead", "OK Computer", "rock", 270000, 1100, now)

	q := db.SmartPlaylistQuery{
		Root: db.SmartRule{Field: db.FieldTitle, Operator: db.OpEquals, Value: "Creep"},
	}
	rows := evalQuery(t, d, q)
	if len(rows) != 1 || rows[0].Hash != "h1" {
		t.Errorf("got %d rows, want 1 row with hash h1", len(rows))
	}
}

func TestEvalSmartPlaylist_GenreStartsWith(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertFullTrack(t, d, "h1", "/a.flac", "Song A", "Artist", "Album", "post-rock", 200000, 1000, now)
	insertFullTrack(t, d, "h2", "/b.flac", "Song B", "Artist", "Album", "electronic", 200000, 1000, now)

	q := db.SmartPlaylistQuery{
		Root: db.SmartRule{Field: db.FieldGenre, Operator: db.OpStartsWith, Value: "post"},
	}
	rows := evalQuery(t, d, q)
	if len(rows) != 1 || rows[0].Hash != "h1" {
		t.Errorf("got %d rows, want 1 (post-rock)", len(rows))
	}
}

func TestEvalSmartPlaylist_AlbumEndsWith(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertFullTrack(t, d, "h1", "/a.flac", "Track", "Artist", "Kid A", "rock", 200000, 1000, now)
	insertFullTrack(t, d, "h2", "/b.flac", "Track2", "Artist", "OK Computer", "rock", 200000, 1000, now)

	q := db.SmartPlaylistQuery{
		Root: db.SmartRule{Field: db.FieldAlbum, Operator: db.OpEndsWith, Value: "A"},
	}
	rows := evalQuery(t, d, q)
	if len(rows) != 1 || rows[0].Hash != "h1" {
		t.Errorf("got %d rows, want 1 (Kid A)", len(rows))
	}
}

func TestEvalSmartPlaylist_TitleNotContains(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertFullTrack(t, d, "h1", "/a.flac", "Creep", "Radiohead", "Pablo Honey", "rock", 200000, 1000, now)
	insertFullTrack(t, d, "h2", "/b.flac", "Karma Police", "Radiohead", "OK Computer", "rock", 200000, 1000, now)

	q := db.SmartPlaylistQuery{
		Root: db.SmartRule{Field: db.FieldTitle, Operator: db.OpNotContains, Value: "Creep"},
	}
	rows := evalQuery(t, d, q)
	if len(rows) != 1 || rows[0].Hash != "h2" {
		t.Errorf("got %d rows (hashes: %v), want 1 row h2", len(rows), hashes(rows))
	}
}

func TestEvalSmartPlaylist_ContainsLiteralPercent(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertFullTrack(t, d, "h1", "/a.flac", "100% Pure", "Artist", "Album", "rock", 200000, 1000, now)
	insertFullTrack(t, d, "h2", "/b.flac", "Pure Gold", "Artist", "Album", "rock", 200000, 1000, now)

	q := db.SmartPlaylistQuery{
		Root: db.SmartRule{Field: db.FieldTitle, Operator: db.OpContains, Value: "%"},
	}
	rows := evalQuery(t, d, q)
	if len(rows) != 1 || rows[0].Hash != "h1" {
		t.Errorf("got %d rows (hashes: %v), want 1 row h1 (title containing literal %%)", len(rows), hashes(rows))
	}
}

func TestEvalSmartPlaylist_ContainsLiteralUnderscore(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertFullTrack(t, d, "h1", "/a.flac", "track_01", "Artist", "Album", "rock", 200000, 1000, now)
	insertFullTrack(t, d, "h2", "/b.flac", "track-02", "Artist", "Album", "rock", 200000, 1000, now)

	q := db.SmartPlaylistQuery{
		Root: db.SmartRule{Field: db.FieldTitle, Operator: db.OpContains, Value: "_"},
	}
	rows := evalQuery(t, d, q)
	if len(rows) != 1 || rows[0].Hash != "h1" {
		t.Errorf("got %d rows (hashes: %v), want 1 row h1 (title containing literal _)", len(rows), hashes(rows))
	}
}

func TestEvalSmartPlaylist_StartsWithLiteralPercent(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertFullTrack(t, d, "h1", "/a.flac", "%special", "Artist", "Album", "rock", 200000, 1000, now)
	insertFullTrack(t, d, "h2", "/b.flac", "normal", "Artist", "Album", "rock", 200000, 1000, now)

	q := db.SmartPlaylistQuery{
		Root: db.SmartRule{Field: db.FieldTitle, Operator: db.OpStartsWith, Value: "%"},
	}
	rows := evalQuery(t, d, q)
	if len(rows) != 1 || rows[0].Hash != "h1" {
		t.Errorf("got %d rows (hashes: %v), want 1 row h1 (title starting with literal %%)", len(rows), hashes(rows))
	}
}

func TestEvalSmartPlaylist_NotContainsLiteralPercent(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertFullTrack(t, d, "h1", "/a.flac", "100% Pure", "Artist", "Album", "rock", 200000, 1000, now)
	insertFullTrack(t, d, "h2", "/b.flac", "Pure Gold", "Artist", "Album", "rock", 200000, 1000, now)

	q := db.SmartPlaylistQuery{
		Root: db.SmartRule{Field: db.FieldTitle, Operator: db.OpNotContains, Value: "%"},
	}
	rows := evalQuery(t, d, q)
	if len(rows) != 1 || rows[0].Hash != "h2" {
		t.Errorf("got %d rows (hashes: %v), want 1 row h2 (title not containing literal %%)", len(rows), hashes(rows))
	}
}
