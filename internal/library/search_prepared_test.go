package library

import (
	"reflect"
	"sync"
	"testing"
)

func TestCachedSearchPreservesMatchesAndTracksMetadataChanges(t *testing.T) {
	tracks := []Track{
		{Hash: "a", Title: "Cafe\u0301 study", Artist: "Björk", Album: "Álbum", AlbumArtist: "Various Artists", Genre: "Jazz", Path: "/one/夜.flac"},
		{Hash: "b", Title: "Unknown", Artist: "Other", Path: "/other/acdc.flac"},
	}
	search := &CachedFuzzySearcher{}
	for range 2 {
		for _, query := range []string{"", "cafe", "bjork", "a:bjork t:cafe", "aa:Various g:Jazz", "夜", "acdc", "a:", "missing"} {
			want := (SahilmFuzzySearcher{}).Search(query, tracks)
			if got := search.Search(query, tracks); !reflect.DeepEqual(got, want) {
				t.Fatalf("%s: got %+v, want %+v", query, got, want)
			}
		}
		tracks[0].Title, tracks[0].Artist = "New title", "New artist"
		tracks[0], tracks[1] = tracks[1], tracks[0]
	}
	if len(search.Search("bjork", tracks)) != 0 {
		t.Fatal("stale metadata matched")
	}
}

func TestCachedSearchCanBeShared(t *testing.T) {
	search := &CachedFuzzySearcher{}
	tracks := []Track{{Title: "Étude"}}
	var group sync.WaitGroup
	for range 4 {
		group.Go(func() {
			for range 20 {
				if len(search.Search("etude", tracks)) != 1 {
					t.Error("missing match")
				}
			}
		})
	}
	group.Wait()
}

func TestSearchHighlightStorageRemainsIndependent(t *testing.T) {
	search := &CachedFuzzySearcher{}
	tracks := []Track{{Title: "Cafe\u0301"}, {Title: "Café"}}
	first := search.Search("cafe", tracks)
	if len(first) != 2 {
		t.Fatal(first)
	}
	if !reflect.DeepEqual(first[0].MatchedIndexes, []int{1, 2, 3, 4, 5}) {
		t.Fatal("combining accent lost", first[0])
	}
	original := append([]int(nil), first[1].MatchedIndexes...)
	first[0].MatchedIndexes[0] = 99
	first[0].MatchedIndexes = append(first[0].MatchedIndexes, 42)
	if !reflect.DeepEqual(first[1].MatchedIndexes, original) {
		t.Fatal("matches share writable spans")
	}
	_ = search.Search("a", tracks)
	if !reflect.DeepEqual(first[1].MatchedIndexes, original) {
		t.Fatal("later search mutated prior results")
	}
	if got := search.Search("missing", tracks); got == nil || len(got) != 0 {
		t.Fatal("no matches must remain a completed non-nil result")
	}
}
