package library

import (
	"slices"
	"testing"
)

func TestSearchEmptyPrefixes(t *testing.T) {
	tracks := []Track{
		{Artist: "Alice", Title: "Sun", Album: "First", Genre: "Rock"},
		{Artist: "Bob", Title: "Moon", Album: "Second", Genre: "Jazz"},
		{Artist: "Alice", Title: "x:literal", Album: "Third", Genre: "Jazz"},
	}
	cases := []struct {
		query string
		want  []int
	}{
		{"a:", []int{0, 1, 2}},
		{"t: l: g:", []int{0, 1, 2}},
		{"artist: title: album: genre:", []int{0, 1, 2}},
		{" \tARTIST: \t", []int{0, 1, 2}},
		{"Alice t:", []int{0, 2}},
		{"a:Alice l:", []int{0, 2}},
		{"a: g:Jazz", []int{1, 2}},
		{"a:Alice t:Moon", nil},
		{"a:Alice a:", []int{0, 1, 2}},
		{"x:", []int{2}},
		{"x:missing", nil},
	}
	for name, searcher := range map[string]FuzzySearcher{"production": SahilmFuzzySearcher{}, "fake": FakeFuzzySearcher{}} {
		for _, tc := range cases {
			t.Run(name+"/"+tc.query, func(t *testing.T) {
				var got []int
				for _, match := range searcher.Search(tc.query, tracks) {
					got = append(got, match.TrackIndex)
				}
				if !slices.Equal(got, tc.want) {
					t.Fatalf("matches = %v, want %v", got, tc.want)
				}
			})
		}
	}
}
