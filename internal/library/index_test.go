package library

import (
	"fmt"
	"runtime"
	"testing"
	"time"
)

// TestIndex is the entry point matched by -run TestIndex; subtests cover all
// index behaviours.
func TestIndex(t *testing.T) {
	t.Run("IncrementalInsertion", testIndexIncrementalInsertion)
	t.Run("ArtistThenTitleSortStability", testIndexArtistThenTitleSortStability)
	t.Run("UnknownArtistBottomSort", testIndexUnknownArtistBottomSort)
}

func testIndexIncrementalInsertion(t *testing.T) {
	var idx LibraryIndex
	toInsert := []Track{
		{Artist: "Z", Title: "z1"},
		{Artist: "A", Title: "a1"},
		{Artist: "M", Title: "m1"},
	}
	for i, tr := range toInsert {
		idx.Insert(tr)
		if idx.Len() != i+1 {
			t.Fatalf("after %d inserts: got Len()=%d, want %d", i+1, idx.Len(), i+1)
		}
	}
	// Tracks must be in sorted order after each incremental insert.
	got := idx.Tracks()
	if got[0].Artist != "A" || got[1].Artist != "M" || got[2].Artist != "Z" {
		t.Errorf("unexpected order after incremental inserts: %v", got)
	}
}

func testIndexArtistThenTitleSortStability(t *testing.T) {
	var idx LibraryIndex
	tracks := []Track{
		{Artist: "Aphex Twin", Title: "Xtal"},
		{Artist: "Aphex Twin", Title: "Actium"},
		{Artist: "Aphex Twin", Title: "Flim"},
		{Artist: "Miles Davis", Title: "So What"},
		{Artist: "Zappa", Title: "Bobby Brown"},
	}
	for _, tr := range tracks {
		idx.Insert(tr)
	}
	want := []Track{
		{Artist: "Aphex Twin", Title: "Actium"},
		{Artist: "Aphex Twin", Title: "Flim"},
		{Artist: "Aphex Twin", Title: "Xtal"},
		{Artist: "Miles Davis", Title: "So What"},
		{Artist: "Zappa", Title: "Bobby Brown"},
	}
	got := idx.Tracks()
	if len(got) != len(want) {
		t.Fatalf("got %d tracks, want %d", len(got), len(want))
	}
	for i, w := range want {
		if got[i].Artist != w.Artist || got[i].Title != w.Title {
			t.Errorf("position %d: got {%q, %q}, want {%q, %q}",
				i, got[i].Artist, got[i].Title, w.Artist, w.Title)
		}
	}
}

func testIndexUnknownArtistBottomSort(t *testing.T) {
	var idx LibraryIndex
	idx.Insert(Track{Artist: unknownArtist, Title: "zzz"})
	idx.Insert(Track{Artist: "Arca", Title: "Piel"})
	idx.Insert(Track{Artist: unknownArtist, Title: "aaa"})
	idx.Insert(Track{Artist: "Boards of Canada", Title: "Roygbiv"})

	got := idx.Tracks()
	// All named artists must precede Unknown Artist entries.
	namedDone := false
	for _, tr := range got {
		if tr.Artist == unknownArtist {
			namedDone = true
		} else if namedDone {
			t.Errorf("named artist %q appeared after Unknown Artist block", tr.Artist)
		}
	}
	// Unknown Artist entries must themselves be title-sorted.
	if got[2].Title != "aaa" || got[3].Title != "zzz" {
		t.Errorf("Unknown Artist tracks not title-sorted: %v", got[2:])
	}
}

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

func TestLibraryIndex_InsertSortedOrder(t *testing.T) {
	var idx LibraryIndex

	tracks := []Track{
		{Artist: "Zappa", Title: "Bobby Brown"},
		{Artist: "Aphex Twin", Title: "Xtal"},
		{Artist: "Aphex Twin", Title: "Actium"},
		{Artist: "Miles Davis", Title: "So What"},
		{Artist: "Aphex Twin", Title: "Flim"},
	}
	for _, tr := range tracks {
		idx.Insert(tr)
	}

	got := idx.Tracks()
	if len(got) != len(tracks) {
		t.Fatalf("expected %d tracks, got %d", len(tracks), len(got))
	}

	want := []Track{
		{Artist: "Aphex Twin", Title: "Actium"},
		{Artist: "Aphex Twin", Title: "Flim"},
		{Artist: "Aphex Twin", Title: "Xtal"},
		{Artist: "Miles Davis", Title: "So What"},
		{Artist: "Zappa", Title: "Bobby Brown"},
	}
	for i, w := range want {
		if got[i].Artist != w.Artist || got[i].Title != w.Title {
			t.Errorf("position %d: got {%q, %q}, want {%q, %q}",
				i, got[i].Artist, got[i].Title, w.Artist, w.Title)
		}
	}
}

func TestLibraryIndex_UnknownArtistSortsLast(t *testing.T) {
	var idx LibraryIndex

	idx.Insert(Track{Artist: unknownArtist, Title: "some-file"})
	idx.Insert(Track{Artist: "Boards of Canada", Title: "Roygbiv"})
	idx.Insert(Track{Artist: unknownArtist, Title: "another-file"})
	idx.Insert(Track{Artist: "Arca", Title: "Piel"})

	got := idx.Tracks()
	// All named artists must come before Unknown Artist entries.
	namedDone := false
	for _, tr := range got {
		if tr.Artist == unknownArtist {
			namedDone = true
		} else if namedDone {
			t.Errorf("named artist %q appeared after Unknown Artist block", tr.Artist)
		}
	}
	// Verify Unknown Artist entries are themselves title-sorted.
	if got[2].Title != "another-file" || got[3].Title != "some-file" {
		t.Errorf("Unknown Artist tracks not title-sorted: %v", got[2:])
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
