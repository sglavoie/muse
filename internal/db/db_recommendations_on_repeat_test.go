package db_test

import (
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/db"
)

func TestReadOnRepeatMix_Empty(t *testing.T) {
	d := openMigratedDB(t)

	tracks, err := d.ReadOnRepeatMix(3, 7, 10)
	if err != nil {
		t.Fatalf("ReadOnRepeatMix: %v", err)
	}
	if len(tracks) != 0 {
		t.Errorf("ReadOnRepeatMix = %d tracks, want 0", len(tracks))
	}
}

func TestReadOnRepeatMix_RequiresMinRecentPlays(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertFullTrack(t, d, "h1", "/a.flac", "Song1", "Artist", "Album", "Rock", 180000, 5000000, now)
	w := newWriter(t, d)

	for i := range 2 {
		if err := w.RecordListen(db.PlayRecord{Hash: "h1", PlayedAt: now - int64(i*3600), ListenedSecs: 60, DurationSecs: 180, Counted: true}); err != nil {
			t.Fatalf("RecordListen: %v", err)
		}
	}

	tracks, err := d.ReadOnRepeatMix(3, 7, 10)
	if err != nil {
		t.Fatalf("ReadOnRepeatMix: %v", err)
	}
	if len(tracks) != 0 {
		t.Errorf("ReadOnRepeatMix = %d tracks, want 0 (only 2 recent plays)", len(tracks))
	}
}

func TestReadOnRepeatMix_Returns3PlusRecentPlays(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertFullTrack(t, d, "h1", "/a.flac", "Song1", "Artist", "Album", "Rock", 180000, 5000000, now)
	w := newWriter(t, d)

	for i := range 3 {
		if err := w.RecordListen(db.PlayRecord{Hash: "h1", PlayedAt: now - int64(i*3600), ListenedSecs: 60, DurationSecs: 180, Counted: true}); err != nil {
			t.Fatalf("RecordListen: %v", err)
		}
	}

	tracks, err := d.ReadOnRepeatMix(3, 7, 10)
	if err != nil {
		t.Fatalf("ReadOnRepeatMix: %v", err)
	}
	if len(tracks) != 1 || tracks[0].Hash != "h1" {
		t.Errorf("ReadOnRepeatMix returned %v, want [h1]", trackHashes(tracks))
	}
}

func TestReadOnRepeatMix_ExcludesOldPlays(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertFullTrack(t, d, "h1", "/a.flac", "Song1", "Artist", "Album", "Rock", 180000, 5000000, now)

	eightDaysAgo := now - 8*86400
	for i := range 3 {
		if _, err := d.Exec(
			`INSERT INTO play_events (track_hash, played_at, listened_secs) VALUES (?, ?, ?)`,
			"h1", eightDaysAgo+int64(i), 60,
		); err != nil {
			t.Fatalf("insert play_events: %v", err)
		}
	}

	tracks, err := d.ReadOnRepeatMix(3, 7, 10)
	if err != nil {
		t.Fatalf("ReadOnRepeatMix: %v", err)
	}
	if len(tracks) != 0 {
		t.Errorf("ReadOnRepeatMix = %d tracks, want 0 (all plays outside window)", len(tracks))
	}
}

func TestReadOnRepeatMix_ExcludesMissingTracks(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertFullTrack(t, d, "h1", "/a.flac", "Song1", "Artist", "Album", "Rock", 180000, 5000000, now)
	w := newWriter(t, d)

	for i := range 3 {
		if err := w.RecordListen(db.PlayRecord{Hash: "h1", PlayedAt: now - int64(i*3600), ListenedSecs: 60, DurationSecs: 180, Counted: true}); err != nil {
			t.Fatalf("RecordListen: %v", err)
		}
	}

	tracks, err := d.ReadOnRepeatMix(3, 7, 10)
	if err != nil {
		t.Fatalf("ReadOnRepeatMix: %v", err)
	}
	if len(tracks) != 1 {
		t.Fatalf("precondition: expected 1 qualifying track, got %d", len(tracks))
	}

	markMissing(t, d, "h1")

	tracks, err = d.ReadOnRepeatMix(3, 7, 10)
	if err != nil {
		t.Fatalf("ReadOnRepeatMix after missing: %v", err)
	}
	if len(tracks) != 0 {
		t.Errorf("ReadOnRepeatMix returned %d tracks, want 0 (track is missing)", len(tracks))
	}
}
