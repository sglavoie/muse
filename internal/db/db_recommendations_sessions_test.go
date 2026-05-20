package db_test

import (
	"testing"
	"time"
)

func TestReadSessionCoOccurrences_Empty(t *testing.T) {
	d := openMigratedDB(t)

	results, err := d.ReadSessionCoOccurrences("nonexistent", 10)
	if err != nil {
		t.Fatalf("ReadSessionCoOccurrences: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("ReadSessionCoOccurrences = %v, want empty", results)
	}
}

func TestReadSessionCoOccurrences_SelfExclusion(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertFullTrack(t, d, "seed", "/seed.flac", "Seed", "Artist", "Album", "Rock", 180000, 5000000, now)
	w := newWriter(t, d)

	if err := w.RecordPlay("seed", now, 60, 180); err != nil {
		t.Fatalf("RecordPlay seed: %v", err)
	}

	results, err := d.ReadSessionCoOccurrences("seed", 10)
	if err != nil {
		t.Fatalf("ReadSessionCoOccurrences: %v", err)
	}
	for _, r := range results {
		if r.Hash == "seed" {
			t.Error("ReadSessionCoOccurrences returned seed hash, want self-excluded")
		}
	}
}

func TestReadSessionCoOccurrences_SessionBoundary(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertFullTrack(t, d, "seed", "/seed.flac", "Seed", "Artist", "Album", "Rock", 180000, 5000000, now)
	insertFullTrack(t, d, "same", "/same.flac", "Same", "Artist", "Album", "Rock", 180000, 5000000, now)
	insertFullTrack(t, d, "other", "/other.flac", "Other", "Artist", "Album", "Rock", 180000, 5000000, now)
	w := newWriter(t, d)

	if err := w.RecordPlay("seed", now, 60, 180); err != nil {
		t.Fatalf("RecordPlay seed: %v", err)
	}
	if err := w.RecordPlay("same", now+60, 60, 180); err != nil {
		t.Fatalf("RecordPlay same: %v", err)
	}
	if err := w.RecordPlay("other", now+2000, 60, 180); err != nil {
		t.Fatalf("RecordPlay other: %v", err)
	}

	results, err := d.ReadSessionCoOccurrences("seed", 10)
	if err != nil {
		t.Fatalf("ReadSessionCoOccurrences: %v", err)
	}
	found := make(map[string]bool)
	for _, r := range results {
		found[r.Hash] = true
	}
	if !found["same"] {
		t.Error("ReadSessionCoOccurrences: expected 'same' in results (same session as seed)")
	}
	if found["other"] {
		t.Error("ReadSessionCoOccurrences: 'other' should not be in results (different session)")
	}
}

func TestReadSessionCoOccurrences_CountOrdering(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertFullTrack(t, d, "seed", "/seed.flac", "Seed", "Artist", "Album", "Rock", 180000, 5000000, now)
	insertFullTrack(t, d, "frequent", "/frequent.flac", "Freq", "Artist", "Album", "Rock", 180000, 5000000, now)
	insertFullTrack(t, d, "rare", "/rare.flac", "Rare", "Artist", "Album", "Rock", 180000, 5000000, now)
	w := newWriter(t, d)

	if err := w.RecordPlay("seed", now, 60, 180); err != nil {
		t.Fatalf("RecordPlay seed 1: %v", err)
	}
	if err := w.RecordPlay("frequent", now+10, 60, 180); err != nil {
		t.Fatalf("RecordPlay frequent 1: %v", err)
	}
	if err := w.RecordPlay("rare", now+20, 60, 180); err != nil {
		t.Fatalf("RecordPlay rare: %v", err)
	}
	if err := w.RecordPlay("seed", now+2000, 60, 180); err != nil {
		t.Fatalf("RecordPlay seed 2: %v", err)
	}
	if err := w.RecordPlay("frequent", now+2010, 60, 180); err != nil {
		t.Fatalf("RecordPlay frequent 2: %v", err)
	}

	results, err := d.ReadSessionCoOccurrences("seed", 10)
	if err != nil {
		t.Fatalf("ReadSessionCoOccurrences: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("ReadSessionCoOccurrences returned empty results")
	}
	if results[0].Hash != "frequent" {
		t.Errorf("results[0].Hash = %q, want frequent (higher co-occurrence count)", results[0].Hash)
	}
}

func TestReadSessionCoOccurrences_RepeatedSeedCountsSharedSessionOnce(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertFullTrack(t, d, "seed", "/seed.flac", "Seed", "Artist", "Album", "Rock", 180000, 5000000, now)
	insertFullTrack(t, d, "mate", "/mate.flac", "Mate", "Artist", "Album", "Rock", 180000, 5000000, now)
	w := newWriter(t, d)

	if err := w.RecordPlay("seed", now, 60, 180); err != nil {
		t.Fatalf("RecordPlay seed 1: %v", err)
	}
	if err := w.RecordPlay("seed", now+10, 60, 180); err != nil {
		t.Fatalf("RecordPlay seed 2: %v", err)
	}
	if err := w.RecordPlay("mate", now+20, 60, 180); err != nil {
		t.Fatalf("RecordPlay mate: %v", err)
	}

	results, err := d.ReadSessionCoOccurrences("seed", 10)
	if err != nil {
		t.Fatalf("ReadSessionCoOccurrences: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Hash != "mate" || results[0].Count != 1 {
		t.Fatalf("got %+v, want mate count=1", results[0])
	}
}

func TestReadSessionCoOccurrences_RepeatedCompanionCountsSharedSessionOnce(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()
	insertFullTrack(t, d, "seed", "/seed.flac", "Seed", "Artist", "Album", "Rock", 180000, 5000000, now)
	insertFullTrack(t, d, "mate", "/mate.flac", "Mate", "Artist", "Album", "Rock", 180000, 5000000, now)
	w := newWriter(t, d)

	if err := w.RecordPlay("seed", now, 60, 180); err != nil {
		t.Fatalf("RecordPlay seed: %v", err)
	}
	if err := w.RecordPlay("mate", now+10, 60, 180); err != nil {
		t.Fatalf("RecordPlay mate 1: %v", err)
	}
	if err := w.RecordPlay("mate", now+20, 60, 180); err != nil {
		t.Fatalf("RecordPlay mate 2: %v", err)
	}

	results, err := d.ReadSessionCoOccurrences("seed", 10)
	if err != nil {
		t.Fatalf("ReadSessionCoOccurrences: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Hash != "mate" || results[0].Count != 1 {
		t.Fatalf("got %+v, want mate count=1", results[0])
	}
}
