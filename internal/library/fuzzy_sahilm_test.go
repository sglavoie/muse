package library

import (
	"fmt"
	"testing"
	"time"
)

// Compile-time check: SahilmFuzzySearcher satisfies FuzzySearcher.
var _ FuzzySearcher = SahilmFuzzySearcher{}

func TestSahilmFuzzySearcher(t *testing.T) {
	tracks := []Track{
		{Artist: "Radiohead", Title: "Creep", Path: "/music/creep.flac"},
		{Artist: "Pink Floyd", Title: "Comfortably Numb", Path: "/music/comfortably_numb.flac"},
		{Artist: "Unknown Artist", Title: "Track 01", Path: "/music/track01.mp3"},
	}

	s := SahilmFuzzySearcher{}

	t.Run("EmptyQueryReturnsAll", func(t *testing.T) {
		got := s.Search("", tracks)
		if len(got) != len(tracks) {
			t.Fatalf("expected %d matches, got %d", len(tracks), len(got))
		}
		for i, m := range got {
			if m.TrackIndex != i {
				t.Errorf("match[%d].TrackIndex = %d, want %d", i, m.TrackIndex, i)
			}
		}
	})

	t.Run("ArtistMatch", func(t *testing.T) {
		got := s.Search("Radiohead", tracks)
		if len(got) == 0 {
			t.Fatal("expected at least one match for 'Radiohead', got none")
		}
		found := false
		for _, m := range got {
			if m.TrackIndex == 0 {
				found = true
			}
		}
		if !found {
			t.Errorf("expected match on Radiohead (index 0), got %+v", got)
		}
	})

	t.Run("TitleMatch", func(t *testing.T) {
		got := s.Search("Numb", tracks)
		if len(got) == 0 {
			t.Fatal("expected at least one match for 'Numb', got none")
		}
		found := false
		for _, m := range got {
			if m.TrackIndex == 1 {
				found = true
			}
		}
		if !found {
			t.Errorf("expected match on Comfortably Numb (index 1), got %+v", got)
		}
	})

	t.Run("FilenameMatch", func(t *testing.T) {
		got := s.Search("track01", tracks)
		if len(got) == 0 {
			t.Fatal("expected at least one match for 'track01', got none")
		}
		found := false
		for _, m := range got {
			if m.TrackIndex == 2 {
				found = true
			}
		}
		if !found {
			t.Errorf("expected match on track01.mp3 (index 2), got %+v", got)
		}
	})

	t.Run("MatchedIndexesNonNilForHits", func(t *testing.T) {
		got := s.Search("Creep", tracks)
		if len(got) == 0 {
			t.Fatal("expected at least one match for 'Creep'")
		}
		for _, m := range got {
			if m.MatchedIndexes == nil {
				t.Errorf("match for track %d has nil MatchedIndexes", m.TrackIndex)
			}
		}
	})

	t.Run("NoMatch", func(t *testing.T) {
		got := s.Search("zzznomatch", tracks)
		if len(got) != 0 {
			t.Errorf("expected no matches, got %+v", got)
		}
	})
}

func TestSahilmFuzzySearcher_Performance(t *testing.T) {
	const n = 10_000
	tracks := make([]Track, n)
	for i := range tracks {
		tracks[i] = Track{
			Artist: fmt.Sprintf("Artist %d", i),
			Title:  fmt.Sprintf("Title %d", i),
			Path:   fmt.Sprintf("/music/track_%d.flac", i),
		}
	}

	s := SahilmFuzzySearcher{}
	start := time.Now()
	results := s.Search("Artist 99", tracks)
	elapsed := time.Since(start)

	if elapsed > 200*time.Millisecond {
		t.Errorf("search took %v, want < 200ms", elapsed)
	}
	if len(results) == 0 {
		t.Error("expected at least one result for 'Artist 99' in 10k tracks")
	}
}
