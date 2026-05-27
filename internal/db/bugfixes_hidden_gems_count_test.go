package db_test

import (
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/db"
)

// TestReadMixCounts_HiddenGemsExcludesZeroPlayTracks verifies that the
// "Hidden Gems" mix count reported by ReadMixCounts matches the actual mix
// produced by ReadHiddenGemsMix, which uses play_count BETWEEN 1 AND max
// (i.e. excludes never-played tracks). Before the fix the count query used
// play_count <= max so it included every never-played track, producing a
// user-visible mismatch where the displayed count was an order of magnitude
// higher than the number of tracks actually surfaced by the mix.
func TestReadMixCounts_HiddenGemsExcludesZeroPlayTracks(t *testing.T) {
	d := openMigratedDB(t)
	now := time.Now().Unix()

	insertFullTrack(t, d, "seed", "/seed.flac", "Seed", "ArtistA", "Album", "Rock", 180000, 5000000, now)
	w := newWriter(t, d)
	if err := w.RecordPlay("seed", now, 60, 180); err != nil {
		t.Fatalf("RecordPlay seed: %v", err)
	}
	if _, err := d.Exec(`UPDATE tracks SET play_count = 3 WHERE hash = 'seed'`); err != nil {
		t.Fatalf("update seed play_count: %v", err)
	}

	insertFullTrack(t, d, "low1", "/low1.flac", "Low1", "ArtistA", "Album", "Rock", 180000, 5000000, now)
	if _, err := d.Exec(`UPDATE tracks SET play_count = 1 WHERE hash = 'low1'`); err != nil {
		t.Fatalf("update low1 play_count: %v", err)
	}

	insertFullTrack(t, d, "zero1", "/zero1.flac", "Zero1", "ArtistA", "Album", "Rock", 180000, 5000000, now)
	insertFullTrack(t, d, "zero2", "/zero2.flac", "Zero2", "ArtistA", "Album", "Rock", 180000, 5000000, now)

	mix, err := d.ReadHiddenGemsMix(2, 20, 10, 100)
	if err != nil {
		t.Fatalf("ReadHiddenGemsMix: %v", err)
	}
	wantCount := 0
	for _, tr := range mix {
		if tr.Hash == "low1" || tr.Hash == "zero1" || tr.Hash == "zero2" {
			wantCount++
		}
	}
	if wantCount != 1 {
		t.Fatalf("precondition: ReadHiddenGemsMix returned %d candidate tracks; want exactly 1 (only low1)", wantCount)
	}

	counts, err := d.ReadMixCounts(db.DefaultMixCountConfig())
	if err != nil {
		t.Fatalf("ReadMixCounts: %v", err)
	}
	got := mixCountByName(t, counts, "Hidden Gems")
	if got != 1 {
		t.Errorf("Hidden Gems count = %d, want 1 (must match ReadHiddenGemsMix which excludes 0-play tracks)", got)
	}
}
