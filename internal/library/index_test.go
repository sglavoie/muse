package library

import (
	"fmt"
	"runtime"
	"testing"
)

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
