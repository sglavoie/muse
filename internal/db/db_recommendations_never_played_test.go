package db_test

import (
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/db"
)

func TestReadNeverPlayedMix_Empty(t *testing.T) {
	d := openMigratedDB(t)

	tracks, err := d.ReadNeverPlayedMix(10)
	if err != nil {
		t.Fatalf("ReadNeverPlayedMix: %v", err)
	}
	if len(tracks) != 0 {
		t.Errorf("ReadNeverPlayedMix = %d tracks, want 0", len(tracks))
	}
}

func TestReadNeverPlayedMix_ExcludesPlayedTracks(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertFullTrack(t, d, "h1", "/a.flac", "Song1", "Artist", "Album", "Rock", 180000, 5000000, now)
	w := newWriter(t, d)

	if err := w.RecordListen(db.PlayRecord{Hash: "h1", PlayedAt: now, ListenedSecs: 60, DurationSecs: 180, Counted: true}); err != nil {
		t.Fatalf("RecordListen: %v", err)
	}
	if _, err := d.Exec(`UPDATE tracks SET play_count = 1 WHERE hash = ?`, "h1"); err != nil {
		t.Fatalf("update play_count: %v", err)
	}

	tracks, err := d.ReadNeverPlayedMix(10)
	if err != nil {
		t.Fatalf("ReadNeverPlayedMix: %v", err)
	}
	if len(tracks) != 0 {
		t.Errorf("ReadNeverPlayedMix = %d tracks, want 0 (track has been played)", len(tracks))
	}
}

func TestReadNeverPlayedMix_ArtistAffinityRanking(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertFullTrack(t, d, "h1", "/a.flac", "Song1", "ArtistA", "AlbumA", "Rock", 180000, 5000000, now)
	insertFullTrack(t, d, "h2", "/b.flac", "Song2", "ArtistA", "AlbumA", "Rock", 180000, 5000000, now)
	insertFullTrack(t, d, "h3", "/c.flac", "Song3", "ArtistB", "AlbumB", "Jazz", 180000, 5000000, now)
	w := newWriter(t, d)

	if err := w.RecordListen(db.PlayRecord{Hash: "h1", PlayedAt: now, ListenedSecs: 60, DurationSecs: 180, Counted: true}); err != nil {
		t.Fatalf("RecordListen: %v", err)
	}
	if _, err := d.Exec(`UPDATE tracks SET play_count = 1 WHERE hash = ?`, "h1"); err != nil {
		t.Fatalf("update play_count: %v", err)
	}

	tracks, err := d.ReadNeverPlayedMix(10)
	if err != nil {
		t.Fatalf("ReadNeverPlayedMix: %v", err)
	}
	if len(tracks) < 2 {
		t.Fatalf("ReadNeverPlayedMix returned %d tracks, want >= 2", len(tracks))
	}
	if tracks[0].Hash != "h2" {
		t.Errorf("tracks[0].Hash = %q, want h2 (higher artist affinity)", tracks[0].Hash)
	}
}

func TestReadNeverPlayedMix_NoGenreTrack(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertFullTrack(t, d, "h1", "/a.flac", "Song1", "Artist", "Album", "", 180000, 5000000, now)

	tracks, err := d.ReadNeverPlayedMix(10)
	if err != nil {
		t.Fatalf("ReadNeverPlayedMix: %v", err)
	}
	if len(tracks) != 1 || tracks[0].Hash != "h1" {
		t.Errorf("ReadNeverPlayedMix = %v, want [h1]", trackHashes(tracks))
	}
}

func TestReadNeverPlayedMix_ExcludesMissingTracks(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertFullTrack(t, d, "h1", "/a.flac", "Song1", "Artist", "Album", "Rock", 180000, 5000000, now)

	tracks, err := d.ReadNeverPlayedMix(10)
	if err != nil {
		t.Fatalf("ReadNeverPlayedMix: %v", err)
	}
	if len(tracks) != 1 {
		t.Fatalf("precondition: expected 1 qualifying track, got %d", len(tracks))
	}

	markMissing(t, d, "h1")

	tracks, err = d.ReadNeverPlayedMix(10)
	if err != nil {
		t.Fatalf("ReadNeverPlayedMix after missing: %v", err)
	}
	if len(tracks) != 0 {
		t.Errorf("ReadNeverPlayedMix returned %d tracks, want 0 (track is missing)", len(tracks))
	}
}
