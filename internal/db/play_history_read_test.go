package db_test

import (
	"testing"
	"time"
)

// --- ReadPlayHistory ---

func TestReadPlayHistory_FieldMapping(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertFullTrack(t, d, "h1", "/a.flac", "Title One", "Artist One", "Album One", "rock", 180000, 5000000, now)
	// Set track_number and disc_number directly so we can verify their mapping.
	if _, err := d.Exec(`UPDATE tracks SET track_number = 3, disc_number = 2 WHERE hash = ?`, "h1"); err != nil {
		t.Fatalf("update track metadata: %v", err)
	}
	w := newWriter(t, d)

	playedAt := now + 100
	if err := w.RecordPlay("h1", playedAt, 60, 180.0); err != nil {
		t.Fatalf("RecordPlay: %v", err)
	}

	tracks, err := d.ReadPlayHistory(10)
	if err != nil {
		t.Fatalf("ReadPlayHistory: %v", err)
	}
	if len(tracks) != 1 {
		t.Fatalf("got %d tracks, want 1", len(tracks))
	}
	tr := tracks[0]
	if tr.Hash != "h1" {
		t.Errorf("Hash = %q, want %q", tr.Hash, "h1")
	}
	if tr.LastPlayedAt != playedAt {
		t.Errorf("LastPlayedAt = %d, want %d (ph.played_at)", tr.LastPlayedAt, playedAt)
	}
	if tr.TrackNumber != 3 {
		t.Errorf("TrackNumber = %d, want 3", tr.TrackNumber)
	}
	if tr.DiscNumber != 2 {
		t.Errorf("DiscNumber = %d, want 2", tr.DiscNumber)
	}
	if tr.Duration != 180.0 {
		t.Errorf("Duration = %f, want 180.0", tr.Duration)
	}
}

func TestReadPlayHistory_OrderedByPlayedAtDesc(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertTrack(t, d, "h1", "/a.flac")
	insertTrack(t, d, "h2", "/b.flac")
	w := newWriter(t, d)

	if err := w.RecordPlay("h1", now-200, 30, 0); err != nil {
		t.Fatalf("RecordPlay h1: %v", err)
	}
	if err := w.RecordPlay("h2", now-100, 30, 0); err != nil {
		t.Fatalf("RecordPlay h2: %v", err)
	}

	tracks, err := d.ReadPlayHistory(10)
	if err != nil {
		t.Fatalf("ReadPlayHistory: %v", err)
	}
	if len(tracks) != 2 {
		t.Fatalf("got %d tracks, want 2", len(tracks))
	}
	if tracks[0].Hash != "h2" || tracks[1].Hash != "h1" {
		t.Errorf("order: got [%s, %s], want [h2, h1]", tracks[0].Hash, tracks[1].Hash)
	}
}

func TestReadPlayHistory_DefaultLimit(t *testing.T) {
	d := openMigratedDB(t)
	insertTrack(t, d, "h1", "/a.flac")
	w := newWriter(t, d)

	now := time.Now().Unix()
	if err := w.RecordPlay("h1", now, 30, 0); err != nil {
		t.Fatalf("RecordPlay: %v", err)
	}

	tracks, err := d.ReadPlayHistory(0) // 0 should default to 500
	if err != nil {
		t.Fatalf("ReadPlayHistory(0): %v", err)
	}
	if len(tracks) != 1 {
		t.Errorf("got %d tracks, want 1", len(tracks))
	}
}
