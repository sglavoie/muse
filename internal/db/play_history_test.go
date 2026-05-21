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
	if err := w.RecordPlay("h1", playedAt, 120, 0); err != nil {
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
		if err := w.RecordPlay("h1", now, 90, 0); err != nil {
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
	if err := w.RecordPlay("h1", playedAt, 60, 0); err != nil {
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

	stats, err := d.ReadListeningStats(0, db.DefaultMixCountConfig())
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
	if err := w.RecordPlay("h1", now, 120, 0); err != nil {
		t.Fatalf("RecordPlay: %v", err)
	}
	if err := w.RecordPlay("h1", now+1, 80, 0); err != nil {
		t.Fatalf("RecordPlay: %v", err)
	}

	stats, err := d.ReadListeningStats(0, db.DefaultMixCountConfig())
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
	if err := w.RecordPlay("h1", old, 100, 0); err != nil {
		t.Fatalf("RecordPlay old: %v", err)
	}
	if err := w.RecordPlay("h1", recent, 50, 0); err != nil {
		t.Fatalf("RecordPlay recent: %v", err)
	}

	// since = yesterday: only the recent play should count
	yesterday := time.Now().Add(-24 * time.Hour).Unix()
	stats, err := d.ReadListeningStats(yesterday, db.DefaultMixCountConfig())
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
		if err := w.RecordPlay("h1", now, 60, 0); err != nil {
			t.Fatalf("RecordPlay h1: %v", err)
		}
	}
	if err := w.RecordPlay("h2", now, 60, 0); err != nil {
		t.Fatalf("RecordPlay h2: %v", err)
	}

	stats, err := d.ReadListeningStats(0, db.DefaultMixCountConfig())
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

// TestEvalSmartPlaylist_LastPlayedInLastExcludesUnplayed verifies that the
// "last_played in_last N days" filter does NOT match tracks that have never
// been played (last_played_at = 0). Without the > 0 guard, the comparison
// 0 >= (unixepoch('now') - N*86400) is true for any positive N, so unplayed
// tracks would silently match the filter.
func TestEvalSmartPlaylist_LastPlayedInLastExcludesUnplayed(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	// h1: never played (last_played_at stays at 0).
	insertFullTrack(t, d, "h1", "/a.flac", "Song1", "Artist", "Album", "rock", 180000, 5000000, now)
	// h2: played 2 days ago (within the 7-day window).
	insertFullTrack(t, d, "h2", "/b.flac", "Song2", "Artist", "Album", "rock", 180000, 5000000, now)
	if _, err := d.Exec(
		`UPDATE tracks SET last_played_at = ? WHERE hash = ?`, now-2*86400, "h2",
	); err != nil {
		t.Fatalf("update h2: %v", err)
	}

	q := db.SmartPlaylistQuery{
		Root: db.SmartRule{Field: db.FieldLastPlayed, Operator: db.OpInLast, Value: "7"},
	}
	rows := evalQuery(t, d, q)
	if len(rows) != 1 || rows[0].Hash != "h2" {
		t.Errorf("got %d rows (hashes: %v), want only h2 (unplayed h1 must be excluded)",
			len(rows), hashes(rows))
	}
}

// --- Streak calculation ---

func TestReadListeningStats_StreakToday(t *testing.T) {
	d := openMigratedDB(t)
	insertTrack(t, d, "h1", "/a.flac")

	// Insert plays for today, yesterday, and the day before.
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, time.Local).Unix()
	yesterday := time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, time.Local).AddDate(0, 0, -1).Unix()
	twoDaysAgo := time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, time.Local).AddDate(0, 0, -2).Unix()

	for _, ts := range []int64{today, yesterday, twoDaysAgo} {
		if _, err := d.Exec(
			`INSERT INTO play_history (track_hash, played_at, listened_secs) VALUES (?, ?, ?)`,
			"h1", ts, 60,
		); err != nil {
			t.Fatalf("insert play_history: %v", err)
		}
	}

	stats, err := d.ReadListeningStats(0, db.DefaultMixCountConfig())
	if err != nil {
		t.Fatalf("ReadListeningStats: %v", err)
	}
	if stats.CurrentStreak != 3 {
		t.Errorf("CurrentStreak = %d, want 3", stats.CurrentStreak)
	}
}

func TestReadListeningStats_StreakStartingYesterday(t *testing.T) {
	d := openMigratedDB(t)
	insertTrack(t, d, "h1", "/a.flac")

	// Insert plays for yesterday and the day before, but NOT today.
	// The streak should still be 2.
	now := time.Now()
	yesterday := time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, time.Local).AddDate(0, 0, -1).Unix()
	twoDaysAgo := time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, time.Local).AddDate(0, 0, -2).Unix()

	for _, ts := range []int64{yesterday, twoDaysAgo} {
		if _, err := d.Exec(
			`INSERT INTO play_history (track_hash, played_at, listened_secs) VALUES (?, ?, ?)`,
			"h1", ts, 60,
		); err != nil {
			t.Fatalf("insert play_history: %v", err)
		}
	}

	stats, err := d.ReadListeningStats(0, db.DefaultMixCountConfig())
	if err != nil {
		t.Fatalf("ReadListeningStats: %v", err)
	}
	if stats.CurrentStreak != 2 {
		t.Errorf("CurrentStreak = %d, want 2 (streak should count from yesterday)", stats.CurrentStreak)
	}
}

