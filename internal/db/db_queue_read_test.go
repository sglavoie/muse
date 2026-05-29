package db_test

import (
	"testing"
	"time"
)

func TestReadQueue_Empty(t *testing.T) {
	d := openMigratedDB(t)

	rows, err := d.ReadQueue()
	if err != nil {
		t.Fatalf("ReadQueue: %v", err)
	}
	if rows == nil {
		t.Fatalf("ReadQueue returned nil slice, want non-nil empty slice")
	}
	if len(rows) != 0 {
		t.Errorf("got %d rows, want 0", len(rows))
	}
}

func TestReadQueue_PartiallySoftDeleted(t *testing.T) {
	d := openMigratedDB(t)

	insertTrack(t, d, "hashA", "/music/a.flac")
	insertTrack(t, d, "hashB", "/music/b.flac")

	insertQueueEntry(t, d, "hashA", 1000)
	qidB := insertQueueEntry(t, d, "hashB", 2000)

	now := time.Now().Unix()
	if _, err := d.Exec(`UPDATE queue SET deleted_at = ? WHERE id = ?`, now, qidB); err != nil {
		t.Fatalf("soft-delete: %v", err)
	}

	rows, err := d.ReadQueue()
	if err != nil {
		t.Fatalf("ReadQueue: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	if rows[0].Hash != "hashA" {
		t.Errorf("got hash %q, want %q", rows[0].Hash, "hashA")
	}
}

func TestReadQueue_TrackMissingMarksQueueRowMissing(t *testing.T) {
	d := openMigratedDB(t)

	insertTrack(t, d, "hashA", "/music/a.flac")
	insertQueueEntry(t, d, "hashA", 1000)

	if _, err := d.Exec(`UPDATE tracks SET missing = 1 WHERE hash = ?`, "hashA"); err != nil {
		t.Fatalf("mark track missing: %v", err)
	}

	rows, err := d.ReadQueue()
	if err != nil {
		t.Fatalf("ReadQueue: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	if !rows[0].Missing {
		t.Fatal("ReadQueue returned Missing=false for a queued track with tracks.missing=1")
	}
}
