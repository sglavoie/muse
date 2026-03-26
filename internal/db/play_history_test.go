package db_test

import (
	"strconv"
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/db"
)

// --- RecordPlay ---

func TestRecordPlay_InsertsHistoryRow(t *testing.T) {
	d := openMigratedDB(t)
	insertTrack(t, d, "h1", "/a.flac")
	w := newWriter(t, d)

	playedAt := time.Now().Unix()
	if err := w.RecordPlay("h1", playedAt, 120); err != nil {
		t.Fatalf("RecordPlay: %v", err)
	}

	var count int
	if err := d.QueryRow(`SELECT COUNT(*) FROM play_history WHERE track_hash = ?`, "h1").Scan(&count); err != nil {
		t.Fatalf("query play_history: %v", err)
	}
	if count != 1 {
		t.Errorf("play_history rows = %d, want 1", count)
	}
}

func TestRecordPlay_IncrementsPlayCount(t *testing.T) {
	d := openMigratedDB(t)
	insertTrack(t, d, "h1", "/a.flac")
	w := newWriter(t, d)

	now := time.Now().Unix()
	for range 3 {
		if err := w.RecordPlay("h1", now, 90); err != nil {
			t.Fatalf("RecordPlay: %v", err)
		}
	}

	var count int
	if err := d.QueryRow(`SELECT play_count FROM tracks WHERE hash = ?`, "h1").Scan(&count); err != nil {
		t.Fatalf("query play_count: %v", err)
	}
	if count != 3 {
		t.Errorf("play_count = %d, want 3", count)
	}
}

func TestRecordPlay_SetsLastPlayedAt(t *testing.T) {
	d := openMigratedDB(t)
	insertTrack(t, d, "h1", "/a.flac")
	w := newWriter(t, d)

	playedAt := time.Now().Unix()
	if err := w.RecordPlay("h1", playedAt, 60); err != nil {
		t.Fatalf("RecordPlay: %v", err)
	}

	var got int64
	if err := d.QueryRow(`SELECT last_played_at FROM tracks WHERE hash = ?`, "h1").Scan(&got); err != nil {
		t.Fatalf("query last_played_at: %v", err)
	}
	if got != playedAt {
		t.Errorf("last_played_at = %d, want %d", got, playedAt)
	}
}

// --- ReadListeningStats ---

func TestReadListeningStats_Empty(t *testing.T) {
	d := openMigratedDB(t)

	stats, err := d.ReadListeningStats(0)
	if err != nil {
		t.Fatalf("ReadListeningStats: %v", err)
	}
	if stats.TotalPlays != 0 {
		t.Errorf("TotalPlays = %d, want 0", stats.TotalPlays)
	}
	if stats.TotalListenedSecs != 0 {
		t.Errorf("TotalListenedSecs = %d, want 0", stats.TotalListenedSecs)
	}
	if stats.CurrentStreak != 0 {
		t.Errorf("CurrentStreak = %d, want 0", stats.CurrentStreak)
	}
}

func TestReadListeningStats_Totals(t *testing.T) {
	d := openMigratedDB(t)
	insertTrack(t, d, "h1", "/a.flac")
	w := newWriter(t, d)

	now := time.Now().Unix()
	if err := w.RecordPlay("h1", now, 120); err != nil {
		t.Fatalf("RecordPlay: %v", err)
	}
	if err := w.RecordPlay("h1", now+1, 80); err != nil {
		t.Fatalf("RecordPlay: %v", err)
	}

	stats, err := d.ReadListeningStats(0)
	if err != nil {
		t.Fatalf("ReadListeningStats: %v", err)
	}
	if stats.TotalPlays != 2 {
		t.Errorf("TotalPlays = %d, want 2", stats.TotalPlays)
	}
	if stats.TotalListenedSecs != 200 {
		t.Errorf("TotalListenedSecs = %d, want 200", stats.TotalListenedSecs)
	}
}

