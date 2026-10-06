package library

import (
	"reflect"
	"testing"
)

func TestAccentSearchHighlightsOriginalRunes(t *testing.T) {
	for _, tc := range []struct {
		value, query string
		want         []int
	}{
		{"Björk", "BJORK", []int{0, 1, 2, 3, 4}},
		{"Cafe\u0301", "café", []int{0, 1, 2, 3, 4}},
		{"é 世界", "世界", []int{2, 3}},
		{"Cafe\u0301 noir", "noir", []int{6, 7, 8, 9}},
	} {
		got, ok := substringIndexes(tc.value, tc.query)
		if !ok || !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%q / %q = %v, %v; want %v", tc.value, tc.query, got, ok, tc.want)
		}
	}
	if FoldSearchText("♪ ❤️") != "♪ ❤️" {
		t.Fatal("emoji selector was lost")
	}
}

func TestFilenameAndAccentMatchingAgree(t *testing.T) {
	track := Track{Path: "/private/Library/Café.flac", Title: "Unknown", Artist: "Björk", Genre: "Électronique"}
	for query, want := range map[string]bool{"cafe": true, "cafe a:bjork g:electronique": true, "private": false, "t:cafe": false, "g:jazz": false} {
		fq := ParseFilterQuery(query)
		got := fq.MatchesMetadata(track.Title, track.Artist, track.Album, track.AlbumArtist, track.Genre, track.Path)
		searched := len((SahilmFuzzySearcher{}).Search(query, []Track{track})) > 0
		if got != want || searched != want {
			t.Errorf("%q: matcher=%v search=%v want=%v", query, got, searched, want)
		}
	}
}
