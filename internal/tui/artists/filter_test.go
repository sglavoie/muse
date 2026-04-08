package artists

import (
	"strings"
	"testing"

	"github.com/sglavoie/muse/internal/library"
)

func TestFilterStringsSubstringMatch(t *testing.T) {
	items := []string{"Alice", "Bob", "Charlie", "alice in chains"}
	got := FilterStrings(items, "ali")
	want := []string{"Alice", "alice in chains"}
	if len(got) != len(want) {
		t.Fatalf("FilterStrings: expected %d items, got %d: %v", len(want), len(got), got)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("FilterStrings[%d]: expected %q, got %q", i, w, got[i])
		}
	}
}

func TestFilterStringsEmptyQuery(t *testing.T) {
	items := []string{"Alice", "Bob"}
	got := FilterStrings(items, "")
	if len(got) != 2 {
		t.Errorf("FilterStrings empty query: expected 2 items, got %d", len(got))
	}
}

func TestFilterStringsNoMatch(t *testing.T) {
	items := []string{"Alice", "Bob"}
	got := FilterStrings(items, "xyz")
	if len(got) != 0 {
		t.Errorf("FilterStrings no match: expected 0, got %d", len(got))
	}
}

func TestFilterTracksMatchesTitleArtistAlbum(t *testing.T) {
	tracks := []library.Track{
		{Title: "Song About Love", Artist: "The Beatles", Album: "Abbey Road"},
		{Title: "Hate Song", Artist: "Rival Sons", Album: "Feral Roots"},
		{Title: "Lovely Day", Artist: "Bill Withers", Album: "Menagerie"},
		{Title: "Ordinary", Artist: "Lovebirds", Album: "Midnight"},
		{Title: "Waves", Artist: "Dean Lewis", Album: "Love Parade"},
	}

	t.Run("match by title", func(t *testing.T) {
		got := FilterTracks(tracks, "love")
		if len(got) != 4 {
			t.Fatalf("expected 4, got %d: %v", len(got), got)
		}
	})

	t.Run("match by artist", func(t *testing.T) {
		got := FilterTracks(tracks, "rival")
		if len(got) != 1 {
			t.Fatalf("expected 1, got %d", len(got))
		}
		if got[0].Artist != "Rival Sons" {
			t.Errorf("expected artist 'Rival Sons', got %q", got[0].Artist)
		}
	})

	t.Run("match by album", func(t *testing.T) {
		got := FilterTracks(tracks, "feral")
		if len(got) != 1 {
			t.Fatalf("expected 1, got %d", len(got))
		}
		if got[0].Album != "Feral Roots" {
			t.Errorf("expected album 'Feral Roots', got %q", got[0].Album)
		}
	})

	t.Run("empty query returns all", func(t *testing.T) {
		got := FilterTracks(tracks, "")
		if len(got) != len(tracks) {
			t.Errorf("expected %d, got %d", len(tracks), len(got))
		}
	})

	t.Run("no match returns empty", func(t *testing.T) {
		got := FilterTracks(tracks, "zzzzz")
		if len(got) != 0 {
			t.Errorf("expected 0, got %d", len(got))
		}
	})
}

func TestItemCountRespectsFilter(t *testing.T) {
	v := View{
		Level:      ArtistLevel,
		Artists:    []string{"Alice", "Bob", "Charlie"},
		FilterText: "b",
	}
	if v.itemCount() != 1 {
		t.Errorf("itemCount with filter: expected 1, got %d", v.itemCount())
	}
}

func TestCursorDownRespectsFilteredCount(t *testing.T) {
	v := View{
		Level:      ArtistLevel,
		Artists:    []string{"Alice", "Bob", "Charlie"},
		FilterText: "b",
		Cursor:     0,
	}
	v = v.CursorDown()
	if v.Cursor != 0 {
		t.Errorf("CursorDown with single filtered item: expected 0, got %d", v.Cursor)
	}
}

func TestRenderArtistsWithFilterShowsFilterBar(t *testing.T) {
	v := View{
		Level:        ArtistLevel,
		Width:        80,
		Artists:      []string{"Alice", "Bob"},
		FilterActive: true,
		FilterText:   "ali",
	}
	out := v.Render()
	if !strings.Contains(out, "Filter: ali") {
		t.Errorf("expected filter bar in output, got:\n%s", out)
	}
	if !strings.Contains(out, "Alice") {
		t.Error("expected 'Alice' in filtered output")
	}
	if strings.Contains(out, "Bob") {
		t.Error("did not expect 'Bob' in filtered output")
	}
}

func TestRenderArtistsFilterNoMatches(t *testing.T) {
	v := View{
		Level:      ArtistLevel,
		Width:      80,
		Artists:    []string{"Alice", "Bob"},
		FilterText: "zzz",
	}
	out := v.Render()
	if !strings.Contains(out, "no matches") {
		t.Errorf("expected '(no matches)' in output, got:\n%s", out)
	}
}

func TestFilterClearedOnOpenAlbums(t *testing.T) {
	v := View{
		Level:        ArtistLevel,
		Artists:      []string{"Alice", "Bob"},
		FilterActive: true,
		FilterText:   "ali",
	}
	v = v.OpenAlbums([]string{"Alpha"})
	if v.FilterActive || v.FilterText != "" {
		t.Errorf("expected filter cleared after OpenAlbums, got active=%v text=%q", v.FilterActive, v.FilterText)
	}
}

func TestFilterClearedOnBack(t *testing.T) {
	v := View{
		Level:        AlbumLevel,
		Albums:       []string{"Alpha"},
		Artists:      []string{"Alice"},
		ArtistCursor: 0,
		FilterActive: true,
		FilterText:   "al",
	}
	v = v.Back()
	if v.FilterActive || v.FilterText != "" {
		t.Errorf("expected filter cleared after Back, got active=%v text=%q", v.FilterActive, v.FilterText)
	}
}
