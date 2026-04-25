package tui

import (
	"testing"

	"github.com/sglavoie/muse/internal/library"
)

func TestSortTracks(t *testing.T) {
	tracks := []library.Track{
		{Artist: "Charlie", Title: "Zulu", Album: "Red", Duration: 300},
		{Artist: "Alpha", Title: "Bravo", Album: "Blue", Duration: 100},
		{Artist: "", Title: "Unknown", Album: "", Duration: 200},
		{Artist: "Bravo", Title: "Alpha", Album: "Green", Duration: 150},
	}

	t.Run("sort by artist asc", func(t *testing.T) {
		sorted := SortTracks(tracks, SortArtist, SortAsc)
		if len(sorted) != 4 {
			t.Fatalf("expected 4 tracks, got %d", len(sorted))
		}
		if sorted[0].Artist != "Alpha" {
			t.Errorf("first should be Alpha, got %q", sorted[0].Artist)
		}
		if sorted[1].Artist != "Bravo" {
			t.Errorf("second should be Bravo, got %q", sorted[1].Artist)
		}
		if sorted[2].Artist != "Charlie" {
			t.Errorf("third should be Charlie, got %q", sorted[2].Artist)
		}
		if sorted[3].Artist != "" {
			t.Errorf("last should be empty (unknown), got %q", sorted[3].Artist)
		}
	})

	t.Run("sort by artist desc", func(t *testing.T) {
		sorted := SortTracks(tracks, SortArtist, SortDesc)
		if sorted[0].Artist != "Charlie" {
			t.Errorf("first should be Charlie, got %q", sorted[0].Artist)
		}
		if sorted[1].Artist != "Bravo" {
			t.Errorf("second should be Bravo, got %q", sorted[1].Artist)
		}
		if sorted[2].Artist != "Alpha" {
			t.Errorf("third should be Alpha, got %q", sorted[2].Artist)
		}
		if sorted[3].Artist != "" {
			t.Errorf("last should be empty (unknown), got %q", sorted[3].Artist)
		}
	})

	t.Run("sort by title asc", func(t *testing.T) {
		sorted := SortTracks(tracks, SortTitle, SortAsc)
		if sorted[0].Title != "Alpha" {
			t.Errorf("first should be Alpha, got %q", sorted[0].Title)
		}
		if sorted[1].Title != "Bravo" {
			t.Errorf("second should be Bravo, got %q", sorted[1].Title)
		}
	})

	t.Run("sort by duration asc", func(t *testing.T) {
		sorted := SortTracks(tracks, SortDuration, SortAsc)
		if sorted[0].Duration != 100 {
			t.Errorf("first duration should be 100, got %f", sorted[0].Duration)
		}
		if sorted[3].Duration != 300 {
			t.Errorf("last duration should be 300, got %f", sorted[3].Duration)
		}
	})

	t.Run("sort by duration desc", func(t *testing.T) {
		sorted := SortTracks(tracks, SortDuration, SortDesc)
		if sorted[0].Duration != 300 {
			t.Errorf("first duration should be 300, got %f", sorted[0].Duration)
		}
		if sorted[3].Duration != 100 {
			t.Errorf("last duration should be 100, got %f", sorted[3].Duration)
		}
	})

	t.Run("sort by album asc", func(t *testing.T) {
		sorted := SortTracks(tracks, SortAlbum, SortAsc)
		if sorted[0].Album != "Blue" {
			t.Errorf("first should be Blue, got %q", sorted[0].Album)
		}
		if sorted[3].Album != "" {
			t.Errorf("last should be empty (unknown), got %q", sorted[3].Album)
		}
	})

	t.Run("sort by date added desc", func(t *testing.T) {
		dateTracks := []library.Track{
			{Title: "Old", CreatedAt: 10},
			{Title: "Newest", CreatedAt: 30},
			{Title: "Mid", CreatedAt: 20},
		}
		sorted := SortTracks(dateTracks, SortDateAdded, SortDesc)
		if sorted[0].Title != "Newest" {
			t.Errorf("first should be Newest, got %q", sorted[0].Title)
		}
		if sorted[2].Title != "Old" {
			t.Errorf("last should be Old, got %q", sorted[2].Title)
		}
	})

	t.Run("sort default returns nil", func(t *testing.T) {
		sorted := SortTracks(tracks, SortDefault, SortAsc)
		if sorted != nil {
			t.Error("SortDefault should return nil")
		}
	})

	t.Run("sort empty tracks returns nil", func(t *testing.T) {
		sorted := SortTracks(nil, SortArtist, SortAsc)
		if sorted != nil {
			t.Error("empty tracks should return nil")
		}
	})

	t.Run("original slice not modified", func(t *testing.T) {
		_ = SortTracks(tracks, SortArtist, SortAsc)
		if tracks[0].Artist != "Charlie" {
			t.Error("original slice was modified")
		}
	})
}

func TestSortTracksZeroDurationSortsLast(t *testing.T) {
	tracks := []library.Track{
		{Hash: "1", Duration: 0},
		{Hash: "2", Duration: 200},
		{Hash: "3", Duration: 100},
		{Hash: "4", Duration: 0},
	}
	t.Run("asc", func(t *testing.T) {
		sorted := SortTracks(tracks, SortDuration, SortAsc)
		if sorted[0].Hash != "3" {
			t.Errorf("first should be h3 (100), got %s", sorted[0].Hash)
		}
		if sorted[1].Hash != "2" {
			t.Errorf("second should be h2 (200), got %s", sorted[1].Hash)
		}
		if sorted[2].Duration != 0 || sorted[3].Duration != 0 {
			t.Error("zero-duration tracks should be last")
		}
	})
	t.Run("desc", func(t *testing.T) {
		sorted := SortTracks(tracks, SortDuration, SortDesc)
		if sorted[0].Hash != "2" {
			t.Errorf("first should be h2 (200), got %s", sorted[0].Hash)
		}
		if sorted[1].Hash != "3" {
			t.Errorf("second should be h3 (100), got %s", sorted[1].Hash)
		}
		if sorted[2].Duration != 0 || sorted[3].Duration != 0 {
			t.Error("zero-duration tracks should be last")
		}
	})
}

func TestSortTracksStableOrder(t *testing.T) {
	tracks := []library.Track{
		{Hash: "1", Artist: "Same", Title: "B"},
		{Hash: "2", Artist: "Same", Title: "A"},
		{Hash: "3", Artist: "Same", Title: "C"},
	}
	sorted := SortTracks(tracks, SortArtist, SortAsc)
	if sorted[0].Hash != "1" || sorted[1].Hash != "2" || sorted[2].Hash != "3" {
		t.Error("stable sort should preserve order for equal keys")
	}
}

func TestSortTracksCaseInsensitive(t *testing.T) {
	tracks := []library.Track{
		{Hash: "1", Artist: "bravo"},
		{Hash: "2", Artist: "Alpha"},
		{Hash: "3", Artist: "CHARLIE"},
	}
	sorted := SortTracks(tracks, SortArtist, SortAsc)
	if sorted[0].Artist != "Alpha" || sorted[1].Artist != "bravo" || sorted[2].Artist != "CHARLIE" {
		t.Errorf("case-insensitive sort failed: got %q, %q, %q",
			sorted[0].Artist, sorted[1].Artist, sorted[2].Artist)
	}
}
