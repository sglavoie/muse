package db_test

import (
	"testing"
	"time"
)

func TestReadHiddenGemsMix_NoHistory(t *testing.T) {
	d := openMigratedDB(t)

	tracks, err := d.ReadHiddenGemsMix(2, 20, 10, 10)
	if err != nil {
		t.Fatalf("ReadHiddenGemsMix: %v", err)
	}
	if len(tracks) != 0 {
		t.Errorf("ReadHiddenGemsMix = %d tracks, want 0 (no history)", len(tracks))
	}
}

func TestReadHiddenGemsMix_MatchesByArtist(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertFullTrack(t, d, "h1", "/a.flac", "Song1", "ArtistA", "Album", "Rock", 180000, 5000000, now)
	insertFullTrack(t, d, "h2", "/b.flac", "Song2", "ArtistA", "Album", "Rock", 180000, 5000000, now)
	insertFullTrack(t, d, "h3", "/c.flac", "Song3", "ArtistZ", "Album", "Jazz", 180000, 5000000, now)
	w := newWriter(t, d)

	if err := w.RecordPlay("h1", now, 60, 180); err != nil {
		t.Fatalf("RecordPlay: %v", err)
	}
	if _, err := d.Exec(`UPDATE tracks SET play_count = 3 WHERE hash = ?`, "h1"); err != nil {
		t.Fatalf("update h1 play_count: %v", err)
	}

	tracks, err := d.ReadHiddenGemsMix(2, 20, 10, 10)
	if err != nil {
		t.Fatalf("ReadHiddenGemsMix: %v", err)
	}
	foundH2 := false
	for _, tr := range tracks {
		if tr.Hash == "h2" {
			foundH2 = true
		}
		if tr.Hash == "h3" {
			t.Errorf("ReadHiddenGemsMix returned h3 (no affinity), want excluded")
		}
	}
	if !foundH2 {
		t.Errorf("ReadHiddenGemsMix did not return h2 (matching artist)")
	}
}

func TestReadHiddenGemsMix_MatchesByGenre(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertFullTrack(t, d, "h1", "/a.flac", "Song1", "ArtistA", "Album", "Rock", 180000, 5000000, now)
	insertFullTrack(t, d, "h2", "/b.flac", "Song2", "ArtistZ", "Album", "Rock", 180000, 5000000, now)
	w := newWriter(t, d)

	if err := w.RecordPlay("h1", now, 60, 180); err != nil {
		t.Fatalf("RecordPlay h1: %v", err)
	}
	if _, err := d.Exec(`UPDATE tracks SET play_count = 3 WHERE hash = ?`, "h1"); err != nil {
		t.Fatalf("update h1 play_count: %v", err)
	}
	if _, err := d.Exec(`UPDATE tracks SET play_count = 1 WHERE hash = ?`, "h2"); err != nil {
		t.Fatalf("update h2 play_count: %v", err)
	}
	if _, err := d.Exec(`INSERT OR IGNORE INTO track_genres (track_hash, genre) VALUES (?, ?)`, "h1", "Rock"); err != nil {
		t.Fatalf("insert track_genres h1: %v", err)
	}
	if _, err := d.Exec(`INSERT OR IGNORE INTO track_genres (track_hash, genre) VALUES (?, ?)`, "h2", "Rock"); err != nil {
		t.Fatalf("insert track_genres h2: %v", err)
	}

	tracks, err := d.ReadHiddenGemsMix(2, 20, 10, 10)
	if err != nil {
		t.Fatalf("ReadHiddenGemsMix: %v", err)
	}
	found := false
	for _, tr := range tracks {
		if tr.Hash == "h2" {
			found = true
		}
	}
	if !found {
		t.Errorf("ReadHiddenGemsMix did not return h2 (genre match via track_genres)")
	}
}

