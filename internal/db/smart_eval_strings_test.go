package db_test

import (
	"slices"
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

// Equals and not-equals ignore case like contains (LIKE) always has, so
// "radiohead" finds the same tracks either way.
func TestEvalSmartPlaylist_TextEqualsIgnoresCase(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertFullTrack(t, d, "h1", "/a.flac", "Creep", "Radiohead", "Pablo Honey", "rock", 200000, 1000, now)
	insertFullTrack(t, d, "h2", "/b.flac", "Song", "Other", "Album", "rock", 200000, 1000, now)

	eq := evalQuery(t, d, db.SmartPlaylistQuery{
		Root: db.SmartRule{Field: db.FieldArtist, Operator: db.OpEquals, Value: "radiohead"},
	})
	if len(eq) != 1 || eq[0].Hash != "h1" {
		t.Errorf("equals: got hashes %v, want [h1]", hashes(eq))
	}
	ne := evalQuery(t, d, db.SmartPlaylistQuery{
		Root: db.SmartRule{Field: db.FieldAlbum, Operator: db.OpNotEquals, Value: "PABLO HONEY"},
	})
	if len(ne) != 1 || ne[0].Hash != "h2" {
		t.Errorf("not equals: got hashes %v, want [h2]", hashes(ne))
	}
}

// Case is ignored beyond ASCII: SQLite's NOCASE and LIKE alone would let
// "björk" miss "BJÖRK". LIKE metacharacters in the value stay literal.
func TestEvalSmartPlaylist_TextIgnoresNonASCIICase(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertFullTrack(t, d, "h1", "/a.flac", "JÓGA", "BJÖRK", "HOMOGENIC", "ÉLECTRO", 200000, 1000, now)
	insertFullTrack(t, d, "h2", "/b.flac", "100% ΣΟΦΊΑ", "Other", "Album", "Rock", 200000, 1000, now)
	insertFullTrack(t, d, "h3", "/c.flac", "100 sofia", "Other", "Album", "Rock", 200000, 1000, now)
	syncTrackGenres(t, d)

	for _, tc := range []struct {
		name string
		rule db.SmartRule
		want []string
	}{
		{"contains", db.SmartRule{Field: db.FieldArtist, Operator: db.OpContains, Value: "björk"}, []string{"h1"}},
		{"equals", db.SmartRule{Field: db.FieldTitle, Operator: db.OpEquals, Value: "jóga"}, []string{"h1"}},
		{"not equals", db.SmartRule{Field: db.FieldArtist, Operator: db.OpNotEquals, Value: "björk"}, []string{"h3", "h2"}},
		{"starts with", db.SmartRule{Field: db.FieldTitle, Operator: db.OpStartsWith, Value: "100% σοφ"}, []string{"h2"}},
		{"ends with", db.SmartRule{Field: db.FieldTitle, Operator: db.OpEndsWith, Value: "σοφία"}, []string{"h2"}},
		{"not contains", db.SmartRule{Field: db.FieldArtist, Operator: db.OpNotContains, Value: "ö"}, []string{"h3", "h2"}},
		{"genre equals", db.SmartRule{Field: db.FieldGenre, Operator: db.OpEquals, Value: "électro"}, []string{"h1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := hashes(evalQuery(t, d, db.SmartPlaylistQuery{Root: tc.rule, SortBy: "title"}))
			if !slices.Equal(got, tc.want) {
				t.Errorf("got hashes %v, want %v", got, tc.want)
			}
		})
	}
}

// Genre equals/not-equals match one genre of a multi-genre track, the way the
// library splits genres, instead of the whole raw genre string.
func TestEvalSmartPlaylist_GenreEqualsMatchesEachGenre(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertFullTrack(t, d, "h1", "/a.flac", "A", "Artist", "Album", "Rock; Pop", 200000, 1000, now)
	insertFullTrack(t, d, "h2", "/b.flac", "B", "Artist", "Album", "Pop", 200000, 1000, now)
	insertFullTrack(t, d, "h3", "/c.flac", "C", "Artist", "Album", "Jazz", 200000, 1000, now)
	insertFullTrack(t, d, "h4", "/d.flac", "D", "Artist", "Album", "", 200000, 1000, now)
	syncTrackGenres(t, d)

	eq := evalQuery(t, d, db.SmartPlaylistQuery{
		Root:   db.SmartRule{Field: db.FieldGenre, Operator: db.OpEquals, Value: "pop"},
		SortBy: "title",
	})
	if got := hashes(eq); len(got) != 2 || got[0] != "h1" || got[1] != "h2" {
		t.Errorf("equals pop: got hashes %v, want [h1 h2]", got)
	}
	ne := evalQuery(t, d, db.SmartPlaylistQuery{
		Root:   db.SmartRule{Field: db.FieldGenre, Operator: db.OpNotEquals, Value: "rock"},
		SortBy: "title",
	})
	if got := hashes(ne); len(got) != 3 || got[0] != "h2" || got[1] != "h3" || got[2] != "h4" {
		t.Errorf("not equals rock: got hashes %v, want [h2 h3 h4]", got)
	}
	// An empty value still compares the raw column: "genre is empty" means an
	// untagged track.
	empty := evalQuery(t, d, db.SmartPlaylistQuery{
		Root: db.SmartRule{Field: db.FieldGenre, Operator: db.OpEquals, Value: ""},
	})
	if got := hashes(empty); len(got) != 1 || got[0] != "h4" {
		t.Errorf("equals empty: got hashes %v, want [h4]", got)
	}
	// Contains is unchanged and still matches the raw string.
	contains := evalQuery(t, d, db.SmartPlaylistQuery{
		Root: db.SmartRule{Field: db.FieldGenre, Operator: db.OpContains, Value: "k; P"},
	})
	if got := hashes(contains); len(got) != 1 || got[0] != "h1" {
		t.Errorf("contains: got hashes %v, want [h1]", got)
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
