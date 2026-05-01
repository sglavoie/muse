package db_test

import (
	"testing"
	"time"
)

func TestReadRediscoverMix_Empty(t *testing.T) {
	d := openMigratedDB(t)

	tracks, err := d.ReadRediscoverMix(10, 90, 10)
	if err != nil {
		t.Fatalf("ReadRediscoverMix: %v", err)
	}
	if len(tracks) != 0 {
		t.Errorf("ReadRediscoverMix = %d tracks, want 0", len(tracks))
	}
}

func TestReadRediscoverMix_ExcludesRecentPlays(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertFullTrack(t, d, "h1", "/a.flac", "Song1", "Artist", "Album", "Rock", 180000, 5000000, now)
	insertFullTrack(t, d, "h2", "/b.flac", "Song2", "Artist", "Album", "Rock", 180000, 5000000, now)

	ninetyOneDaysAgo := now - 91*86400
	if _, err := d.Exec(`UPDATE tracks SET play_count = 10, last_played_at = ? WHERE hash = ?`, ninetyOneDaysAgo, "h1"); err != nil {
		t.Fatalf("update h1: %v", err)
	}
	if _, err := d.Exec(`UPDATE tracks SET play_count = 10, last_played_at = ? WHERE hash = ?`, now, "h2"); err != nil {
		t.Fatalf("update h2: %v", err)
	}

	tracks, err := d.ReadRediscoverMix(10, 90, 10)
	if err != nil {
		t.Fatalf("ReadRediscoverMix: %v", err)
	}
	if len(tracks) != 1 || tracks[0].Hash != "h1" {
		t.Errorf("ReadRediscoverMix returned %v, want [h1]", trackHashes(tracks))
	}
}

func TestReadRediscoverMix_RequiresMinPlayCount(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertFullTrack(t, d, "h1", "/a.flac", "Song1", "Artist", "Album", "Rock", 180000, 5000000, now)

	oldEnough := now - 91*86400
	if _, err := d.Exec(`UPDATE tracks SET play_count = 9, last_played_at = ? WHERE hash = ?`, oldEnough, "h1"); err != nil {
		t.Fatalf("update h1: %v", err)
	}

	tracks, err := d.ReadRediscoverMix(10, 90, 10)
	if err != nil {
		t.Fatalf("ReadRediscoverMix: %v", err)
	}
	if len(tracks) != 0 {
		t.Errorf("ReadRediscoverMix returned %d tracks, want 0 (play_count below threshold)", len(tracks))
	}
}

func TestReadRediscoverMix_ExcludesMissingTracks(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertFullTrack(t, d, "h1", "/a.flac", "Song1", "Artist", "Album", "Rock", 180000, 5000000, now)

	ninetyOneDaysAgo := now - 91*86400
	if _, err := d.Exec(`UPDATE tracks SET play_count = 10, last_played_at = ? WHERE hash = ?`, ninetyOneDaysAgo, "h1"); err != nil {
		t.Fatalf("update h1: %v", err)
	}

	tracks, err := d.ReadRediscoverMix(10, 90, 10)
	if err != nil {
		t.Fatalf("ReadRediscoverMix: %v", err)
	}
	if len(tracks) != 1 {
		t.Fatalf("precondition: expected 1 qualifying track, got %d", len(tracks))
	}

	markMissing(t, d, "h1")

	tracks, err = d.ReadRediscoverMix(10, 90, 10)
	if err != nil {
		t.Fatalf("ReadRediscoverMix after missing: %v", err)
	}
	if len(tracks) != 0 {
		t.Errorf("ReadRediscoverMix returned %d tracks, want 0 (track is missing)", len(tracks))
	}
}
