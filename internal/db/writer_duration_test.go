package db_test

import (
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
)

func insertTrackForDuration(t *testing.T, d *db.DB, hash, path string, durationMs int64) {
	t.Helper()
	now := time.Now().Unix()
	if _, err := d.Exec(
		`INSERT INTO tracks (hash, path, duration_ms, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?)`,
		hash, path, durationMs, now, now,
	); err != nil {
		t.Fatalf("insertTrackForDuration %q: %v", hash, err)
	}
}

// TestVerifyTrackDuration verifies that VerifyTrackDuration updates duration_ms
// and sets duration_verified=1 for the matching track.
func TestVerifyTrackDuration(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)

	insertTrackForDuration(t, d, "hash-v", "/a.mp3", 120000)

	if err := w.VerifyTrackDuration("hash-v", 300000); err != nil {
		t.Fatalf("VerifyTrackDuration: %v", err)
	}

	var durationMs int64
	var verified int
	if err := d.QueryRow(
		`SELECT duration_ms, duration_verified FROM tracks WHERE hash='hash-v'`,
	).Scan(&durationMs, &verified); err != nil {
		t.Fatalf("select: %v", err)
	}
	if durationMs != 300000 {
		t.Errorf("duration_ms = %d; want 300000", durationMs)
	}
	if verified != 1 {
		t.Errorf("duration_verified = %d; want 1", verified)
	}
}

// TestMarkDurationVerified verifies that MarkDurationVerified sets
// duration_verified=1 without changing duration_ms.
func TestMarkDurationVerified(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)

	insertTrackForDuration(t, d, "hash-m", "/b.mp3", 180000)

	if err := w.MarkDurationVerified("hash-m"); err != nil {
		t.Fatalf("MarkDurationVerified: %v", err)
	}

	var durationMs int64
	var verified int
	if err := d.QueryRow(
		`SELECT duration_ms, duration_verified FROM tracks WHERE hash='hash-m'`,
	).Scan(&durationMs, &verified); err != nil {
		t.Fatalf("select: %v", err)
	}
	if durationMs != 180000 {
		t.Errorf("duration_ms = %d; want 180000 (unchanged)", durationMs)
	}
	if verified != 1 {
		t.Errorf("duration_verified = %d; want 1", verified)
	}
}

// TestReadUnverifiedDurations verifies that ReadUnverifiedDurations returns
// only non-missing tracks with duration_verified=0.
func TestReadUnverifiedDurations(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)

	tracks := []library.Track{
		{Hash: "hash-un1", Path: "/a.mp3", Title: "A", Duration: 120.0},
		{Hash: "hash-un2", Path: "/b.mp3", Title: "B", Duration: 180.0},
		{Hash: "hash-un3", Path: "/c.mp3", Title: "C", Duration: 240.0},
	}
	if err := w.UpsertBatch(tracks); err != nil {
		t.Fatalf("UpsertBatch: %v", err)
	}

	// Mark one verified and one missing.
	if err := w.MarkDurationVerified("hash-un2"); err != nil {
		t.Fatalf("MarkDurationVerified: %v", err)
	}
	if err := w.MarkTrackMissing("hash-un3"); err != nil {
		t.Fatalf("MarkTrackMissing: %v", err)
	}

	rows, err := d.ReadUnverifiedDurations()
	if err != nil {
		t.Fatalf("ReadUnverifiedDurations: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows; want 1 (only unverified, non-missing)", len(rows))
	}
	if rows[0].Hash != "hash-un1" {
		t.Errorf("hash = %q; want hash-un1", rows[0].Hash)
	}
	if rows[0].DurationMs != 120000 {
		t.Errorf("DurationMs = %d; want 120000", rows[0].DurationMs)
	}
}

// TestUpsertBatch_ResetsDurationVerified verifies that re-upserting a track
// resets duration_verified=0, even if it was previously verified.
func TestUpsertBatch_ResetsDurationVerified(t *testing.T) {
	d := openMigratedTestDB(t)
	w := db.NewWriter(t.Context(), d.DB)

	track := library.Track{Hash: "hash-dv", Path: "/x.mp3", Title: "X", Duration: 60.0}
	if err := w.UpsertBatch([]library.Track{track}); err != nil {
		t.Fatalf("first UpsertBatch: %v", err)
	}
	if err := w.MarkDurationVerified("hash-dv"); err != nil {
		t.Fatalf("MarkDurationVerified: %v", err)
	}

	// Verify flag is set.
	var v int
	if err := d.QueryRow(`SELECT duration_verified FROM tracks WHERE hash='hash-dv'`).Scan(&v); err != nil {
		t.Fatalf("select (before re-upsert): %v", err)
	}
	if v != 1 {
		t.Fatalf("duration_verified = %d before re-upsert; want 1", v)
	}

	// Re-upsert the same track.
	if err := w.UpsertBatch([]library.Track{track}); err != nil {
		t.Fatalf("second UpsertBatch: %v", err)
	}

	if err := d.QueryRow(`SELECT duration_verified FROM tracks WHERE hash='hash-dv'`).Scan(&v); err != nil {
		t.Fatalf("select (after re-upsert): %v", err)
	}
	if v != 0 {
		t.Errorf("duration_verified = %d after re-upsert; want 0 (reset for re-audit)", v)
	}
}
