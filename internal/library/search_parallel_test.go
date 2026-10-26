package library

import (
	"fmt"
	"reflect"
	"testing"
)

// TestSearchTracks_ParallelMatchesSequential verifies that a library large
// enough to be searched in parallel chunks yields exactly the matches, in the
// same order and with the same highlight positions, as one sequential scan, on
// both a cold and a warm cache.
func TestSearchTracks_ParallelMatchesSequential(t *testing.T) {
	const n = 5*searchChunkSize + 123
	tracks := make([]Track, n)
	for i := range tracks {
		tracks[i] = Track{
			Path:   fmt.Sprintf("/music/a%d/%d.flac", i%37, i),
			Artist: fmt.Sprintf("Artíst %03d", i%300),
			Title:  fmt.Sprintf("Étude Ñumber %06d", i),
			Album:  fmt.Sprintf("Album %d", i%91),
			Genre:  []string{"Rock", "Jazz", "Électro"}[i%3],
		}
	}
	queries := []string{"e", "etude", "etude 0123", "numbe", "t:etude number 01", "a:artist 01 g:jazz", "electro", "flac", "not-found"}

	cached := &CachedFuzzySearcher{}
	for _, query := range queries {
		fq := ParseFilterQuery(query)
		for _, term := range []*string{&fq.Artist, &fq.Title, &fq.Album, &fq.Genre, &fq.AlbumArtist, &fq.General} {
			*term = FoldSearchText(*term)
		}
		want := searchChunk(fq, tracks, 0, prepareSearchValue)
		for pass, got := range map[string][]Match{
			"uncached":   SahilmFuzzySearcher{}.Search(query, tracks),
			"cache-cold": cached.Search(query, tracks),
			"cache-warm": cached.Search(query, tracks),
		} {
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("%q (%s): parallel search differs from sequential: got %d matches, want %d",
					query, pass, len(got), len(want))
			}
		}
	}
	if len(cached.values) == 0 {
		t.Fatal("parallel search left the normalization cache empty")
	}
}
