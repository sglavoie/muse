package db_test

import (
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
)

func TestReadTracks_Empty(t *testing.T) {
	d := openMigratedDB(t)

	rows, err := d.ReadTracks()
	if err != nil {
		t.Fatalf("ReadTracks: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("got %d rows, want 0", len(rows))
	}
}

func TestReadTracks_ReturnsUpsertedTracks(t *testing.T) {
	d := openMigratedDB(t)
	w := db.NewWriter(t.Context(), d.DB)

	now := time.Now().Unix()
	tracks := []library.Track{
		{
			Hash:      "hash1",
			Path:      "/music/artist1/track1.flac",
			Title:     "Track One",
			Artist:    "Artist One",
			Album:     "Album One",
			Duration:  180.5,
			ModTime:   now,
			SizeBytes: 4096,
		},
		{
			Hash:      "hash2",
			Path:      "/music/artist2/track2.mp3",
			Title:     "Track Two",
			Artist:    "Artist Two",
			Album:     "Album Two",
			Duration:  240.0,
			ModTime:   now - 60,
			SizeBytes: 8192,
		},
	}
	if err := w.UpsertBatch(tracks); err != nil {
		t.Fatalf("UpsertBatch: %v", err)
	}

	rows, err := d.ReadTracks()
	if err != nil {
		t.Fatalf("ReadTracks: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}

	byHash := make(map[string]db.TrackRow, len(rows))
	for _, r := range rows {
		byHash[r.Hash] = r
	}

	r1 := byHash["hash1"]
	if r1.Path != "/music/artist1/track1.flac" {
		t.Errorf("path: got %q, want %q", r1.Path, "/music/artist1/track1.flac")
	}
	if r1.Title != "Track One" {
		t.Errorf("title: got %q, want %q", r1.Title, "Track One")
	}
	if r1.Artist != "Artist One" {
		t.Errorf("artist: got %q, want %q", r1.Artist, "Artist One")
	}
	if r1.DurationMs != 180500 {
		t.Errorf("duration_ms: got %d, want 180500", r1.DurationMs)
	}
	if r1.SizeBytes != 4096 {
		t.Errorf("size_bytes: got %d, want 4096", r1.SizeBytes)
	}
	if r1.ModTime != now {
		t.Errorf("mod_time: got %d, want %d", r1.ModTime, now)
	}
}

func TestReadTracks_ExcludesMissing(t *testing.T) {
	d := openMigratedDB(t)
	w := db.NewWriter(t.Context(), d.DB)

	tracks := []library.Track{
		{Hash: "hashA", Path: "/music/a.flac", Title: "A", Artist: "Artist", Duration: 1},
		{Hash: "hashB", Path: "/music/b.flac", Title: "B", Artist: "Artist", Duration: 1},
	}
	if err := w.UpsertBatch(tracks); err != nil {
		t.Fatalf("UpsertBatch: %v", err)
	}
	if err := w.MarkTrackMissing("hashA"); err != nil {
		t.Fatalf("MarkTrackMissing: %v", err)
	}

	rows, err := d.ReadTracks()
	if err != nil {
		t.Fatalf("ReadTracks: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1 (missing track should be excluded)", len(rows))
	}
	if rows[0].Hash != "hashB" {
		t.Errorf("got hash %q, want %q", rows[0].Hash, "hashB")
	}
}