func TestReadHiddenGemsMix_ExcludesHighPlayCount(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertFullTrack(t, d, "h1", "/a.flac", "Song1", "ArtistA", "Album", "Rock", 180000, 5000000, now)
	insertFullTrack(t, d, "h2", "/b.flac", "Song2", "ArtistA", "Album", "Rock", 180000, 5000000, now)
	w := newWriter(t, d)

	if err := w.RecordPlay("h1", now, 60, 180); err != nil {
		t.Fatalf("RecordPlay: %v", err)
	}
	if _, err := d.Exec(`UPDATE tracks SET play_count = 5 WHERE hash = ?`, "h1"); err != nil {
		t.Fatalf("update h1: %v", err)
	}
	if _, err := d.Exec(`UPDATE tracks SET play_count = 3 WHERE hash = ?`, "h2"); err != nil {
		t.Fatalf("update h2: %v", err)
	}

	tracks, err := d.ReadHiddenGemsMix(2, 20, 10, 10)
	if err != nil {
		t.Fatalf("ReadHiddenGemsMix: %v", err)
	}
	for _, tr := range tracks {
		if tr.Hash == "h2" {
			t.Errorf("ReadHiddenGemsMix returned h2 (play_count=3 > 2), want excluded")
		}
	}
}

func TestReadHiddenGemsMix_MultiGenreTrack(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertFullTrack(t, d, "h1", "/a.flac", "Song1", "ArtistA", "Album", "Rock", 180000, 5000000, now)
	insertFullTrack(t, d, "h2", "/b.flac", "Song2", "ArtistZ", "Album", "Rock; Jazz", 180000, 5000000, now)
	w := newWriter(t, d)

	if err := w.RecordPlay("h1", now, 60, 180); err != nil {
		t.Fatalf("RecordPlay: %v", err)
	}
	if _, err := d.Exec(`UPDATE tracks SET play_count = 3 WHERE hash = ?`, "h1"); err != nil {
		t.Fatalf("update h1 play_count: %v", err)
	}
	if _, err := d.Exec(`INSERT OR IGNORE INTO track_genres (track_hash, genre) VALUES (?, ?)`, "h1", "Rock"); err != nil {
		t.Fatalf("insert track_genres h1: %v", err)
	}
	if _, err := d.Exec(`INSERT OR IGNORE INTO track_genres (track_hash, genre) VALUES (?, ?)`, "h2", "Rock"); err != nil {
		t.Fatalf("insert track_genres h2 Rock: %v", err)
	}
	if _, err := d.Exec(`INSERT OR IGNORE INTO track_genres (track_hash, genre) VALUES (?, ?)`, "h2", "Jazz"); err != nil {
		t.Fatalf("insert track_genres h2 Jazz: %v", err)
	}

	tracks, err := d.ReadHiddenGemsMix(2, 20, 10, 10)
	if err != nil {
		t.Fatalf("ReadHiddenGemsMix: %v", err)
	}
	found := false
	for _, tr := range tracks {
		if tr.Hash == "h2" {
			found = true
		}
	}
	if !found {
		t.Errorf("ReadHiddenGemsMix did not return multi-genre track h2")
	}
}

func TestReadHiddenGemsMix_ExcludesMissingTracks(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertFullTrack(t, d, "h1", "/a.flac", "Song1", "ArtistA", "Album", "Rock", 180000, 5000000, now)
	insertFullTrack(t, d, "h2", "/b.flac", "Song2", "ArtistA", "Album", "Rock", 180000, 5000000, now)
	w := newWriter(t, d)

	if err := w.RecordPlay("h1", now, 60, 180); err != nil {
		t.Fatalf("RecordPlay: %v", err)
	}
	if _, err := d.Exec(`UPDATE tracks SET play_count = 3 WHERE hash = ?`, "h1"); err != nil {
		t.Fatalf("update h1: %v", err)
	}

	tracks, err := d.ReadHiddenGemsMix(2, 20, 10, 10)
	if err != nil {
		t.Fatalf("ReadHiddenGemsMix: %v", err)
	}
	foundH2 := false
	for _, tr := range tracks {
		if tr.Hash == "h2" {
			foundH2 = true
		}
	}
	if !foundH2 {
		t.Fatalf("precondition: expected h2 in results")
	}

	markMissing(t, d, "h2")

	tracks, err = d.ReadHiddenGemsMix(2, 20, 10, 10)
	if err != nil {
		t.Fatalf("ReadHiddenGemsMix after missing: %v", err)
	}
	for _, tr := range tracks {
		if tr.Hash == "h2" {
			t.Errorf("ReadHiddenGemsMix returned h2, want excluded (track is missing)")
		}
	}
}
