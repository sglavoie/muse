package db_test

import (
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/db"
)

func TestEvalSmartPlaylist_AndGroup(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertFullTrack(t, d, "h1", "/a.flac", "Creep", "Radiohead", "Pablo Honey", "rock", 240000, 1000, now)
	insertFullTrack(t, d, "h2", "/b.flac", "Karma Police", "Radiohead", "OK Computer", "rock", 270000, 1000, now)
	insertFullTrack(t, d, "h3", "/c.flac", "Song", "Other", "Album", "pop", 180000, 1000, now)

	q := db.SmartPlaylistQuery{
		Root: db.SmartRule{
			Conjunction: db.ConjAnd,
			Rules: []db.SmartRule{
				{Field: db.FieldArtist, Operator: db.OpEquals, Value: "Radiohead"},
				{Field: db.FieldDuration, Operator: db.OpGreaterThan, Value: "250"},
			},
		},
	}
	rows := evalQuery(t, d, q)
	if len(rows) != 1 || rows[0].Hash != "h2" {
		t.Errorf("got %d rows (hashes: %v), want 1 (h2)", len(rows), hashes(rows))
	}
}

func TestEvalSmartPlaylist_OrGroup(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertFullTrack(t, d, "h1", "/a.flac", "Track A", "Artist A", "Album", "rock", 200000, 1000, now)
	insertFullTrack(t, d, "h2", "/b.flac", "Track B", "Artist B", "Album", "pop", 200000, 1000, now)
	insertFullTrack(t, d, "h3", "/c.flac", "Track C", "Artist C", "Album", "jazz", 200000, 1000, now)

	q := db.SmartPlaylistQuery{
		Root: db.SmartRule{
			Conjunction: db.ConjOr,
			Rules: []db.SmartRule{
				{Field: db.FieldGenre, Operator: db.OpEquals, Value: "rock"},
				{Field: db.FieldGenre, Operator: db.OpEquals, Value: "pop"},
			},
		},
	}
	rows := evalQuery(t, d, q)
	if len(rows) != 2 {
		t.Errorf("got %d rows, want 2", len(rows))
	}
}

func TestEvalSmartPlaylist_NestedGroups(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertFullTrack(t, d, "h1", "/a.flac", "A", "X", "Alb", "rock", 200000, 1000, now)
	insertFullTrack(t, d, "h2", "/b.flac", "B", "X", "Alb", "pop", 200000, 1000, now)
	insertFullTrack(t, d, "h3", "/c.flac", "C", "X", "Alb", "jazz", 200000, 1000, now)
	insertFullTrack(t, d, "h4", "/d.flac", "D", "Y", "Alb", "rock", 200000, 1000, now)

	q := db.SmartPlaylistQuery{
		Root: db.SmartRule{
			Conjunction: db.ConjAnd,
			Rules: []db.SmartRule{
				{Field: db.FieldArtist, Operator: db.OpEquals, Value: "X"},
				{
					Conjunction: db.ConjOr,
					Rules: []db.SmartRule{
						{Field: db.FieldGenre, Operator: db.OpEquals, Value: "rock"},
						{Field: db.FieldGenre, Operator: db.OpEquals, Value: "pop"},
					},
				},
			},
		},
	}
	rows := evalQuery(t, d, q)
	if len(rows) != 2 {
		t.Errorf("got %d rows (hashes: %v), want 2 (h1, h2)", len(rows), hashes(rows))
	}
}

func TestEvalSmartPlaylist_EmptyResults(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertFullTrack(t, d, "h1", "/a.flac", "Track", "Artist", "Album", "rock", 200000, 1000, now)

	q := db.SmartPlaylistQuery{
		Root: db.SmartRule{Field: db.FieldArtist, Operator: db.OpEquals, Value: "No Such Artist"},
	}
	rows := evalQuery(t, d, q)
	if len(rows) != 0 {
		t.Errorf("got %d rows, want 0", len(rows))
	}
}

