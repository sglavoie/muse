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
