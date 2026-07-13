package library

import (
	"fmt"
	"runtime"
	"testing"
	"time"
)

// BenchmarkIndex measures insertion of 50,000 tracks and reports per-op
// allocations. Run with -bench BenchmarkIndex.
func BenchmarkIndex(b *testing.B) {
	for b.Loop() {
		var idx LibraryIndex
		for i := range 50_000 {
			idx.Insert(Track{
				Artist: fmt.Sprintf("Artist%05d", i%1000),
				Title:  fmt.Sprintf("Track%05d", i),
				Album:  fmt.Sprintf("Album%05d", i%200),
				Hash:   fmt.Sprintf("%064x", i),
				Path:   fmt.Sprintf("/music/artist%05d/track%05d.flac", i%1000, i),
			})
		}
	}
}

func TestLibraryIndex_Sub100msLatency(t *testing.T) {
	const n = 10_001

	var idx LibraryIndex
	for i := range n {
		idx.Insert(Track{
			Artist: fmt.Sprintf("Artist%05d", i%1000),
			Title:  fmt.Sprintf("Track%05d", i),
			Album:  fmt.Sprintf("Album%05d", i%200),
			Hash:   fmt.Sprintf("%064x", i),
			Path:   fmt.Sprintf("/music/artist%05d/track%05d.flac", i%1000, i),
		})
	}

	start := time.Now()
	tracks := idx.Tracks()
	for range tracks {
	}
	elapsed := time.Since(start)

	t.Logf("Tracks() access + iteration over %d tracks: %v", n, elapsed)

	const limit = 100 * time.Millisecond
	if elapsed >= limit {
		t.Errorf("access + iteration took %v, want < %v", elapsed, limit)
	}
}

func TestLibraryIndex_MemoryUnder25MB(t *testing.T) {
	const n = 50_000

	var before runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)

	tracks := make([]Track, n)
	for i := range n {
		tracks[i] = Track{
			Artist: fmt.Sprintf("Artist%05d", i%1000),
			Title:  fmt.Sprintf("Track%05d", i),
			Album:  fmt.Sprintf("Album%05d", i%200),
			Hash:   fmt.Sprintf("%064x", i),
			Path:   fmt.Sprintf("/music/artist%05d/track%05d.flac", i%1000, i),
		}
	}

	var idx LibraryIndex
	idx.BulkLoad(tracks)
	// Drop the reference so the 50k-track input slice is collected by the GC
	// below, isolating the index's own heap footprint in the measurement. The
	// assignment is deliberately "ineffectual" — its purpose is the side effect
	// of releasing the backing array, not a value later read.
	tracks = nil //nolint:ineffassign // release input slice before measuring index heap

	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)

	peakMB := float64(after.HeapInuse-before.HeapInuse) / (1024 * 1024)
	t.Logf("heap delta for %d tracks: %.2f MB", n, peakMB)

	const limitMB = 25.0
	if peakMB > limitMB {
		t.Errorf("peak heap delta %.2f MB exceeds %.0f MB limit", peakMB, limitMB)
	}

	if idx.Len() != n {
		t.Errorf("expected %d tracks, got %d", n, idx.Len())
	}
}
