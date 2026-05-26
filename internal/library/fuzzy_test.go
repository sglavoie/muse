package library

import "testing"

// Compile-time check: FakeFuzzySearcher satisfies FuzzySearcher.
var _ FuzzySearcher = FakeFuzzySearcher{}

func TestFakeFuzzySearcher(t *testing.T) {
	tracks := []Track{
		{Artist: "Radiohead", Title: "Creep", Path: "/music/creep.flac"},
		{Artist: "Pink Floyd", Title: "Comfortably Numb", Path: "/music/comfortably_numb.flac"},
		{Artist: "Unknown Artist", Title: "Track 01", Path: "/music/track01.mp3"},
	}

	t.Run("EmptyQueryReturnsAll", func(t *testing.T) {
		f := FakeFuzzySearcher{}
		got := f.Search("", tracks)
		if len(got) != len(tracks) {
			t.Fatalf("expected %d matches, got %d", len(tracks), len(got))
		}
		for i, m := range got {
			if m.TrackIndex != i {
				t.Errorf("match[%d].TrackIndex = %d, want %d", i, m.TrackIndex, i)
			}
		}
	})

	t.Run("WhitespaceOnlyQueryReturnsAll", func(t *testing.T) {
		f := FakeFuzzySearcher{}
		got := f.Search(" \t\n ", tracks)
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
		f := FakeFuzzySearcher{}
		got := f.Search("radiohead", tracks)
		if len(got) != 1 || got[0].TrackIndex != 0 {
			t.Errorf("expected match on Radiohead (index 0), got %+v", got)
		}
	})

	t.Run("TitleMatch", func(t *testing.T) {
		f := FakeFuzzySearcher{}
		got := f.Search("numb", tracks)
		if len(got) != 1 || got[0].TrackIndex != 1 {
			t.Errorf("expected match on Comfortably Numb (index 1), got %+v", got)
		}
	})

	t.Run("PathMatch", func(t *testing.T) {
		f := FakeFuzzySearcher{}
		got := f.Search("track01", tracks)
		if len(got) != 1 || got[0].TrackIndex != 2 {
			t.Errorf("expected match on track01.mp3 (index 2), got %+v", got)
		}
	})

	t.Run("CaseInsensitive", func(t *testing.T) {
		f := FakeFuzzySearcher{}
		got := f.Search("PINK FLOYD", tracks)
		if len(got) != 1 || got[0].TrackIndex != 1 {
			t.Errorf("expected case-insensitive match on Pink Floyd (index 1), got %+v", got)
		}
	})

	t.Run("NoMatch", func(t *testing.T) {
		f := FakeFuzzySearcher{}
		got := f.Search("zzznomatch", tracks)
		if len(got) != 0 {
			t.Errorf("expected no matches, got %+v", got)
		}
	})

	t.Run("AlbumMatch", func(t *testing.T) {
		f := FakeFuzzySearcher{}
		tracksWithAlbum := []Track{
			{Artist: "Radiohead", Title: "Creep", Album: "Pablo Honey", Path: "/music/creep.flac"},
			{Artist: "Pink Floyd", Title: "Comfortably Numb", Album: "The Wall", Path: "/music/numb.flac"},
		}
		got := f.Search("pablo", tracksWithAlbum)
		if len(got) != 1 || got[0].TrackIndex != 0 {
			t.Errorf("expected general search to match album, got %+v", got)
		}
	})

	t.Run("ArtistPrefix", func(t *testing.T) {
		f := FakeFuzzySearcher{}
		got := f.Search("a:radiohead", tracks)
		if len(got) != 1 || got[0].TrackIndex != 0 {
			t.Errorf("expected a:radiohead to match index 0, got %+v", got)
		}
	})

	t.Run("TitlePrefix", func(t *testing.T) {
		f := FakeFuzzySearcher{}
		got := f.Search("t:numb", tracks)
		if len(got) != 1 || got[0].TrackIndex != 1 {
			t.Errorf("expected t:numb to match index 1, got %+v", got)
		}
	})

	t.Run("AlbumPrefix", func(t *testing.T) {
		f := FakeFuzzySearcher{}
		tracksWithAlbum := []Track{
			{Artist: "Radiohead", Title: "Creep", Album: "Pablo Honey", Path: "/music/creep.flac"},
			{Artist: "Pink Floyd", Title: "Comfortably Numb", Album: "The Wall", Path: "/music/numb.flac"},
		}
		got := f.Search("l:wall", tracksWithAlbum)
		if len(got) != 1 || got[0].TrackIndex != 1 {
			t.Errorf("expected l:wall to match The Wall, got %+v", got)
		}
	})

	t.Run("FieldANDLogic", func(t *testing.T) {
		f := FakeFuzzySearcher{}
		tracksWithAlbum := []Track{
			{Artist: "Radiohead", Title: "Creep", Album: "Pablo Honey", Path: "/music/creep.flac"},
			{Artist: "Radiohead", Title: "Lucky", Album: "OK Computer", Path: "/music/lucky.flac"},
		}
		got := f.Search("a:radiohead t:lucky", tracksWithAlbum)
		if len(got) != 1 || got[0].TrackIndex != 1 {
			t.Errorf("expected AND logic to match only Lucky, got %+v", got)
		}
	})

	t.Run("ResultsInIndexOrder", func(t *testing.T) {
		f := FakeFuzzySearcher{}
		// "track" appears in Path of index 2 and Title of index 2; "Unknown" in Artist of index 2
		// "c" appears in multiple tracks — verify stable ascending order
		got := f.Search("c", tracks)
		for i := 1; i < len(got); i++ {
			if got[i].TrackIndex < got[i-1].TrackIndex {
				t.Errorf("results not in ascending index order: %v", got)
			}
		}
	})
}