func TestReadListeningStats_SinceFilter(t *testing.T) {
	d := openMigratedDB(t)
	insertTrack(t, d, "h1", "/a.flac")
	w := newWriter(t, d)

	old := time.Now().Add(-48 * time.Hour).Unix()
	recent := time.Now().Unix()
	if err := w.RecordPlay("h1", old, 100); err != nil {
		t.Fatalf("RecordPlay old: %v", err)
	}
	if err := w.RecordPlay("h1", recent, 50); err != nil {
		t.Fatalf("RecordPlay recent: %v", err)
	}

	// since = yesterday: only the recent play should count
	yesterday := time.Now().Add(-24 * time.Hour).Unix()
	stats, err := d.ReadListeningStats(yesterday)
	if err != nil {
		t.Fatalf("ReadListeningStats: %v", err)
	}
	if stats.TotalPlays != 1 {
		t.Errorf("TotalPlays = %d, want 1", stats.TotalPlays)
	}
	if stats.TotalListenedSecs != 50 {
		t.Errorf("TotalListenedSecs = %d, want 50", stats.TotalListenedSecs)
	}
}

func TestReadListeningStats_TopArtists(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	// Artist A: 3 plays, Artist B: 1 play
	insertFullTrack(t, d, "h1", "/a.flac", "Song1", "ArtistA", "AlbumA", "rock", 180000, 5000000, now)
	insertFullTrack(t, d, "h2", "/b.flac", "Song2", "ArtistB", "AlbumB", "pop", 200000, 6000000, now)

	w := newWriter(t, d)
	for range 3 {
		if err := w.RecordPlay("h1", now, 60); err != nil {
			t.Fatalf("RecordPlay h1: %v", err)
		}
	}
	if err := w.RecordPlay("h2", now, 60); err != nil {
		t.Fatalf("RecordPlay h2: %v", err)
	}

	stats, err := d.ReadListeningStats(0)
	if err != nil {
		t.Fatalf("ReadListeningStats: %v", err)
	}
	if len(stats.TopArtists) == 0 {
		t.Fatal("TopArtists is empty")
	}
	if stats.TopArtists[0].Name != "ArtistA" {
		t.Errorf("TopArtists[0].Name = %q, want %q", stats.TopArtists[0].Name, "ArtistA")
	}
	if stats.TopArtists[0].PlayCount != 3 {
		t.Errorf("TopArtists[0].PlayCount = %d, want 3", stats.TopArtists[0].PlayCount)
	}
}

// --- FieldLastPlayed smart rule ---

func TestEvalSmartPlaylist_LastPlayedInLast(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertFullTrack(t, d, "h1", "/a.flac", "Song1", "Artist", "Album", "rock", 180000, 5000000, now)
	insertFullTrack(t, d, "h2", "/b.flac", "Song2", "Artist", "Album", "rock", 180000, 5000000, now)

	// h1: played 1 hour ago (within 7 days), h2: played 8 days ago (outside)
	_, err := d.Exec(`UPDATE tracks SET last_played_at = ? WHERE hash = ?`, now-3600, "h1")
	if err != nil {
		t.Fatalf("update h1: %v", err)
	}
	_, err = d.Exec(`UPDATE tracks SET last_played_at = ? WHERE hash = ?`, now-8*86400, "h2")
	if err != nil {
		t.Fatalf("update h2: %v", err)
	}

	q := db.SmartPlaylistQuery{
		Root: db.SmartRule{Field: db.FieldLastPlayed, Operator: db.OpInLast, Value: "7"},
	}
	rows := evalQuery(t, d, q)
	if len(rows) != 1 || rows[0].Hash != "h1" {
		t.Errorf("got %d rows (hashes: %v), want 1 row with hash h1", len(rows), hashes(rows))
	}
}

func TestEvalSmartPlaylist_LastPlayedGreaterThan(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertFullTrack(t, d, "h1", "/a.flac", "Song1", "Artist", "Album", "rock", 180000, 5000000, now)
	insertFullTrack(t, d, "h2", "/b.flac", "Song2", "Artist", "Album", "rock", 180000, 5000000, now)

	threshold := now - 3600
	_, err := d.Exec(`UPDATE tracks SET last_played_at = ? WHERE hash = ?`, now, "h1")
	if err != nil {
		t.Fatalf("update h1: %v", err)
	}
	// h2 last_played_at stays at 0 (never played)

	q := db.SmartPlaylistQuery{
		Root: db.SmartRule{
			Field:    db.FieldLastPlayed,
			Operator: db.OpGreaterThan,
			Value:    strconv.FormatInt(threshold, 10),
		},
	}
	rows := evalQuery(t, d, q)
	if len(rows) != 1 || rows[0].Hash != "h1" {
		t.Errorf("got %d rows (hashes: %v), want 1 row with hash h1", len(rows), hashes(rows))
	}
}
