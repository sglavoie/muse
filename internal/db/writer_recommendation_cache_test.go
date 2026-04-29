package db_test

import (
	"testing"

	"github.com/sglavoie/muse/internal/db"
)

// TestRebuildRecommendationCache_RoundTrip verifies that entries written by
// RebuildRecommendationCache are returned by ReadCachedMix in score order.
func TestRebuildRecommendationCache_RoundTrip(t *testing.T) {
	d := openMigratedDB(t)
	insertTrack(t, d, "h1", "/a.flac")
	insertTrack(t, d, "h2", "/b.flac")
	insertTrack(t, d, "h3", "/c.flac")
	w := newWriter(t, d)

	entries := []db.CacheEntry{
		{MixID: int(db.MixRediscover), TrackHash: "h1", Score: 10.0},
		{MixID: int(db.MixRediscover), TrackHash: "h2", Score: 30.0},
		{MixID: int(db.MixRediscover), TrackHash: "h3", Score: 20.0},
	}
	if err := w.RebuildRecommendationCache(entries); err != nil {
		t.Fatalf("RebuildRecommendationCache: %v", err)
	}

	tracks, err := d.ReadCachedMix(int(db.MixRediscover), 100)
	if err != nil {
		t.Fatalf("ReadCachedMix: %v", err)
	}
	if len(tracks) != 3 {
		t.Fatalf("got %d tracks, want 3", len(tracks))
	}
	// Expect descending score order: h2 (30), h3 (20), h1 (10).
	wantOrder := []string{"h2", "h3", "h1"}
	for i, want := range wantOrder {
		if tracks[i].Hash != want {
			t.Errorf("tracks[%d].Hash = %q, want %q", i, tracks[i].Hash, want)
		}
	}
}

// TestRebuildRecommendationCache_Replaces verifies that a second rebuild
// completely replaces the previous cache (no stale rows remain).
func TestRebuildRecommendationCache_Replaces(t *testing.T) {
	d := openMigratedDB(t)
	insertTrack(t, d, "h1", "/a.flac")
	insertTrack(t, d, "h2", "/b.flac")
	w := newWriter(t, d)

	first := []db.CacheEntry{
		{MixID: int(db.MixRediscover), TrackHash: "h1", Score: 5.0},
		{MixID: int(db.MixRediscover), TrackHash: "h2", Score: 5.0},
	}
	if err := w.RebuildRecommendationCache(first); err != nil {
		t.Fatalf("first RebuildRecommendationCache: %v", err)
	}

	second := []db.CacheEntry{
		{MixID: int(db.MixRediscover), TrackHash: "h1", Score: 1.0},
	}
	if err := w.RebuildRecommendationCache(second); err != nil {
		t.Fatalf("second RebuildRecommendationCache: %v", err)
	}

	tracks, err := d.ReadCachedMix(int(db.MixRediscover), 100)
	if err != nil {
		t.Fatalf("ReadCachedMix: %v", err)
	}
	if len(tracks) != 1 {
		t.Errorf("got %d tracks after replace, want 1 (h2 should be gone)", len(tracks))
	}
}

// TestRebuildRecommendationCache_ExcludesMissing verifies that tracks with
// missing=1 are not returned by ReadCachedMix.
func TestRebuildRecommendationCache_ExcludesMissing(t *testing.T) {
	d := openMigratedDB(t)
	insertTrack(t, d, "h1", "/a.flac")
	insertTrack(t, d, "h2", "/b.flac")
	// Mark h2 as missing.
	if _, err := d.Exec(`UPDATE tracks SET missing=1 WHERE hash='h2'`); err != nil {
		t.Fatalf("mark missing: %v", err)
	}
	w := newWriter(t, d)

	entries := []db.CacheEntry{
		{MixID: int(db.MixHiddenGems), TrackHash: "h1", Score: 10.0},
		{MixID: int(db.MixHiddenGems), TrackHash: "h2", Score: 20.0},
	}
	if err := w.RebuildRecommendationCache(entries); err != nil {
		t.Fatalf("RebuildRecommendationCache: %v", err)
	}

	tracks, err := d.ReadCachedMix(int(db.MixHiddenGems), 100)
	if err != nil {
		t.Fatalf("ReadCachedMix: %v", err)
	}
	if len(tracks) != 1 {
		t.Fatalf("got %d tracks, want 1 (missing track excluded)", len(tracks))
	}
	if tracks[0].Hash != "h1" {
		t.Errorf("got hash %q, want h1", tracks[0].Hash)
	}
}

// TestReadCachedMix_EmptyCache verifies that ReadCachedMix returns an empty
// non-nil slice (no error) when no cache rows exist for the mix.
func TestReadCachedMix_EmptyCache(t *testing.T) {
	d := openMigratedDB(t)

	tracks, err := d.ReadCachedMix(int(db.MixRediscover), 100)
	if err != nil {
		t.Fatalf("ReadCachedMix on empty cache: %v", err)
	}
	if tracks == nil {
		t.Error("ReadCachedMix returned nil slice, want empty non-nil slice")
	}
	if len(tracks) != 0 {
		t.Errorf("got %d tracks, want 0", len(tracks))
	}
}

// TestRebuildRecommendationCache_EmptyEntries verifies that rebuilding with
// no entries clears the cache without error.
func TestRebuildRecommendationCache_EmptyEntries(t *testing.T) {
	d := openMigratedDB(t)
	insertTrack(t, d, "h1", "/a.flac")
	w := newWriter(t, d)

	// Populate then clear.
	if err := w.RebuildRecommendationCache([]db.CacheEntry{
		{MixID: int(db.MixRediscover), TrackHash: "h1", Score: 1.0},
	}); err != nil {
		t.Fatalf("initial RebuildRecommendationCache: %v", err)
	}
	if err := w.RebuildRecommendationCache(nil); err != nil {
		t.Fatalf("RebuildRecommendationCache(nil): %v", err)
	}

	tracks, err := d.ReadCachedMix(int(db.MixRediscover), 100)
	if err != nil {
		t.Fatalf("ReadCachedMix: %v", err)
	}
	if len(tracks) != 0 {
		t.Errorf("got %d tracks after clear, want 0", len(tracks))
	}
}
