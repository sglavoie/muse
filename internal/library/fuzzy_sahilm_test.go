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

func TestSahilmFuzzySearcher_FieldSpecific(t *testing.T) {
	tracks := []Track{
		{Artist: "Radiohead", Title: "Creep", Album: "Pablo Honey", Path: "/music/creep.flac"},
		{Artist: "Pink Floyd", Title: "Comfortably Numb", Album: "The Wall", Path: "/music/numb.flac"},
		{Artist: "Radiohead", Title: "Lucky", Album: "OK Computer", Path: "/music/lucky.flac"},
	}

	s := SahilmFuzzySearcher{}

	t.Run("ArtistPrefix", func(t *testing.T) {
		got := s.Search("a:radiohead", tracks)
		if len(got) != 2 {
			t.Fatalf("expected 2 matches for a:radiohead, got %d", len(got))
		}
		if got[0].TrackIndex != 0 || got[1].TrackIndex != 2 {
			t.Errorf("unexpected tracks: %+v", got)
		}
	})

	t.Run("TitlePrefix", func(t *testing.T) {
		got := s.Search("t:creep", tracks)
		if len(got) != 1 || got[0].TrackIndex != 0 {
			t.Errorf("expected match on Creep (index 0), got %+v", got)
		}
	})

	t.Run("AlbumPrefix", func(t *testing.T) {
		got := s.Search("l:wall", tracks)
		if len(got) != 1 || got[0].TrackIndex != 1 {
			t.Errorf("expected match on The Wall (index 1), got %+v", got)
		}
	})

	t.Run("ANDLogic", func(t *testing.T) {
		got := s.Search("a:radiohead t:creep", tracks)
		if len(got) != 1 || got[0].TrackIndex != 0 {
			t.Errorf("expected 1 match (AND logic), got %+v", got)
		}
	})

	t.Run("GeneralPlusField", func(t *testing.T) {
		got := s.Search("creep a:radiohead", tracks)
		if len(got) != 1 || got[0].TrackIndex != 0 {
			t.Errorf("expected 1 match (general+field), got %+v", got)
		}
	})

	t.Run("FieldPositionsPopulated", func(t *testing.T) {
		got := s.Search("a:radio", tracks)
		if len(got) == 0 {
			t.Fatal("expected matches for a:radio")
		}
		for _, m := range got {
			if m.ArtistPositions == nil {
				t.Errorf("track %d: ArtistPositions should not be nil", m.TrackIndex)
			}
			if m.MatchedIndexes != nil {
				t.Errorf("track %d: MatchedIndexes should be nil for field-specific query", m.TrackIndex)
			}
		}
	})

	t.Run("AlbumPositionsPopulated", func(t *testing.T) {
		got := s.Search("l:computer", tracks)
		if len(got) != 1 {
			t.Fatalf("expected 1 match, got %d", len(got))
		}
		if got[0].AlbumPositions == nil {
			t.Error("AlbumPositions should not be nil")
		}
		// "Computer" starts at index 3 in "OK Computer"
		if got[0].AlbumPositions[0] != 3 {
			t.Errorf("AlbumPositions[0] = %d, want 3", got[0].AlbumPositions[0])
		}
	})

	t.Run("GeneralSearchIncludesAlbum", func(t *testing.T) {
		got := s.Search("pablo", tracks)
		if len(got) != 1 || got[0].TrackIndex != 0 {
			t.Errorf("expected general search to match album 'Pablo Honey', got %+v", got)
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
