package library

import (
	"slices"
	"testing"
)

func TestAlbumArtistFilter(t *testing.T) {
	tracks := []Track{
		{Title: "Live", Artist: "Guest", Album: "Compilation", AlbumArtist: "Björk Ensemble", Genre: "Jazz"},
		{Title: "Studio", Artist: "Björk Ensemble", Album: "Solo", Genre: "Jazz"},
		{Title: "Other", AlbumArtist: "Björk Ensemble", Genre: "Rock"},
	}
	for _, tc := range []struct {
		query string
		want  []int
	}{
		{"aa:bjork ensemble", []int{0, 2}},
		{"ALBUMARTIST:BJORK ENSEMBLE g:jazz", []int{0}},
		{"aa:bjork a:guest l:compilation t:live", []int{0}},
		{"aa:missing", nil},
		{"aa:", []int{0, 1, 2}},
		{"aa: g:jazz", []int{0, 1}},
		{"aa:missing albumartist:bjork", []int{0, 2}},
	} {
		t.Run(tc.query, func(t *testing.T) {
			fq := ParseFilterQuery(tc.query)
			var got []int
			for i, tr := range tracks {
				if fq.MatchesMetadata(tr.Title, tr.Artist, tr.Album, tr.AlbumArtist, tr.Genre, tr.Path) {
					got = append(got, i)
				}
			}
			if !slices.Equal(got, tc.want) {
				t.Fatalf("metadata: %v, want %v", got, tc.want)
			}
			for _, searcher := range []FuzzySearcher{SahilmFuzzySearcher{}, FakeFuzzySearcher{}} {
				got = nil
				for _, m := range searcher.Search(tc.query, tracks) {
					got = append(got, m.TrackIndex)
				}
				if !slices.Equal(got, tc.want) {
					t.Fatalf("%T: %v, want %v", searcher, got, tc.want)
				}
			}
		})
	}
}