func TestEvalSmartPlaylist_EmptyLibrary(t *testing.T) {
	d := openMigratedDB(t)

	q := db.SmartPlaylistQuery{
		Root: db.SmartRule{Field: db.FieldArtist, Operator: db.OpContains, Value: "anything"},
	}
	rows := evalQuery(t, d, q)
	if len(rows) != 0 {
		t.Errorf("got %d rows, want 0", len(rows))
	}
}

func TestEvalSmartPlaylist_SortByArtistAsc(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertFullTrack(t, d, "hZ", "/z.flac", "Z", "Zebra", "A", "rock", 200000, 1000, now)
	insertFullTrack(t, d, "hA", "/a.flac", "A", "Alpha", "B", "rock", 200000, 1000, now)

	q := db.SmartPlaylistQuery{
		Root:    db.SmartRule{Field: db.FieldArtist, Operator: db.OpNotEquals, Value: "nobody"},
		SortBy:  "artist",
		SortDir: "asc",
	}
	rows := evalQuery(t, d, q)
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	if rows[0].Hash != "hA" || rows[1].Hash != "hZ" {
		t.Errorf("order wrong: got %v, want [hA, hZ]", hashes(rows))
	}
}

func TestEvalSmartPlaylist_SortByArtistDesc(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertFullTrack(t, d, "hZ", "/z.flac", "Z", "Zebra", "A", "rock", 200000, 1000, now)
	insertFullTrack(t, d, "hA", "/a.flac", "A", "Alpha", "B", "rock", 200000, 1000, now)

	q := db.SmartPlaylistQuery{
		Root:    db.SmartRule{Field: db.FieldArtist, Operator: db.OpNotEquals, Value: "nobody"},
		SortBy:  "artist",
		SortDir: "desc",
	}
	rows := evalQuery(t, d, q)
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	if rows[0].Hash != "hZ" || rows[1].Hash != "hA" {
		t.Errorf("order wrong: got %v, want [hZ, hA]", hashes(rows))
	}
}

func TestEvalSmartPlaylist_SortByAlbumAsc(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertFullTrack(t, d, "hZ", "/z.flac", "Z", "Zebra", "Bravo", "rock", 200000, 1000, now)
	insertFullTrack(t, d, "hA", "/a.flac", "A", "Alpha", "Alpha", "rock", 200000, 1000, now)

	q := db.SmartPlaylistQuery{
		Root:    db.SmartRule{Field: db.FieldArtist, Operator: db.OpNotEquals, Value: "nobody"},
		SortBy:  "album",
		SortDir: "asc",
	}
	rows := evalQuery(t, d, q)
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	if rows[0].Hash != "hA" || rows[1].Hash != "hZ" {
		t.Errorf("order wrong: got %v, want [hA, hZ]", hashes(rows))
	}
}

func TestEvalSmartPlaylist_SortByAlbumDesc(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertFullTrack(t, d, "hZ", "/z.flac", "Z", "Zebra", "Bravo", "rock", 200000, 1000, now)
	insertFullTrack(t, d, "hA", "/a.flac", "A", "Alpha", "Alpha", "rock", 200000, 1000, now)

	q := db.SmartPlaylistQuery{
		Root:    db.SmartRule{Field: db.FieldArtist, Operator: db.OpNotEquals, Value: "nobody"},
		SortBy:  "album",
		SortDir: "desc",
	}
	rows := evalQuery(t, d, q)
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	if rows[0].Hash != "hZ" || rows[1].Hash != "hA" {
		t.Errorf("order wrong: got %v, want [hZ, hA]", hashes(rows))
	}
}

func TestEvalSmartPlaylist_MaxTracks(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	for i := range 5 {
		hash := string(rune('a'+i)) + "_hash"
		insertFullTrack(t, d, hash, "/"+hash+".flac", "T", "Artist", "B", "rock", 200000, 1000, now)
	}

	q := db.SmartPlaylistQuery{
		Root:      db.SmartRule{Field: db.FieldArtist, Operator: db.OpEquals, Value: "Artist"},
		MaxTracks: 3,
	}
	rows := evalQuery(t, d, q)
	if len(rows) != 3 {
		t.Errorf("got %d rows, want 3 (MaxTracks=3)", len(rows))
	}
}