func TestReadListeningStats_StreakBroken(t *testing.T) {
	d := openMigratedDB(t)
	insertTrack(t, d, "h1", "/a.flac")

	// Insert a play only 3 days ago — no play today or yesterday.
	// The streak should be 0.
	now := time.Now()
	threeDaysAgo := time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, time.Local).AddDate(0, 0, -3).Unix()

	if _, err := d.Exec(
		`INSERT INTO play_history (track_hash, played_at, listened_secs) VALUES (?, ?, ?)`,
		"h1", threeDaysAgo, 60,
	); err != nil {
		t.Fatalf("insert play_history: %v", err)
	}

	stats, err := d.ReadListeningStats(0, db.DefaultMixCountConfig())
	if err != nil {
		t.Fatalf("ReadListeningStats: %v", err)
	}
	if stats.CurrentStreak != 0 {
		t.Errorf("CurrentStreak = %d, want 0 (streak broken — no play today or yesterday)", stats.CurrentStreak)
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

// --- RecordPlay completion_rate ---

func TestRecordPlay_CompletionRate_Computed(t *testing.T) {
	d := openMigratedDB(t)
	insertTrack(t, d, "h1", "/a.flac")
	w := newWriter(t, d)

	// 60 seconds listened out of 120 seconds = 0.5 completion rate.
	if err := w.RecordPlay("h1", time.Now().Unix(), 60, 120.0); err != nil {
		t.Fatalf("RecordPlay: %v", err)
	}

	var rate float64
	if err := d.QueryRow(`SELECT completion_rate FROM play_history WHERE track_hash = ?`, "h1").Scan(&rate); err != nil {
		t.Fatalf("query completion_rate: %v", err)
	}
	if rate != 0.5 {
		t.Errorf("completion_rate = %f, want 0.5", rate)
	}
}

func TestRecordPlay_CompletionRate_ZeroDuration(t *testing.T) {
	d := openMigratedDB(t)
	insertTrack(t, d, "h1", "/a.flac")
	w := newWriter(t, d)

	// Zero duration: completion_rate must be 0, not NaN or panic.
	if err := w.RecordPlay("h1", time.Now().Unix(), 60, 0); err != nil {
		t.Fatalf("RecordPlay: %v", err)
	}

	var rate float64
	if err := d.QueryRow(`SELECT completion_rate FROM play_history WHERE track_hash = ?`, "h1").Scan(&rate); err != nil {
		t.Fatalf("query completion_rate: %v", err)
	}
	if rate != 0.0 {
		t.Errorf("completion_rate = %f, want 0.0 for zero duration", rate)
	}
}

func TestRecordPlay_CompletionRate_CapsAt1(t *testing.T) {
	d := openMigratedDB(t)
	insertTrack(t, d, "h1", "/a.flac")
	w := newWriter(t, d)

	// Listened longer than the track duration (e.g. due to clock drift) — capped at 1.0.
	if err := w.RecordPlay("h1", time.Now().Unix(), 180, 120.0); err != nil {
		t.Fatalf("RecordPlay: %v", err)
	}

	var rate float64
	if err := d.QueryRow(`SELECT completion_rate FROM play_history WHERE track_hash = ?`, "h1").Scan(&rate); err != nil {
		t.Fatalf("query completion_rate: %v", err)
	}
	if rate != 1.0 {
		t.Errorf("completion_rate = %f, want 1.0 (capped)", rate)
	}
}

func TestRecordPlay_CompletionRate_FloorsAtZero(t *testing.T) {
	d := openMigratedDB(t)
	insertTrack(t, d, "h1", "/a.flac")
	w := newWriter(t, d)

	// A negative listenedSecs (e.g. from a clock skew) must not produce a
	// negative completion_rate, which would drag down AVG() in deep-cuts /
	// rates queries.
	if err := w.RecordPlay("h1", time.Now().Unix(), -10, 120.0); err != nil {
		t.Fatalf("RecordPlay: %v", err)
	}

	var rate float64
	if err := d.QueryRow(`SELECT completion_rate FROM play_history WHERE track_hash = ?`, "h1").Scan(&rate); err != nil {
		t.Fatalf("query completion_rate: %v", err)
	}
	if rate != 0.0 {
		t.Errorf("completion_rate = %f, want 0.0 for negative listenedSecs", rate)
	}
}

func TestRecordPlay_ListenedSecs_FloorsAtZero(t *testing.T) {
	d := openMigratedDB(t)
	insertTrack(t, d, "h1", "/a.flac")
	w := newWriter(t, d)

	// A negative listenedSecs must also be clamped before insertion so it
	// does not corrupt SUM(listened_secs) aggregates used in stats and
	// top-N queries.
	if err := w.RecordPlay("h1", time.Now().Unix(), -10, 120.0); err != nil {
		t.Fatalf("RecordPlay: %v", err)
	}

	var listened int
	if err := d.QueryRow(`SELECT listened_secs FROM play_history WHERE track_hash = ?`, "h1").Scan(&listened); err != nil {
		t.Fatalf("query listened_secs: %v", err)
	}
	if listened != 0 {
		t.Errorf("listened_secs = %d, want 0 for negative input", listened)
	}

	// Mix a negative play with a normal play: TotalListenedSecs must equal
	// only the normal contribution (negative is floored to 0, not summed).
	if err := w.RecordPlay("h1", time.Now().Unix(), 60, 120.0); err != nil {
		t.Fatalf("RecordPlay: %v", err)
	}
	stats, err := d.ReadListeningStats(0, db.DefaultMixCountConfig())
	if err != nil {
		t.Fatalf("ReadListeningStats: %v", err)
	}
	if stats.TotalListenedSecs != 60 {
		t.Errorf("TotalListenedSecs = %d, want 60 (negative play floored to 0)", stats.TotalListenedSecs)
	}
}
