package db_test

import (
	"testing"

	"github.com/sglavoie/muse/internal/db"
)

type playHistoryHarness struct {
	d *db.DB
	w *db.Writer
}

func newPlayHistoryHarness(t *testing.T) playHistoryHarness {
	t.Helper()
	d := openMigratedDB(t)
	return playHistoryHarness{
		d: d,
		w: newWriter(t, d),
	}
}

func (h playHistoryHarness) addTrack(t *testing.T, hash, path string) {
	t.Helper()
	insertTrack(t, h.d, hash, path)
}

func (h playHistoryHarness) recordPlay(t *testing.T, hash string, playedAt int64, listenedSecs int, duration float64) {
	t.Helper()
	if err := h.w.RecordPlay(hash, playedAt, listenedSecs, duration); err != nil {
		t.Fatalf("RecordPlay(%q): %v", hash, err)
	}
}

func (h playHistoryHarness) readListeningStats(t *testing.T, since int64) db.ListeningStats {
	t.Helper()
	stats, err := h.d.ReadListeningStats(since, db.DefaultMixCountConfig())
	if err != nil {
		t.Fatalf("ReadListeningStats(%d): %v", since, err)
	}
	return stats
}

func insertPlayHistoryRow(t *testing.T, d *db.DB, hash string, playedAt int64, listenedSecs int) {
	t.Helper()
	if _, err := d.Exec(
		`INSERT INTO play_history (track_hash, played_at, listened_secs) VALUES (?, ?, ?)`,
		hash, playedAt, listenedSecs,
	); err != nil {
		t.Fatalf("insert play_history for %q: %v", hash, err)
	}
}

func setTrackLastPlayedAt(t *testing.T, d *db.DB, hash string, playedAt int64) {
	t.Helper()
	if _, err := d.Exec(`UPDATE tracks SET last_played_at = ? WHERE hash = ?`, playedAt, hash); err != nil {
		t.Fatalf("update last_played_at for %q: %v", hash, err)
	}
}

func playHistoryRowCount(t *testing.T, d *db.DB, hash string) int {
	t.Helper()
	var count int
	if err := d.QueryRow(`SELECT COUNT(*) FROM play_history WHERE track_hash = ?`, hash).Scan(&count); err != nil {
		t.Fatalf("query play_history count for %q: %v", hash, err)
	}
	return count
}

func trackPlayCount(t *testing.T, d *db.DB, hash string) int {
	t.Helper()
	var count int
	if err := d.QueryRow(`SELECT play_count FROM tracks WHERE hash = ?`, hash).Scan(&count); err != nil {
		t.Fatalf("query play_count for %q: %v", hash, err)
	}
	return count
}

func trackLastPlayedAt(t *testing.T, d *db.DB, hash string) int64 {
	t.Helper()
	var playedAt int64
	if err := d.QueryRow(`SELECT last_played_at FROM tracks WHERE hash = ?`, hash).Scan(&playedAt); err != nil {
		t.Fatalf("query last_played_at for %q: %v", hash, err)
	}
	return playedAt
}

func playHistoryCompletionRate(t *testing.T, d *db.DB, hash string) float64 {
	t.Helper()
	var rate float64
	if err := d.QueryRow(`SELECT completion_rate FROM play_history WHERE track_hash = ?`, hash).Scan(&rate); err != nil {
		t.Fatalf("query completion_rate for %q: %v", hash, err)
	}
	return rate
}

func playHistoryListenedSecs(t *testing.T, d *db.DB, hash string) int {
	t.Helper()
	var listened int
	if err := d.QueryRow(`SELECT listened_secs FROM play_history WHERE track_hash = ?`, hash).Scan(&listened); err != nil {
		t.Fatalf("query listened_secs for %q: %v", hash, err)
	}
	return listened
}
