package db_test

import (
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/db"
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

// --- String field tests ---

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

// --- LIKE ESCAPE tests ---

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

// --- Numeric field tests ---

func TestEvalSmartPlaylist_DurationLessThan(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	// 3 min (180 s) and 5 min (300 s)
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

// --- Date field ---

func TestEvalSmartPlaylist_InLast(t *testing.T) {
	d := openMigratedDB(t)
	recentTime := time.Now().Unix() - 86400 // 1 day ago
	oldTime := time.Now().Unix() - 30*86400 // 30 days ago

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

// --- Favorite sub-select ---

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

// --- AND / OR groups ---

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
	// artist=X AND (genre=rock OR genre=pop)
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

// --- Empty results ---

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

// --- Sorting ---

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

// --- MaxTracks ---

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

// --- Missing tracks excluded ---

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

// --- IncrementPlayCount ---

func TestIncrementPlayCount(t *testing.T) {
	d := openMigratedDB(t)
	insertTrack(t, d, "hashPC", "/pc.flac")

	w := newWriter(t, d)

	if err := w.IncrementPlayCount("hashPC"); err != nil {
		t.Fatalf("IncrementPlayCount: %v", err)
	}
	if err := w.IncrementPlayCount("hashPC"); err != nil {
		t.Fatalf("IncrementPlayCount (2nd): %v", err)
	}

	var count int
	if err := d.QueryRow(`SELECT play_count FROM tracks WHERE hash = 'hashPC'`).Scan(&count); err != nil {
		t.Fatalf("query play_count: %v", err)
	}
	if count != 2 {
		t.Errorf("play_count = %d, want 2", count)
	}
}

func TestIncrementPlayCount_NoopForUnknownHash(t *testing.T) {
	d := openMigratedDB(t)
	w := newWriter(t, d)

	// Should not error even when no track matches.
	if err := w.IncrementPlayCount("nonexistent"); err != nil {
		t.Fatalf("IncrementPlayCount for unknown hash: %v", err)
	}
}

// --- CreateSmartPlaylist / UpdateSmartPlaylistQuery ---

func TestCreateSmartPlaylist(t *testing.T) {
	d := openMigratedDB(t)
	w := newWriter(t, d)

	q := db.SmartPlaylistQuery{
		Root: db.SmartRule{Field: db.FieldArtist, Operator: db.OpContains, Value: "test"},
	}
	raw, err := db.MarshalSmartPlaylistQuery(q)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	id, err := w.CreateSmartPlaylist("My Smart PL", raw)
	if err != nil {
		t.Fatalf("CreateSmartPlaylist: %v", err)
	}
	if id == 0 {
		t.Fatal("expected non-zero ID")
	}

	playlists, err := d.ReadPlaylists()
	if err != nil {
		t.Fatalf("ReadPlaylists: %v", err)
	}
	if len(playlists) != 1 {
		t.Fatalf("got %d playlists, want 1", len(playlists))
	}
	pl := playlists[0]
	if !pl.IsSmart {
		t.Error("IsSmart = false, want true")
	}
	if pl.TrackCount != -1 {
		t.Errorf("TrackCount = %d, want -1", pl.TrackCount)
	}
	if pl.Name != "My Smart PL" {
		t.Errorf("Name = %q, want %q", pl.Name, "My Smart PL")
	}
}

func TestUpdateSmartPlaylistQuery(t *testing.T) {
	d := openMigratedDB(t)
	w := newWriter(t, d)

	q1 := db.SmartPlaylistQuery{
		Root: db.SmartRule{Field: db.FieldArtist, Operator: db.OpEquals, Value: "A"},
	}
	raw1, _ := db.MarshalSmartPlaylistQuery(q1)
	id, err := w.CreateSmartPlaylist("PL", raw1)
	if err != nil {
		t.Fatalf("CreateSmartPlaylist: %v", err)
	}

	q2 := db.SmartPlaylistQuery{
		Root: db.SmartRule{Field: db.FieldGenre, Operator: db.OpEquals, Value: "jazz"},
	}
	raw2, _ := db.MarshalSmartPlaylistQuery(q2)
	if err := w.UpdateSmartPlaylistQuery(id, "PL-Renamed", raw2); err != nil {
		t.Fatalf("UpdateSmartPlaylistQuery: %v", err)
	}

	playlists, err := d.ReadPlaylists()
	if err != nil {
		t.Fatalf("ReadPlaylists: %v", err)
	}
	if len(playlists) != 1 {
		t.Fatalf("got %d playlists, want 1", len(playlists))
	}
	if playlists[0].Query != raw2 {
		t.Errorf("Query = %q, want %q", playlists[0].Query, raw2)
	}
	if playlists[0].Name != "PL-Renamed" {
		t.Errorf("Name = %q, want %q", playlists[0].Name, "PL-Renamed")
	}
}

// --- ReadPlaylists IsSmart field ---

func TestReadPlaylists_IsSmart(t *testing.T) {
	d := openMigratedDB(t)
	w := newWriter(t, d)

	// Create one regular and one smart playlist.
	_, err := w.CreatePlaylist("Regular")
	if err != nil {
		t.Fatalf("CreatePlaylist: %v", err)
	}
	q := db.SmartPlaylistQuery{
		Root: db.SmartRule{Field: db.FieldArtist, Operator: db.OpContains, Value: "x"},
	}
	raw, _ := db.MarshalSmartPlaylistQuery(q)
	_, err = w.CreateSmartPlaylist("Smart", raw)
	if err != nil {
		t.Fatalf("CreateSmartPlaylist: %v", err)
	}

	playlists, err := d.ReadPlaylists()
	if err != nil {
		t.Fatalf("ReadPlaylists: %v", err)
	}
	if len(playlists) != 2 {
		t.Fatalf("got %d playlists, want 2", len(playlists))
	}

	// Alphabetically: Regular < Smart
	reg := playlists[0]
	smart := playlists[1]
	if reg.IsSmart {
		t.Errorf("Regular playlist: IsSmart = true, want false")
	}
	if reg.TrackCount != 0 {
		t.Errorf("Regular playlist: TrackCount = %d, want 0", reg.TrackCount)
	}
	if !smart.IsSmart {
		t.Errorf("Smart playlist: IsSmart = false, want true")
	}
	if smart.TrackCount != -1 {
		t.Errorf("Smart playlist: TrackCount = %d, want -1", smart.TrackCount)
	}
}

// --- helpers ---

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

// --- has_tag tests ---

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

func TestEvalSmartPlaylist_HasTagEquals(t *testing.T) {
	d := openMigratedDB(t)
	w := newWriter(t, d)
	now := time.Now().Unix()
	insertFullTrack(t, d, "h1", "/a.flac", "Tagged", "A", "B", "rock", 200000, 1000, now)
	insertFullTrack(t, d, "h2", "/b.flac", "Untagged", "A", "B", "rock", 200000, 1000, now)

	tagID := insertTag(t, d, w, "workout")
	tagTrack(t, w, "h1", tagID)

	q := db.SmartPlaylistQuery{
		Root: db.SmartRule{Field: db.FieldTag, Operator: db.OpEquals, Value: "workout"},
	}
	rows := evalQuery(t, d, q)
	if len(rows) != 1 || rows[0].Hash != "h1" {
		t.Errorf("got %v, want [h1]", hashes(rows))
	}
}

func TestEvalSmartPlaylist_HasTagNotEquals(t *testing.T) {
	d := openMigratedDB(t)
	w := newWriter(t, d)
	now := time.Now().Unix()
	insertFullTrack(t, d, "h1", "/a.flac", "Tagged", "A", "B", "rock", 200000, 1000, now)
	insertFullTrack(t, d, "h2", "/b.flac", "Untagged", "A", "B", "rock", 200000, 1000, now)

	tagID := insertTag(t, d, w, "workout")
	tagTrack(t, w, "h1", tagID)

	q := db.SmartPlaylistQuery{
		Root: db.SmartRule{Field: db.FieldTag, Operator: db.OpNotEquals, Value: "workout"},
	}
	rows := evalQuery(t, d, q)
	if len(rows) != 1 || rows[0].Hash != "h2" {
		t.Errorf("got %v, want [h2]", hashes(rows))
	}
}

func TestEvalSmartPlaylist_HasTagSoftDeletedExcluded(t *testing.T) {
	d := openMigratedDB(t)
	w := newWriter(t, d)
	now := time.Now().Unix()
	insertFullTrack(t, d, "h1", "/a.flac", "Tagged", "A", "B", "rock", 200000, 1000, now)

	tagID := insertTag(t, d, w, "deleted-tag")
	tagTrack(t, w, "h1", tagID)
	if err := w.DeleteTagSoft(tagID); err != nil {
		t.Fatalf("DeleteTagSoft: %v", err)
	}

	q := db.SmartPlaylistQuery{
		Root: db.SmartRule{Field: db.FieldTag, Operator: db.OpEquals, Value: "deleted-tag"},
	}
	rows := evalQuery(t, d, q)
	if len(rows) != 0 {
		t.Errorf("got %v, want [] (soft-deleted tag should be excluded)", hashes(rows))
	}
}

func TestEvalSmartPlaylist_HasTagInAndGroup(t *testing.T) {
	d := openMigratedDB(t)
	w := newWriter(t, d)
	now := time.Now().Unix()
	insertFullTrack(t, d, "h1", "/a.flac", "Both", "Radiohead", "B", "rock", 200000, 1000, now)
	insertFullTrack(t, d, "h2", "/b.flac", "TagOnly", "Other", "B", "rock", 200000, 1000, now)
	insertFullTrack(t, d, "h3", "/c.flac", "ArtistOnly", "Radiohead", "B", "rock", 200000, 1000, now)

	tagID := insertTag(t, d, w, "chill")
	tagTrack(t, w, "h1", tagID)
	tagTrack(t, w, "h2", tagID)

	q := db.SmartPlaylistQuery{
		Root: db.SmartRule{
			Conjunction: db.ConjAnd,
			Rules: []db.SmartRule{
				{Field: db.FieldArtist, Operator: db.OpEquals, Value: "Radiohead"},
				{Field: db.FieldTag, Operator: db.OpEquals, Value: "chill"},
			},
		},
	}
	rows := evalQuery(t, d, q)
	if len(rows) != 1 || rows[0].Hash != "h1" {
		t.Errorf("got %v, want [h1]", hashes(rows))
	}
}

func TestEvalSmartPlaylist_HasTagInOrGroup(t *testing.T) {
	d := openMigratedDB(t)
	w := newWriter(t, d)
	now := time.Now().Unix()
	insertFullTrack(t, d, "h1", "/a.flac", "TaggedA", "A", "B", "rock", 200000, 1000, now)
	insertFullTrack(t, d, "h2", "/b.flac", "TaggedB", "A", "B", "rock", 200000, 1000, now)
	insertFullTrack(t, d, "h3", "/c.flac", "NoTag", "A", "B", "rock", 200000, 1000, now)

	tagA := insertTag(t, d, w, "workout")
	tagB := insertTag(t, d, w, "driving")
	tagTrack(t, w, "h1", tagA)
	tagTrack(t, w, "h2", tagB)

	q := db.SmartPlaylistQuery{
		Root: db.SmartRule{
			Conjunction: db.ConjOr,
			Rules: []db.SmartRule{
				{Field: db.FieldTag, Operator: db.OpEquals, Value: "workout"},
				{Field: db.FieldTag, Operator: db.OpEquals, Value: "driving"},
			},
		},
	}
	rows := evalQuery(t, d, q)
	got := hashes(rows)
	if len(got) != 2 {
		t.Fatalf("got %v, want 2 rows", got)
	}
	want := map[string]bool{"h1": true, "h2": true}
	for _, h := range got {
		if !want[h] {
			t.Errorf("unexpected hash %q in results", h)
		}
	}
}
