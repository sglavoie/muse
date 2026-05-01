package db_test

import (
	"testing"
	"time"
)

func TestReadDeepCutsMix_Empty(t *testing.T) {
	d := openMigratedDB(t)

	tracks, err := d.ReadDeepCutsMix(1, 5, 0.8, 10)
	if err != nil {
		t.Fatalf("ReadDeepCutsMix: %v", err)
	}
	if len(tracks) != 0 {
		t.Errorf("ReadDeepCutsMix = %d tracks, want 0", len(tracks))
	}
}

func TestReadDeepCutsMix_HighCompletionRate(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertFullTrack(t, d, "h1", "/a.flac", "Song1", "Artist", "Album", "Rock", 180000, 5000000, now)
	w := newWriter(t, d)

	for i := range 2 {
		if err := w.RecordPlay("h1", now+int64(i), 162, 180); err != nil {
			t.Fatalf("RecordPlay: %v", err)
		}
	}
	if _, err := d.Exec(`UPDATE tracks SET play_count = 2 WHERE hash = ?`, "h1"); err != nil {
		t.Fatalf("update play_count: %v", err)
	}

	tracks, err := d.ReadDeepCutsMix(1, 5, 0.8, 10)
	if err != nil {
		t.Fatalf("ReadDeepCutsMix: %v", err)
	}
	if len(tracks) != 1 || tracks[0].Hash != "h1" {
		t.Errorf("ReadDeepCutsMix = %v, want [h1]", trackHashes(tracks))
	}
}

func TestReadDeepCutsMix_LowCompletionRateExcluded(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertFullTrack(t, d, "h1", "/a.flac", "Song1", "Artist", "Album", "Rock", 180000, 5000000, now)
	w := newWriter(t, d)

	for i := range 2 {
		if err := w.RecordPlay("h1", now+int64(i), 90, 180); err != nil {
			t.Fatalf("RecordPlay: %v", err)
		}
	}
	if _, err := d.Exec(`UPDATE tracks SET play_count = 2 WHERE hash = ?`, "h1"); err != nil {
		t.Fatalf("update play_count: %v", err)
	}

	tracks, err := d.ReadDeepCutsMix(1, 5, 0.8, 10)
	if err != nil {
		t.Fatalf("ReadDeepCutsMix: %v", err)
	}
	if len(tracks) != 0 {
		t.Errorf("ReadDeepCutsMix = %d tracks, want 0 (low completion rate)", len(tracks))
	}
}

func TestReadDeepCutsMix_ExcludesHighPlayCount(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertFullTrack(t, d, "h1", "/a.flac", "Song1", "Artist", "Album", "Rock", 180000, 5000000, now)
	w := newWriter(t, d)

	if err := w.RecordPlay("h1", now, 162, 180); err != nil {
		t.Fatalf("RecordPlay: %v", err)
	}
	if _, err := d.Exec(`UPDATE tracks SET play_count = 6 WHERE hash = ?`, "h1"); err != nil {
		t.Fatalf("update play_count: %v", err)
	}

	tracks, err := d.ReadDeepCutsMix(1, 5, 0.8, 10)
	if err != nil {
		t.Fatalf("ReadDeepCutsMix: %v", err)
	}
	if len(tracks) != 0 {
		t.Errorf("ReadDeepCutsMix = %d tracks, want 0 (play_count > 5)", len(tracks))
	}
}

func TestReadDeepCutsMix_ExcludesMissingTracks(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertFullTrack(t, d, "h1", "/a.flac", "Song1", "Artist", "Album", "Rock", 180000, 5000000, now)
	w := newWriter(t, d)

	for i := range 2 {
		if err := w.RecordPlay("h1", now+int64(i), 162, 180); err != nil {
			t.Fatalf("RecordPlay: %v", err)
		}
	}
	if _, err := d.Exec(`UPDATE tracks SET play_count = 2 WHERE hash = ?`, "h1"); err != nil {
		t.Fatalf("update play_count: %v", err)
	}

	tracks, err := d.ReadDeepCutsMix(1, 5, 0.8, 10)
	if err != nil {
		t.Fatalf("ReadDeepCutsMix: %v", err)
	}
	if len(tracks) != 1 {
		t.Fatalf("precondition: expected 1 qualifying track, got %d", len(tracks))
	}

	markMissing(t, d, "h1")

	tracks, err = d.ReadDeepCutsMix(1, 5, 0.8, 10)
	if err != nil {
		t.Fatalf("ReadDeepCutsMix after missing: %v", err)
	}
	if len(tracks) != 0 {
		t.Errorf("ReadDeepCutsMix returned %d tracks, want 0 (track is missing)", len(tracks))
	}
}
